package k8s

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// MutatingWebhook implements a Kubernetes mutating admission webhook
// that automatically injects secret sidecar/init containers into pods.
type MutatingWebhook struct {
	config *WebhookConfig
	server *http.Server

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Stats
	stats WebhookStats
}

// WebhookStats contains webhook statistics.
type WebhookStats struct {
	StartTime       time.Time
	RequestsTotal   int64
	RequestsAllowed int64
	RequestsDenied  int64
	RequestsSkipped int64
	RequestsErrored int64
}

// AdmissionReview represents a Kubernetes admission review request/response.
type AdmissionReview struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Request    *AdmissionRequest `json:"request,omitempty"`
	Response   *AdmissionResponse `json:"response,omitempty"`
}

// AdmissionRequest is the request portion of an AdmissionReview.
type AdmissionRequest struct {
	UID       string          `json:"uid"`
	Kind      GroupVersionKind `json:"kind"`
	Resource  GroupVersionResource `json:"resource"`
	Namespace string          `json:"namespace"`
	Name      string          `json:"name"`
	Operation string          `json:"operation"`
	Object    json.RawMessage `json:"object"`
	OldObject json.RawMessage `json:"oldObject,omitempty"`
}

// AdmissionResponse is the response portion of an AdmissionReview.
type AdmissionResponse struct {
	UID     string `json:"uid"`
	Allowed bool   `json:"allowed"`
	Status  *Status `json:"status,omitempty"`
	Patch   []byte `json:"patch,omitempty"`
	PatchType *string `json:"patchType,omitempty"`
}

// GroupVersionKind identifies a Kubernetes resource type.
type GroupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// GroupVersionResource identifies a Kubernetes resource.
type GroupVersionResource struct {
	Group    string `json:"group"`
	Version  string `json:"version"`
	Resource string `json:"resource"`
}

// Status contains status information.
type Status struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// PodSpec represents the relevant parts of a Kubernetes PodSpec.
type PodSpec struct {
	Metadata       PodMetadata              `json:"metadata"`
	Spec           PodSpecInner             `json:"spec"`
}

// PodMetadata contains pod metadata.
type PodMetadata struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodSpecInner contains the spec portion of a pod.
type PodSpecInner struct {
	Containers               []Container         `json:"containers,omitempty"`
	InitContainers           []Container         `json:"initContainers,omitempty"`
	Volumes                  []Volume            `json:"volumes,omitempty"`
	ServiceAccountName       string              `json:"serviceAccountName,omitempty"`
	ShareProcessNamespace    *bool               `json:"shareProcessNamespace,omitempty"`
}

// Container represents a Kubernetes container.
type Container struct {
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Command      []string          `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Env          []EnvVar          `json:"env,omitempty"`
	VolumeMounts []VolumeMount     `json:"volumeMounts,omitempty"`
	Resources    *ContainerResources `json:"resources,omitempty"`
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// VolumeMount represents a volume mount.
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// Volume represents a Kubernetes volume.
type Volume struct {
	Name     string      `json:"name"`
	EmptyDir *EmptyDir   `json:"emptyDir,omitempty"`
	Projected *Projected `json:"projected,omitempty"`
}

// EmptyDir represents an emptyDir volume.
type EmptyDir struct {
	Medium string `json:"medium,omitempty"`
}

// Projected represents a projected volume.
type Projected struct {
	Sources []ProjectedVolumeSource `json:"sources,omitempty"`
}

// ProjectedVolumeSource represents a projected volume source.
type ProjectedVolumeSource struct {
	ServiceAccountToken *ServiceAccountTokenProjection `json:"serviceAccountToken,omitempty"`
}

// ServiceAccountTokenProjection represents a service account token projection.
type ServiceAccountTokenProjection struct {
	Path              string `json:"path"`
	ExpirationSeconds int64  `json:"expirationSeconds,omitempty"`
	Audience          string `json:"audience,omitempty"`
}

// ContainerResources represents container resource requirements.
type ContainerResources struct {
	Limits   map[string]string `json:"limits,omitempty"`
	Requests map[string]string `json:"requests,omitempty"`
}

// JSONPatch represents a JSON Patch operation.
type JSONPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// NewMutatingWebhook creates a new mutating admission webhook.
func NewMutatingWebhook(config *WebhookConfig) (*MutatingWebhook, error) {
	if config == nil {
		config = DefaultWebhookConfig()
	}

	if config.CertFile == "" || config.KeyFile == "" {
		return nil, fmt.Errorf("TLS certificate and key are required")
	}

	return &MutatingWebhook{
		config: config,
		stats: WebhookStats{
			StartTime: time.Now(),
		},
	}, nil
}

// Run starts the webhook server and blocks until stopped.
func (w *MutatingWebhook) Run(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("webhook already running")
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.mu.Unlock()

	defer func() {
		close(w.doneCh)
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	// Load TLS certificate
	cert, err := tls.LoadX509KeyPair(w.config.CertFile, w.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", w.handleMutate)
	mux.HandleFunc("/health", w.handleHealth)
	mux.HandleFunc("/ready", w.handleReady)

	w.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", w.config.Port),
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("starting webhook server on port %d\n", w.config.Port)
		if err := w.server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return w.server.Shutdown(shutdownCtx)
	case <-w.stopCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return w.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Stop stops the webhook server.
func (w *MutatingWebhook) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()

	close(w.stopCh)
	<-w.doneCh
	return nil
}

// Stats returns the webhook statistics.
func (w *MutatingWebhook) Stats() WebhookStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.stats
}

func (w *MutatingWebhook) handleMutate(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	w.stats.RequestsTotal++
	w.mu.Unlock()

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.writeError(rw, "", http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse admission review
	var review AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		w.writeError(rw, "", http.StatusBadRequest, "failed to parse admission review")
		return
	}

	if review.Request == nil {
		w.writeError(rw, "", http.StatusBadRequest, "missing request in admission review")
		return
	}

	// Handle the admission request
	response := w.mutate(review.Request)

	// Build response
	review.Response = response
	review.Request = nil

	// Write response
	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(review); err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
}

func (w *MutatingWebhook) mutate(req *AdmissionRequest) *AdmissionResponse {
	response := &AdmissionResponse{
		UID:     req.UID,
		Allowed: true,
	}

	// Only handle pods
	if req.Kind.Kind != "Pod" {
		w.mu.Lock()
		w.stats.RequestsSkipped++
		w.mu.Unlock()
		return response
	}

	// Parse pod
	var pod PodSpec
	if err := json.Unmarshal(req.Object, &pod); err != nil {
		w.mu.Lock()
		w.stats.RequestsErrored++
		w.mu.Unlock()
		response.Status = &Status{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("failed to parse pod: %v", err),
		}
		return response
	}

	// Check if injection is enabled
	if !w.shouldInject(&pod) {
		w.mu.Lock()
		w.stats.RequestsSkipped++
		w.mu.Unlock()
		return response
	}

	// Check if already injected
	if w.isInjected(&pod) {
		w.mu.Lock()
		w.stats.RequestsSkipped++
		w.mu.Unlock()
		return response
	}

	// Parse injection spec from annotations
	spec, err := w.parseInjectionSpec(&pod)
	if err != nil {
		w.mu.Lock()
		w.stats.RequestsErrored++
		w.mu.Unlock()
		response.Allowed = false
		response.Status = &Status{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("failed to parse injection spec: %v", err),
		}
		return response
	}

	// Build patches
	patches := w.buildPatches(&pod, spec)

	if len(patches) > 0 {
		patchBytes, err := json.Marshal(patches)
		if err != nil {
			w.mu.Lock()
			w.stats.RequestsErrored++
			w.mu.Unlock()
			response.Status = &Status{
				Code:    http.StatusInternalServerError,
				Message: fmt.Sprintf("failed to marshal patches: %v", err),
			}
			return response
		}

		patchType := "JSONPatch"
		response.Patch = patchBytes
		response.PatchType = &patchType
	}

	w.mu.Lock()
	w.stats.RequestsAllowed++
	w.mu.Unlock()

	return response
}

func (w *MutatingWebhook) shouldInject(pod *PodSpec) bool {
	if pod.Metadata.Annotations == nil {
		return false
	}

	inject, ok := pod.Metadata.Annotations[AnnotationInject]
	return ok && inject == "true"
}

func (w *MutatingWebhook) isInjected(pod *PodSpec) bool {
	if pod.Metadata.Labels == nil {
		return false
	}

	_, injected := pod.Metadata.Labels[LabelInjected]
	return injected
}

func (w *MutatingWebhook) parseInjectionSpec(pod *PodSpec) (*PodInjectionSpec, error) {
	spec := &PodInjectionSpec{
		Enabled: true,
		Mode:    w.config.InjectorConfig.Mode,
	}

	// Parse mode override
	if mode, ok := pod.Metadata.Annotations[AnnotationMode]; ok {
		spec.Mode = InjectionMode(mode)
	}

	// Parse secrets
	if secretsJSON, ok := pod.Metadata.Annotations[AnnotationSecrets]; ok {
		var secrets []SecretInjection
		if err := json.Unmarshal([]byte(secretsJSON), &secrets); err != nil {
			return nil, fmt.Errorf("invalid secrets annotation: %w", err)
		}
		spec.Secrets = secrets
	}

	// Parse service account auth
	if saAuth, ok := pod.Metadata.Annotations[AnnotationServiceAccountAuth]; ok {
		spec.ServiceAccountAuth = saAuth == "true"
	}

	return spec, nil
}

func (w *MutatingWebhook) buildPatches(pod *PodSpec, spec *PodInjectionSpec) []JSONPatch {
	var patches []JSONPatch

	// Add injected label
	if pod.Metadata.Labels == nil {
		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/labels",
			Value: map[string]string{LabelInjected: "true"},
		})
	} else {
		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/labels/" + escapeJSONPointer(LabelInjected),
			Value: "true",
		})
	}

	// Add status annotation
	if pod.Metadata.Annotations == nil {
		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: map[string]string{AnnotationStatus: "injected"},
		})
	} else {
		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/annotations/" + escapeJSONPointer(AnnotationStatus),
			Value: "injected",
		})
	}

	// Build sidecar spec builder
	builder := NewSidecarSpecBuilder(w.config.InjectorConfig).
		WithInjectionSpec(spec)

	// Add secrets volume
	volumePatch := JSONPatch{
		Op:   "add",
		Path: "/spec/volumes/-",
		Value: builder.BuildVolumeSpec(),
	}
	if len(pod.Spec.Volumes) == 0 {
		volumePatch.Path = "/spec/volumes"
		volumePatch.Value = []interface{}{builder.BuildVolumeSpec()}
	}
	patches = append(patches, volumePatch)

	// Add service account token volume if needed
	if spec.ServiceAccountAuth {
		saVolumePatch := JSONPatch{
			Op:    "add",
			Path:  "/spec/volumes/-",
			Value: builder.BuildServiceAccountVolumeSpec(),
		}
		patches = append(patches, saVolumePatch)
	}

	// Add init container if mode is init or both
	if spec.Mode == InjectionModeInit || spec.Mode == InjectionModeBoth {
		initBuilder := NewInitContainerSpecBuilder(w.config.InjectorConfig).
			WithInjectionSpec(spec)

		initPatch := JSONPatch{
			Op:   "add",
			Path: "/spec/initContainers/-",
			Value: initBuilder.BuildContainerSpec(),
		}
		if len(pod.Spec.InitContainers) == 0 {
			initPatch.Path = "/spec/initContainers"
			initPatch.Value = []interface{}{initBuilder.BuildContainerSpec()}
		}
		patches = append(patches, initPatch)
	}

	// Add sidecar container if mode is sidecar or both
	if spec.Mode == InjectionModeSidecar || spec.Mode == InjectionModeBoth {
		sidecarPatch := JSONPatch{
			Op:    "add",
			Path:  "/spec/containers/-",
			Value: builder.BuildContainerSpec(),
		}
		patches = append(patches, sidecarPatch)
	}

	// Add volume mounts to existing containers
	for i := range pod.Spec.Containers {
		mountPatch := JSONPatch{
			Op:   "add",
			Path: fmt.Sprintf("/spec/containers/%d/volumeMounts/-", i),
			Value: VolumeMount{
				Name:      "secrets-volume",
				MountPath: w.config.InjectorConfig.SecretVolumePath,
				ReadOnly:  true,
			},
		}
		if len(pod.Spec.Containers[i].VolumeMounts) == 0 {
			mountPatch.Path = fmt.Sprintf("/spec/containers/%d/volumeMounts", i)
			mountPatch.Value = []VolumeMount{{
				Name:      "secrets-volume",
				MountPath: w.config.InjectorConfig.SecretVolumePath,
				ReadOnly:  true,
			}}
		}
		patches = append(patches, mountPatch)
	}

	return patches
}

func (w *MutatingWebhook) handleHealth(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte("ok"))
}

func (w *MutatingWebhook) handleReady(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte("ready"))
}

func (w *MutatingWebhook) writeError(rw http.ResponseWriter, uid string, code int, message string) {
	w.mu.Lock()
	w.stats.RequestsErrored++
	w.mu.Unlock()

	response := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Response: &AdmissionResponse{
			UID:     uid,
			Allowed: false,
			Status: &Status{
				Code:    code,
				Message: message,
			},
		},
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	json.NewEncoder(rw).Encode(response)
}

// escapeJSONPointer escapes a string for use in a JSON Pointer path.
func escapeJSONPointer(s string) string {
	// Replace ~ with ~0 and / with ~1
	result := ""
	for _, c := range s {
		switch c {
		case '~':
			result += "~0"
		case '/':
			result += "~1"
		default:
			result += string(c)
		}
	}
	return result
}

// =============================================================================
// Webhook Configuration Registration
// =============================================================================

// MutatingWebhookConfiguration represents a Kubernetes MutatingWebhookConfiguration.
type MutatingWebhookConfiguration struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"`
	Metadata   Metadata  `json:"metadata"`
	Webhooks   []Webhook `json:"webhooks"`
}

// Metadata contains Kubernetes object metadata.
type Metadata struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Webhook defines a webhook configuration.
type Webhook struct {
	Name                    string              `json:"name"`
	AdmissionReviewVersions []string            `json:"admissionReviewVersions"`
	SideEffects             string              `json:"sideEffects"`
	FailurePolicy           string              `json:"failurePolicy"`
	MatchPolicy             string              `json:"matchPolicy"`
	ClientConfig            WebhookClientConfig `json:"clientConfig"`
	Rules                   []WebhookRule       `json:"rules"`
	NamespaceSelector       *LabelSelector      `json:"namespaceSelector,omitempty"`
	ObjectSelector          *LabelSelector      `json:"objectSelector,omitempty"`
}

// WebhookClientConfig defines how to communicate with the webhook.
type WebhookClientConfig struct {
	Service  *ServiceReference `json:"service,omitempty"`
	URL      *string           `json:"url,omitempty"`
	CABundle []byte            `json:"caBundle,omitempty"`
}

// ServiceReference points to a Kubernetes service.
type ServiceReference struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Port      int    `json:"port"`
}

// WebhookRule defines what resources the webhook handles.
type WebhookRule struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Operations  []string `json:"operations"`
	Resources   []string `json:"resources"`
	Scope       string   `json:"scope"`
}

// BuildWebhookConfiguration builds a MutatingWebhookConfiguration YAML.
func BuildWebhookConfiguration(config *WebhookConfig, serviceName, serviceNamespace string) *MutatingWebhookConfiguration {
	if config == nil {
		config = DefaultWebhookConfig()
	}

	return &MutatingWebhookConfiguration{
		APIVersion: "admissionregistration.k8s.io/v1",
		Kind:       "MutatingWebhookConfiguration",
		Metadata: Metadata{
			Name: "keystone-secret-injector",
			Labels: map[string]string{
				"app.kubernetes.io/name":    "keystone-secret-injector",
				"app.kubernetes.io/part-of": "keystone",
			},
		},
		Webhooks: []Webhook{
			{
				Name:                    "secrets.keystone.io",
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             "None",
				FailurePolicy:           config.FailurePolicy,
				MatchPolicy:             "Equivalent",
				ClientConfig: WebhookClientConfig{
					Service: &ServiceReference{
						Namespace: serviceNamespace,
						Name:      serviceName,
						Path:      "/mutate",
						Port:      config.Port,
					},
					CABundle: config.CABundle,
				},
				Rules: []WebhookRule{
					{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Operations:  []string{"CREATE"},
						Resources:   []string{"pods"},
						Scope:       "Namespaced",
					},
				},
				NamespaceSelector: config.NamespaceSelector,
				ObjectSelector:    config.ObjectSelector,
			},
		},
	}
}
