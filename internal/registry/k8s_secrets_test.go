package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewK8sSecretCredentialProvider(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	provider := NewK8sSecretCredentialProvider(clientset)

	if provider == nil {
		t.Fatal("Expected provider to be non-nil")
	}
	if provider.clientset == nil {
		t.Error("Expected clientset to be non-nil")
	}
}

func TestK8sSecretCredentialProvider_GetFromSecret_DockerConfigJson(t *testing.T) {
	// Create a dockerconfigjson secret
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"gcr.io": map[string]string{
				"username": "testuser",
				"password": "testpass",
				"email":    "test@example.com",
			},
		},
	}
	configData, _ := json.Marshal(dockerConfig)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configData,
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	creds, err := provider.GetFromSecret(ctx, "default", "my-secret")
	if err != nil {
		t.Fatalf("GetFromSecret error = %v", err)
	}

	if len(creds) != 1 {
		t.Fatalf("Expected 1 credential, got %d", len(creds))
	}

	cred := creds[0]
	if cred.Username != "testuser" {
		t.Errorf("Username = %q, want %q", cred.Username, "testuser")
	}
	if cred.Password != "testpass" {
		t.Errorf("Password = %q, want %q", cred.Password, "testpass")
	}
	if cred.Registry != "gcr.io" {
		t.Errorf("Registry = %q, want %q", cred.Registry, "gcr.io")
	}
}

func TestK8sSecretCredentialProvider_GetFromSecret_DockerConfigJson_AuthField(t *testing.T) {
	// Create a secret using the auth field (base64 encoded username:password)
	auth := base64.StdEncoding.EncodeToString([]byte("myuser:mypassword"))
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"docker.io": map[string]string{
				"auth": auth,
			},
		},
	}
	configData, _ := json.Marshal(dockerConfig)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auth-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configData,
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	creds, err := provider.GetFromSecret(ctx, "default", "auth-secret")
	if err != nil {
		t.Fatalf("GetFromSecret error = %v", err)
	}

	if len(creds) != 1 {
		t.Fatalf("Expected 1 credential, got %d", len(creds))
	}

	cred := creds[0]
	if cred.Username != "myuser" {
		t.Errorf("Username = %q, want %q", cred.Username, "myuser")
	}
	if cred.Password != "mypassword" {
		t.Errorf("Password = %q, want %q", cred.Password, "mypassword")
	}
}

func TestK8sSecretCredentialProvider_GetFromSecret_Dockercfg(t *testing.T) {
	// Create a legacy dockercfg secret
	dockerCfg := map[string]interface{}{
		"quay.io": map[string]string{
			"username": "quayuser",
			"password": "quaypass",
		},
	}
	cfgData, _ := json.Marshal(dockerCfg)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockercfg,
		Data: map[string][]byte{
			corev1.DockerConfigKey: cfgData,
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	creds, err := provider.GetFromSecret(ctx, "default", "legacy-secret")
	if err != nil {
		t.Fatalf("GetFromSecret error = %v", err)
	}

	if len(creds) != 1 {
		t.Fatalf("Expected 1 credential, got %d", len(creds))
	}

	cred := creds[0]
	if cred.Username != "quayuser" {
		t.Errorf("Username = %q, want %q", cred.Username, "quayuser")
	}
}

func TestK8sSecretCredentialProvider_GetFromSecret_MultipleRegistries(t *testing.T) {
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"gcr.io": map[string]string{
				"username": "gcr-user",
				"password": "gcr-pass",
			},
			"docker.io": map[string]string{
				"username": "docker-user",
				"password": "docker-pass",
			},
		},
	}
	configData, _ := json.Marshal(dockerConfig)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configData,
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	creds, err := provider.GetFromSecret(ctx, "default", "multi-secret")
	if err != nil {
		t.Fatalf("GetFromSecret error = %v", err)
	}

	if len(creds) != 2 {
		t.Fatalf("Expected 2 credentials, got %d", len(creds))
	}
}

func TestK8sSecretCredentialProvider_GetForRegistry(t *testing.T) {
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"gcr.io": map[string]string{
				"username": "gcr-user",
				"password": "gcr-pass",
			},
			"docker.io": map[string]string{
				"username": "docker-user",
				"password": "docker-pass",
			},
		},
	}
	configData, _ := json.Marshal(dockerConfig)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configData,
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()

	// Test getting specific registry
	cred, err := provider.GetForRegistry(ctx, "default", "multi-secret", "gcr.io")
	if err != nil {
		t.Fatalf("GetForRegistry error = %v", err)
	}
	if cred.Username != "gcr-user" {
		t.Errorf("Username = %q, want %q", cred.Username, "gcr-user")
	}

	// Test registry not found
	_, err = provider.GetForRegistry(ctx, "default", "multi-secret", "unknown.registry.io")
	if err == nil {
		t.Error("Expected error for unknown registry")
	}
}

func TestK8sSecretCredentialProvider_GetFromServiceAccount(t *testing.T) {
	// Create a secret
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			"gcr.io": map[string]string{
				"username": "sa-user",
				"password": "sa-pass",
			},
		},
	}
	configData, _ := json.Marshal(dockerConfig)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configData,
		},
	}

	// Create a service account with imagePullSecrets
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-sa",
			Namespace: "default",
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "pull-secret"},
		},
	}

	clientset := fake.NewSimpleClientset(secret, sa)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	creds, err := provider.GetFromServiceAccount(ctx, "default", "my-sa")
	if err != nil {
		t.Fatalf("GetFromServiceAccount error = %v", err)
	}

	if len(creds) != 1 {
		t.Fatalf("Expected 1 credential, got %d", len(creds))
	}

	if creds[0].Username != "sa-user" {
		t.Errorf("Username = %q, want %q", creds[0].Username, "sa-user")
	}
}

func TestK8sSecretCredentialProvider_GetFromSecret_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	_, err := provider.GetFromSecret(ctx, "default", "nonexistent")

	if err == nil {
		t.Error("Expected error for nonexistent secret")
	}
}

func TestK8sSecretCredentialProvider_GetFromSecret_UnsupportedType(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opaque-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("user"),
			"password": []byte("pass"),
		},
	}

	clientset := fake.NewSimpleClientset(secret)
	provider := NewK8sSecretCredentialProvider(clientset)

	ctx := context.Background()
	_, err := provider.GetFromSecret(ctx, "default", "opaque-secret")

	if err == nil {
		t.Error("Expected error for unsupported secret type")
	}
}

func TestMatchesRegistry(t *testing.T) {
	tests := []struct {
		credRegistry   string
		targetRegistry string
		want           bool
	}{
		// Exact matches
		{"gcr.io", "gcr.io", true},
		{"docker.io", "docker.io", true},

		// Docker Hub aliases
		{"docker.io", "index.docker.io", true},
		{"index.docker.io/v1/", "docker.io", true},
		{"https://index.docker.io/v1/", "docker.io", true},

		// Prefix matches
		{"gcr.io", "gcr.io/project/image", true},

		// Non-matches
		{"gcr.io", "us.gcr.io", false},
		{"docker.io", "gcr.io", false},
	}

	for _, tt := range tests {
		name := tt.credRegistry + "_" + tt.targetRegistry
		t.Run(name, func(t *testing.T) {
			if got := matchesRegistry(tt.credRegistry, tt.targetRegistry); got != tt.want {
				t.Errorf("matchesRegistry(%q, %q) = %v, want %v", tt.credRegistry, tt.targetRegistry, got, tt.want)
			}
		})
	}
}

func TestNormalizeRegistryURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://gcr.io", "gcr.io"},
		{"http://gcr.io", "gcr.io"},
		{"gcr.io/", "gcr.io"},
		{"gcr.io/v1", "gcr.io"},
		{"gcr.io/v2/", "gcr.io"},
		{"DOCKER.IO", "docker.io"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeRegistryURL(tt.input); got != tt.want {
				t.Errorf("normalizeRegistryURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCreateDockerConfigSecret(t *testing.T) {
	creds := []*Credential{
		{
			Registry: "gcr.io",
			Username: "user1",
			Password: "pass1",
		},
		{
			Registry: "docker.io",
			Username: "user2",
			Password: "pass2",
		},
	}

	secret := CreateDockerConfigSecret("my-secret", "default", creds)

	if secret.Name != "my-secret" {
		t.Errorf("Name = %q, want %q", secret.Name, "my-secret")
	}
	if secret.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", secret.Namespace, "default")
	}
	if secret.Type != corev1.SecretTypeDockerConfigJson {
		t.Errorf("Type = %v, want %v", secret.Type, corev1.SecretTypeDockerConfigJson)
	}
	if _, ok := secret.Data[corev1.DockerConfigJsonKey]; !ok {
		t.Error("Expected .dockerconfigjson key in data")
	}
}
