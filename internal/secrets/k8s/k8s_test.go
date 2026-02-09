package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets/injection"
)

// mockSecretSource implements injection.SecretSource for testing.
type mockSecretSource struct {
	secrets map[string]*injection.Secret
}

func newMockSecretSource() *mockSecretSource {
	return &mockSecretSource{
		secrets: make(map[string]*injection.Secret),
	}
}

func (m *mockSecretSource) GetSecret(ctx context.Context, path string) (*injection.Secret, error) {
	if s, ok := m.secrets[path]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockSecretSource) WatchSecret(ctx context.Context, path string) (<-chan *injection.Secret, error) {
	ch := make(chan *injection.Secret)
	// Simple mock: close immediately
	close(ch)
	return ch, nil
}

func (m *mockSecretSource) addSecret(path string, data map[string]interface{}) {
	m.secrets[path] = &injection.Secret{
		Path:    path,
		Data:    data,
		Version: 1,
	}
}

// =============================================================================
// Sidecar Tests
// =============================================================================

func TestNewSidecar(t *testing.T) {
	source := newMockSecretSource()

	t.Run("nil config", func(t *testing.T) {
		_, err := NewSidecar(nil, source)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("nil source", func(t *testing.T) {
		_, err := NewSidecar(&SidecarConfig{}, nil)
		if err == nil {
			t.Error("expected error for nil source")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		config := &SidecarConfig{
			SecretVolumePath: "/secrets",
			RefreshInterval:  10 * time.Second,
		}
		sidecar, err := NewSidecar(config, source)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sidecar == nil {
			t.Fatal("expected non-nil sidecar")
		}
	})

	t.Run("default values", func(t *testing.T) {
		config := &SidecarConfig{}
		sidecar, err := NewSidecar(config, source)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sidecar.config.SecretVolumePath != "/secrets" {
			t.Errorf("expected default path /secrets, got %s", sidecar.config.SecretVolumePath)
		}
		if sidecar.config.RefreshInterval != 30*time.Second {
			t.Errorf("expected default refresh 30s, got %v", sidecar.config.RefreshInterval)
		}
	})
}

func TestSidecarStop(t *testing.T) {
	source := newMockSecretSource()
	config := &SidecarConfig{
		SecretVolumePath: "/secrets",
	}
	sidecar, err := NewSidecar(config, source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stop without running should not error
	if err := sidecar.Stop(); err != nil {
		t.Errorf("unexpected error stopping non-running sidecar: %v", err)
	}
}

func TestSidecarStats(t *testing.T) {
	source := newMockSecretSource()
	config := &SidecarConfig{}
	sidecar, err := NewSidecar(config, source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := sidecar.Stats()
	if stats.StartTime.IsZero() {
		t.Error("expected non-zero start time")
	}
}

func TestSidecarSpecBuilder(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		builder := NewSidecarSpecBuilder(nil)
		spec := builder.BuildContainerSpec()

		name, ok := spec["name"].(string)
		if !ok || name != "keystone-secret-sidecar" {
			t.Errorf("unexpected container name: %v", spec["name"])
		}

		image, ok := spec["image"].(string)
		if !ok || image == "" {
			t.Error("expected non-empty image")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		config := &InjectorConfig{
			Image:            "custom-image:v1",
			SecretVolumePath: "/custom/secrets",
			RefreshInterval:  60 * time.Second,
		}
		builder := NewSidecarSpecBuilder(config)
		spec := builder.BuildContainerSpec()

		if spec["image"] != "custom-image:v1" {
			t.Errorf("unexpected image: %v", spec["image"])
		}
	})

	t.Run("with injection spec", func(t *testing.T) {
		injSpec := &PodInjectionSpec{
			Enabled:            true,
			ServiceAccountAuth: true,
			Secrets: []SecretInjection{
				{Name: "db-creds", SecretPath: "secrets/db"},
			},
		}
		builder := NewSidecarSpecBuilder(nil).WithInjectionSpec(injSpec)
		spec := builder.BuildContainerSpec()

		volumeMounts, ok := spec["volumeMounts"].([]map[string]interface{})
		if !ok {
			t.Fatal("expected volumeMounts array")
		}
		// Should have secrets-volume and sa-token
		if len(volumeMounts) != 2 {
			t.Errorf("expected 2 volume mounts, got %d", len(volumeMounts))
		}
	})

	t.Run("build volume spec", func(t *testing.T) {
		builder := NewSidecarSpecBuilder(nil)
		vol := builder.BuildVolumeSpec()

		if vol["name"] != "secrets-volume" {
			t.Errorf("unexpected volume name: %v", vol["name"])
		}
		emptyDir, ok := vol["emptyDir"].(map[string]interface{})
		if !ok {
			t.Fatal("expected emptyDir")
		}
		if emptyDir["medium"] != "Memory" {
			t.Errorf("unexpected medium: %v", emptyDir["medium"])
		}
	})
}

// =============================================================================
// Init Container Tests
// =============================================================================

func TestNewInitContainer(t *testing.T) {
	source := newMockSecretSource()

	t.Run("nil config", func(t *testing.T) {
		_, err := NewInitContainer(nil, source)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("nil source", func(t *testing.T) {
		_, err := NewInitContainer(&InitContainerConfig{}, nil)
		if err == nil {
			t.Error("expected error for nil source")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		config := &InitContainerConfig{
			SecretVolumePath: "/secrets",
			Timeout:          30 * time.Second,
		}
		init, err := NewInitContainer(config, source)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if init == nil {
			t.Fatal("expected non-nil init container")
		}
	})

	t.Run("default values", func(t *testing.T) {
		config := &InitContainerConfig{}
		init, err := NewInitContainer(config, source)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if init.config.SecretVolumePath != "/secrets" {
			t.Errorf("expected default path /secrets, got %s", init.config.SecretVolumePath)
		}
		if init.config.Timeout != 60*time.Second {
			t.Errorf("expected default timeout 60s, got %v", init.config.Timeout)
		}
		if init.config.MaxRetries != 3 {
			t.Errorf("expected default retries 3, got %d", init.config.MaxRetries)
		}
	})
}

func TestInitContainerSpecBuilder(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		builder := NewInitContainerSpecBuilder(nil)
		spec := builder.BuildContainerSpec()

		name, ok := spec["name"].(string)
		if !ok || name != "keystone-secret-init" {
			t.Errorf("unexpected container name: %v", spec["name"])
		}

		resources, ok := spec["resources"].(map[string]interface{})
		if !ok {
			t.Fatal("expected resources")
		}
		limits, ok := resources["limits"].(map[string]string)
		if !ok {
			t.Fatal("expected limits")
		}
		if limits["cpu"] != "50m" {
			t.Errorf("unexpected cpu limit: %v", limits["cpu"])
		}
	})
}

func TestParseSecretsFromEnv(t *testing.T) {
	t.Run("empty env", func(t *testing.T) {
		os.Unsetenv("KEYSTONE_SECRETS")
		secrets, err := ParseSecretsFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if secrets != nil {
			t.Error("expected nil secrets")
		}
	})

	t.Run("valid json", func(t *testing.T) {
		secretsJSON := `[{"name":"db","secret_path":"secrets/db"}]`
		os.Setenv("KEYSTONE_SECRETS", secretsJSON)
		defer os.Unsetenv("KEYSTONE_SECRETS")

		secrets, err := ParseSecretsFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 1 {
			t.Fatalf("expected 1 secret, got %d", len(secrets))
		}
		if secrets[0].Name != "db" {
			t.Errorf("unexpected secret name: %s", secrets[0].Name)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		os.Setenv("KEYSTONE_SECRETS", "not-json")
		defer os.Unsetenv("KEYSTONE_SECRETS")

		_, err := ParseSecretsFromEnv()
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})
}

// =============================================================================
// Secret Sync Tests
// =============================================================================

func TestNewSecretSync(t *testing.T) {
	source := newMockSecretSource()
	client := NewMockKubernetesClient()

	t.Run("nil source", func(t *testing.T) {
		_, err := NewSecretSync(nil, nil, client)
		if err == nil {
			t.Error("expected error for nil source")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		_, err := NewSecretSync(nil, source, nil)
		if err == nil {
			t.Error("expected error for nil client")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		sync, err := NewSecretSync(nil, source, client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sync == nil {
			t.Fatal("expected non-nil sync")
		}
	})
}

func TestSecretSyncOnce(t *testing.T) {
	source := newMockSecretSource()
	source.addSecret("secrets/db", map[string]interface{}{
		"username": "admin",
		"password": "secret123",
	})

	client := NewMockKubernetesClient()
	config := &SyncConfig{
		Namespace: "default",
		Secrets: []SyncSecretSpec{
			{
				SourcePath: "secrets/db",
				DestName:   "db-credentials",
			},
		},
	}

	sync, err := NewSecretSync(config, source, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	if err := sync.SyncOnce(ctx); err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Verify secret was created
	secret, err := client.GetSecret(ctx, "default", "db-credentials")
	if err != nil {
		t.Fatalf("get secret error: %v", err)
	}
	if secret == nil {
		t.Fatal("expected secret to be created")
	}
	if string(secret.Data["username"]) != "admin" {
		t.Errorf("unexpected username: %s", string(secret.Data["username"]))
	}
}

func TestSecretSyncUpdate(t *testing.T) {
	source := newMockSecretSource()
	source.addSecret("secrets/db", map[string]interface{}{
		"password": "original",
	})

	client := NewMockKubernetesClient()
	config := &SyncConfig{
		Namespace: "default",
		Secrets: []SyncSecretSpec{
			{SourcePath: "secrets/db", DestName: "db-creds"},
		},
	}

	sync, err := NewSecretSync(config, source, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()

	// First sync creates
	if err := sync.SyncOnce(ctx); err != nil {
		t.Fatalf("first sync error: %v", err)
	}

	// Update source with new version
	source.secrets["secrets/db"].Data["password"] = "updated"
	source.secrets["secrets/db"].Version = 2

	// Second sync updates
	if err := sync.SyncOnce(ctx); err != nil {
		t.Fatalf("second sync error: %v", err)
	}

	secret, _ := client.GetSecret(ctx, "default", "db-creds")
	if string(secret.Data["password"]) != "updated" {
		t.Errorf("expected updated password, got %s", string(secret.Data["password"]))
	}

	stats := sync.Stats()
	if stats.SecretsCreated != 1 {
		t.Errorf("expected 1 created, got %d", stats.SecretsCreated)
	}
	if stats.SecretsUpdated != 1 {
		t.Errorf("expected 1 updated, got %d", stats.SecretsUpdated)
	}
}

func TestSecretSyncKeyMapping(t *testing.T) {
	source := newMockSecretSource()
	source.addSecret("secrets/db", map[string]interface{}{
		"user": "admin",
		"pass": "secret",
	})

	client := NewMockKubernetesClient()
	config := &SyncConfig{
		Namespace: "default",
		Secrets: []SyncSecretSpec{
			{
				SourcePath: "secrets/db",
				DestName:   "db-creds",
				KeyMapping: map[string]string{
					"user": "DB_USER",
					"pass": "DB_PASS",
				},
			},
		},
	}

	sync, _ := NewSecretSync(config, source, client)
	ctx := context.Background()

	if err := sync.SyncOnce(ctx); err != nil {
		t.Fatalf("sync error: %v", err)
	}

	secret, _ := client.GetSecret(ctx, "default", "db-creds")
	if _, ok := secret.Data["DB_USER"]; !ok {
		t.Error("expected DB_USER key")
	}
	if _, ok := secret.Data["DB_PASS"]; !ok {
		t.Error("expected DB_PASS key")
	}
}

func TestSecretSyncStop(t *testing.T) {
	source := newMockSecretSource()
	client := NewMockKubernetesClient()
	sync, _ := NewSecretSync(nil, source, client)

	// Stop without running should not error
	if err := sync.Stop(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockKubernetesClient(t *testing.T) {
	client := NewMockKubernetesClient()
	ctx := context.Background()

	t.Run("create and get", func(t *testing.T) {
		err := client.CreateSecret(ctx, "ns", "test", map[string][]byte{
			"key": []byte("value"),
		}, "Opaque", nil, nil)
		if err != nil {
			t.Fatalf("create error: %v", err)
		}

		secret, err := client.GetSecret(ctx, "ns", "test")
		if err != nil {
			t.Fatalf("get error: %v", err)
		}
		if string(secret.Data["key"]) != "value" {
			t.Errorf("unexpected value: %s", string(secret.Data["key"]))
		}
	})

	t.Run("create duplicate", func(t *testing.T) {
		err := client.CreateSecret(ctx, "ns", "test", nil, "Opaque", nil, nil)
		if err == nil {
			t.Error("expected error for duplicate")
		}
	})

	t.Run("update", func(t *testing.T) {
		err := client.UpdateSecret(ctx, "ns", "test", map[string][]byte{
			"key": []byte("updated"),
		}, nil, nil)
		if err != nil {
			t.Fatalf("update error: %v", err)
		}

		secret, _ := client.GetSecret(ctx, "ns", "test")
		if string(secret.Data["key"]) != "updated" {
			t.Errorf("expected updated, got %s", string(secret.Data["key"]))
		}
	})

	t.Run("update non-existent", func(t *testing.T) {
		err := client.UpdateSecret(ctx, "ns", "nonexistent", nil, nil, nil)
		if err == nil {
			t.Error("expected error for non-existent")
		}
	})

	t.Run("list with labels", func(t *testing.T) {
		client.CreateSecret(ctx, "ns", "labeled", nil, "Opaque",
			map[string]string{"app": "test"}, nil)

		secrets, err := client.ListSecrets(ctx, "ns", map[string]string{"app": "test"})
		if err != nil {
			t.Fatalf("list error: %v", err)
		}
		found := false
		for _, s := range secrets {
			if s.Name == "labeled" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find labeled secret")
		}
	})

	t.Run("delete", func(t *testing.T) {
		err := client.DeleteSecret(ctx, "ns", "test")
		if err != nil {
			t.Fatalf("delete error: %v", err)
		}

		secret, _ := client.GetSecret(ctx, "ns", "test")
		if secret != nil {
			t.Error("expected nil after delete")
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		err := client.DeleteSecret(ctx, "ns", "nonexistent")
		if err == nil {
			t.Error("expected error for non-existent")
		}
	})
}

// =============================================================================
// Webhook Tests
// =============================================================================

func TestNewMutatingWebhook(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewMutatingWebhook(nil)
		if err == nil {
			t.Error("expected error for missing cert/key")
		}
	})

	t.Run("missing cert", func(t *testing.T) {
		config := &WebhookConfig{
			KeyFile: "/path/to/key",
		}
		_, err := NewMutatingWebhook(config)
		if err == nil {
			t.Error("expected error for missing cert")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		config := &WebhookConfig{
			CertFile: "/path/to/cert",
			KeyFile:  "/path/to/key",
			Port:     8443,
		}
		wh, err := NewMutatingWebhook(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wh == nil {
			t.Fatal("expected non-nil webhook")
		}
	})
}

func TestWebhookMutateEndpoint(t *testing.T) {
	config := DefaultWebhookConfig()
	config.CertFile = "test.crt"
	config.KeyFile = "test.key"

	wh, _ := NewMutatingWebhook(config)

	// Test health endpoint
	t.Run("health check", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		rr := httptest.NewRecorder()
		wh.handleHealth(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	// Test ready endpoint
	t.Run("ready check", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ready", nil)
		rr := httptest.NewRecorder()
		wh.handleReady(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestWebhookMutate(t *testing.T) {
	config := DefaultWebhookConfig()
	config.CertFile = "test.crt"
	config.KeyFile = "test.key"

	wh, _ := NewMutatingWebhook(config)

	t.Run("non-pod resource", func(t *testing.T) {
		req := &AdmissionRequest{
			UID: "test-uid",
			Kind: GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
		}
		resp := wh.mutate(req)
		if !resp.Allowed {
			t.Error("expected allowed for non-pod")
		}
		if resp.Patch != nil {
			t.Error("expected no patches for non-pod")
		}
	})

	t.Run("pod without annotation", func(t *testing.T) {
		pod := PodSpec{
			Metadata: PodMetadata{
				Name: "test-pod",
			},
			Spec: PodSpecInner{},
		}
		podJSON, _ := json.Marshal(pod)

		req := &AdmissionRequest{
			UID: "test-uid",
			Kind: GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			Object: podJSON,
		}
		resp := wh.mutate(req)
		if !resp.Allowed {
			t.Error("expected allowed")
		}
		if resp.Patch != nil {
			t.Error("expected no patches without annotation")
		}
	})

	t.Run("pod with injection enabled", func(t *testing.T) {
		pod := PodSpec{
			Metadata: PodMetadata{
				Name: "test-pod",
				Annotations: map[string]string{
					AnnotationInject:  "true",
					AnnotationSecrets: `[{"name":"db","secret_path":"secrets/db"}]`,
				},
			},
			Spec: PodSpecInner{
				Containers: []Container{
					{Name: "app", Image: "app:latest"},
				},
			},
		}
		podJSON, _ := json.Marshal(pod)

		req := &AdmissionRequest{
			UID: "test-uid",
			Kind: GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			Object: podJSON,
		}
		resp := wh.mutate(req)
		if !resp.Allowed {
			t.Error("expected allowed")
		}
		if resp.Patch == nil {
			t.Fatal("expected patches")
		}

		var patches []JSONPatch
		if err := json.Unmarshal(resp.Patch, &patches); err != nil {
			t.Fatalf("failed to parse patches: %v", err)
		}

		// Should have patches for labels, annotations, volumes, containers, mounts
		if len(patches) < 4 {
			t.Errorf("expected at least 4 patches, got %d", len(patches))
		}
	})

	t.Run("already injected pod", func(t *testing.T) {
		pod := PodSpec{
			Metadata: PodMetadata{
				Name: "test-pod",
				Labels: map[string]string{
					LabelInjected: "true",
				},
				Annotations: map[string]string{
					AnnotationInject: "true",
				},
			},
		}
		podJSON, _ := json.Marshal(pod)

		req := &AdmissionRequest{
			UID: "test-uid",
			Kind: GroupVersionKind{
				Kind: "Pod",
			},
			Object: podJSON,
		}
		resp := wh.mutate(req)
		if !resp.Allowed {
			t.Error("expected allowed")
		}
		if resp.Patch != nil {
			t.Error("expected no patches for already injected")
		}
	})
}

func TestBuildWebhookConfiguration(t *testing.T) {
	config := &WebhookConfig{
		Port:          8443,
		FailurePolicy: "Fail",
		CABundle:      []byte("ca-bundle"),
	}

	whConfig := BuildWebhookConfiguration(config, "webhook-svc", "keystone-system")

	if whConfig.APIVersion != "admissionregistration.k8s.io/v1" {
		t.Errorf("unexpected apiVersion: %s", whConfig.APIVersion)
	}
	if len(whConfig.Webhooks) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(whConfig.Webhooks))
	}

	webhook := whConfig.Webhooks[0]
	if webhook.FailurePolicy != "Fail" {
		t.Errorf("unexpected failure policy: %s", webhook.FailurePolicy)
	}
	if webhook.ClientConfig.Service.Name != "webhook-svc" {
		t.Errorf("unexpected service name: %s", webhook.ClientConfig.Service.Name)
	}
	if webhook.ClientConfig.Service.Namespace != "keystone-system" {
		t.Errorf("unexpected namespace: %s", webhook.ClientConfig.Service.Namespace)
	}
}

func TestEscapeJSONPointer(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with/slash", "with~1slash"},
		{"with~tilde", "with~0tilde"},
		{"both~/mixed", "both~0~1mixed"},
		{"secrets.keystone.io/synced", "secrets.keystone.io~1synced"},
	}

	for _, tc := range tests {
		result := escapeJSONPointer(tc.input)
		if result != tc.expected {
			t.Errorf("escapeJSONPointer(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// =============================================================================
// CSI Driver Tests
// =============================================================================

func TestNewCSIDriver(t *testing.T) {
	source := newMockSecretSource()

	t.Run("nil source", func(t *testing.T) {
		_, err := NewCSIDriver(nil, nil)
		if err == nil {
			t.Error("expected error for nil source")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		driver, err := NewCSIDriver(nil, source)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if driver == nil {
			t.Fatal("expected non-nil driver")
		}
	})

	t.Run("default node id", func(t *testing.T) {
		config := &CSIDriverConfig{}
		driver, _ := NewCSIDriver(config, source)
		if driver.config.NodeID == "" {
			t.Error("expected non-empty node ID")
		}
	})
}

func TestCSIDriverPluginInfo(t *testing.T) {
	source := newMockSecretSource()
	driver, _ := NewCSIDriver(nil, source)

	info := driver.GetPluginInfo()
	if info["name"] != DefaultCSIDriverConfig().DriverName {
		t.Errorf("unexpected driver name: %s", info["name"])
	}
	if info["version"] == "" {
		t.Error("expected non-empty version")
	}
}

func TestCSIDriverProbe(t *testing.T) {
	source := newMockSecretSource()
	driver, _ := NewCSIDriver(nil, source)

	ready, err := driver.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe error: %v", err)
	}
	if !ready {
		t.Error("expected ready")
	}
}

func TestCSIDriverNodeOperations(t *testing.T) {
	source := newMockSecretSource()
	source.addSecret("secrets/api", map[string]interface{}{
		"key": "api-key-value",
	})

	driver, _ := NewCSIDriver(nil, source)
	ctx := context.Background()
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "secrets")

	volumeContext := map[string]string{
		"secrets": `[{"secretPath":"secrets/api","secretKey":"key","fileName":"api.txt"}]`,
	}

	t.Run("publish volume", func(t *testing.T) {
		err := driver.NodePublishVolume(ctx, "vol-1", targetPath, volumeContext, true)
		if err != nil {
			t.Fatalf("publish error: %v", err)
		}

		// Verify file was created
		content, err := os.ReadFile(filepath.Join(targetPath, "api.txt"))
		if err != nil {
			t.Fatalf("read error: %v", err)
		}
		if string(content) != "api-key-value" {
			t.Errorf("unexpected content: %s", string(content))
		}
	})

	t.Run("publish duplicate", func(t *testing.T) {
		// Should be idempotent
		err := driver.NodePublishVolume(ctx, "vol-1", targetPath, volumeContext, true)
		if err != nil {
			t.Errorf("duplicate publish should succeed: %v", err)
		}
	})

	t.Run("list volumes", func(t *testing.T) {
		volumes, err := driver.ListVolumes(ctx)
		if err != nil {
			t.Fatalf("list error: %v", err)
		}
		if len(volumes) != 1 {
			t.Errorf("expected 1 volume, got %d", len(volumes))
		}
	})

	t.Run("unpublish volume", func(t *testing.T) {
		err := driver.NodeUnpublishVolume(ctx, "vol-1", targetPath)
		if err != nil {
			t.Fatalf("unpublish error: %v", err)
		}

		// Verify directory removed
		if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
			t.Error("expected target path to be removed")
		}
	})

	t.Run("unpublish non-existent", func(t *testing.T) {
		err := driver.NodeUnpublishVolume(ctx, "vol-nonexistent", "/nonexistent")
		if err != nil {
			t.Errorf("unpublish non-existent should succeed: %v", err)
		}
	})
}

func TestCSIDriverRefreshSecrets(t *testing.T) {
	source := newMockSecretSource()
	source.addSecret("secrets/db", map[string]interface{}{
		"password": "original",
	})

	driver, _ := NewCSIDriver(nil, source)
	ctx := context.Background()
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "secrets")

	volumeContext := map[string]string{
		"secrets": `[{"secretPath":"secrets/db","secretKey":"password","fileName":"db-pass"}]`,
	}

	// Publish initial
	driver.NodePublishVolume(ctx, "vol-1", targetPath, volumeContext, true)

	// Update source
	source.secrets["secrets/db"].Data["password"] = "updated"

	// Refresh
	if err := driver.RefreshSecrets(ctx); err != nil {
		t.Fatalf("refresh error: %v", err)
	}

	// Verify updated
	content, _ := os.ReadFile(filepath.Join(targetPath, "db-pass"))
	if string(content) != "updated" {
		t.Errorf("expected updated, got %s", string(content))
	}
}

func TestCSIVolumeSpecBuilder(t *testing.T) {
	t.Run("build volume", func(t *testing.T) {
		builder := NewCSIVolumeSpecBuilder("").
			WithSecrets([]SecretMount{
				{SecretPath: "secrets/db", FileName: "db.json"},
			}).
			WithReadOnly(true)

		vol := builder.Build("secret-vol")

		if vol["name"] != "secret-vol" {
			t.Errorf("unexpected name: %v", vol["name"])
		}

		csi, ok := vol["csi"].(map[string]interface{})
		if !ok {
			t.Fatal("expected csi section")
		}
		if csi["driver"] != DefaultCSIDriverConfig().DriverName {
			t.Errorf("unexpected driver: %v", csi["driver"])
		}
	})

	t.Run("build volume mount", func(t *testing.T) {
		builder := NewCSIVolumeSpecBuilder("")
		mount := builder.BuildVolumeMount("secret-vol", "/app/secrets")

		if mount["name"] != "secret-vol" {
			t.Errorf("unexpected name: %v", mount["name"])
		}
		if mount["mountPath"] != "/app/secrets" {
			t.Errorf("unexpected mount path: %v", mount["mountPath"])
		}
	})
}

// =============================================================================
// Types Tests
// =============================================================================

func TestDefaultConfigs(t *testing.T) {
	t.Run("default injector config", func(t *testing.T) {
		config := DefaultInjectorConfig()
		if config.Mode != InjectionModeSidecar {
			t.Errorf("expected sidecar mode, got %s", config.Mode)
		}
		if config.SecretVolumePath != "/secrets" {
			t.Errorf("expected /secrets, got %s", config.SecretVolumePath)
		}
		if config.Resources == nil {
			t.Error("expected default resources")
		}
	})

	t.Run("default webhook config", func(t *testing.T) {
		config := DefaultWebhookConfig()
		if !config.Enabled {
			t.Error("expected enabled by default")
		}
		if config.Port != 8443 {
			t.Errorf("expected port 8443, got %d", config.Port)
		}
		if config.FailurePolicy != "Ignore" {
			t.Errorf("expected Ignore policy, got %s", config.FailurePolicy)
		}
	})

	t.Run("default sync config", func(t *testing.T) {
		config := DefaultSyncConfig()
		if !config.Enabled {
			t.Error("expected enabled by default")
		}
		if config.SyncInterval != 60*time.Second {
			t.Errorf("expected 60s interval, got %v", config.SyncInterval)
		}
		if config.DeleteOrphans {
			t.Error("expected delete orphans false by default")
		}
	})
}

func TestParseFileMode(t *testing.T) {
	tests := []struct {
		input    string
		expected os.FileMode
		wantErr  bool
	}{
		{"0600", 0600, false},
		{"0644", 0644, false},
		{"0755", 0755, false},
		{"600", 0600, false},
		{"invalid", 0, true},
	}

	for _, tc := range tests {
		mode, err := parseFileMode(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseFileMode(%q) expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseFileMode(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if mode != tc.expected {
			t.Errorf("parseFileMode(%q) = %o, want %o", tc.input, mode, tc.expected)
		}
	}
}

func TestEncodeDecodeSecretData(t *testing.T) {
	t.Run("encode", func(t *testing.T) {
		data := map[string]interface{}{
			"string": "value",
			"bytes":  []byte("binary"),
			"number": 42,
		}

		encoded, err := EncodeSecretData(data)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}

		if string(encoded["string"]) != "value" {
			t.Errorf("unexpected string value: %s", encoded["string"])
		}
		if string(encoded["bytes"]) != "binary" {
			t.Errorf("unexpected bytes value: %s", encoded["bytes"])
		}
	})

	t.Run("decode", func(t *testing.T) {
		data := map[string][]byte{
			"key": []byte("value"),
		}

		decoded := DecodeSecretData(data)
		if decoded["key"] != "value" {
			t.Errorf("unexpected decoded value: %s", decoded["key"])
		}
	})

	t.Run("base64 encode", func(t *testing.T) {
		data := map[string][]byte{
			"secret": []byte("hello"),
		}

		encoded := Base64EncodeData(data)
		if encoded["secret"] != "aGVsbG8=" {
			t.Errorf("unexpected base64: %s", encoded["secret"])
		}
	})
}
