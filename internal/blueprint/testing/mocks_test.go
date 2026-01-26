package testing

import (
	"context"
	"testing"
)

func TestNewMockRegistry(t *testing.T) {
	registry := NewMockRegistry()
	if registry == nil {
		t.Fatal("NewMockRegistry returned nil")
	}
	if registry.commands == nil {
		t.Error("commands map should be initialized")
	}
	if registry.files == nil {
		t.Error("files map should be initialized")
	}
	if registry.http == nil {
		t.Error("http map should be initialized")
	}
	if registry.packages == nil {
		t.Error("packages map should be initialized")
	}
	if registry.services == nil {
		t.Error("services map should be initialized")
	}
	if registry.users == nil {
		t.Error("users map should be initialized")
	}
	if registry.groups == nil {
		t.Error("groups map should be initialized")
	}
}

func TestMockRegistry_RegisterCommand(t *testing.T) {
	registry := NewMockRegistry()

	mock := &CommandMockHandler{
		Stdout:   "hello world",
		Stderr:   "",
		ExitCode: 0,
	}

	err := registry.RegisterCommand("echo.*", mock)
	if err != nil {
		t.Fatalf("RegisterCommand failed: %v", err)
	}

	// Verify mock is retrievable
	retrieved := registry.GetCommandMock("echo hello")
	if retrieved == nil {
		t.Error("GetCommandMock returned nil for matching command")
	}
	if retrieved.Stdout != "hello world" {
		t.Errorf("Stdout = %q, want %q", retrieved.Stdout, "hello world")
	}

	// Non-matching command should return nil
	retrieved = registry.GetCommandMock("ls -la")
	if retrieved != nil {
		t.Error("GetCommandMock should return nil for non-matching command")
	}
}

func TestMockRegistry_RegisterCommand_InvalidPattern(t *testing.T) {
	registry := NewMockRegistry()

	mock := &CommandMockHandler{}
	err := registry.RegisterCommand("[invalid", mock)
	if err == nil {
		t.Error("RegisterCommand should fail for invalid regex pattern")
	}
}

func TestMockRegistry_RegisterFile(t *testing.T) {
	registry := NewMockRegistry()

	mock := &FileMockHandler{
		Content: []byte("file content"),
		Mode:    "0644",
		Owner:   "root",
		Group:   "root",
		Exists:  true,
		IsDir:   false,
	}

	registry.RegisterFile("/etc/test.conf", mock)

	retrieved := registry.GetFileMock("/etc/test.conf")
	if retrieved == nil {
		t.Fatal("GetFileMock returned nil")
	}
	if string(retrieved.Content) != "file content" {
		t.Errorf("Content = %q, want %q", string(retrieved.Content), "file content")
	}
	if retrieved.Mode != "0644" {
		t.Errorf("Mode = %q, want %q", retrieved.Mode, "0644")
	}
	if !retrieved.Exists {
		t.Error("Exists should be true")
	}

	// Non-existent path should return nil
	retrieved = registry.GetFileMock("/other/path")
	if retrieved != nil {
		t.Error("GetFileMock should return nil for unregistered path")
	}
}

func TestMockRegistry_RegisterHTTP(t *testing.T) {
	registry := NewMockRegistry()

	mock := &HTTPMockHandler{
		StatusCode: 200,
		Body:       []byte(`{"status": "ok"}`),
		Headers:    map[string]string{"Content-Type": "application/json"},
	}

	err := registry.RegisterHTTP("https://api.example.com/.*", mock)
	if err != nil {
		t.Fatalf("RegisterHTTP failed: %v", err)
	}

	retrieved := registry.GetHTTPMock("https://api.example.com/v1/health")
	if retrieved == nil {
		t.Fatal("GetHTTPMock returned nil for matching URL")
	}
	if retrieved.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", retrieved.StatusCode)
	}

	// Non-matching URL should return nil
	retrieved = registry.GetHTTPMock("https://other.example.com/api")
	if retrieved != nil {
		t.Error("GetHTTPMock should return nil for non-matching URL")
	}
}

func TestMockRegistry_RegisterHTTP_InvalidPattern(t *testing.T) {
	registry := NewMockRegistry()

	mock := &HTTPMockHandler{}
	err := registry.RegisterHTTP("[invalid", mock)
	if err == nil {
		t.Error("RegisterHTTP should fail for invalid regex pattern")
	}
}

func TestMockRegistry_RegisterPackage(t *testing.T) {
	registry := NewMockRegistry()

	mock := &PackageMockHandler{
		Installed:         true,
		Version:           "1.2.3",
		AvailableVersions: []string{"1.2.3", "1.2.4", "2.0.0"},
	}

	registry.RegisterPackage("nginx", mock)

	retrieved := registry.GetPackageMock("nginx")
	if retrieved == nil {
		t.Fatal("GetPackageMock returned nil")
	}
	if !retrieved.Installed {
		t.Error("Installed should be true")
	}
	if retrieved.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", retrieved.Version, "1.2.3")
	}
	if len(retrieved.AvailableVersions) != 3 {
		t.Errorf("len(AvailableVersions) = %d, want 3", len(retrieved.AvailableVersions))
	}

	// Non-existent package should return nil
	retrieved = registry.GetPackageMock("apache")
	if retrieved != nil {
		t.Error("GetPackageMock should return nil for unregistered package")
	}
}

func TestMockRegistry_RegisterService(t *testing.T) {
	registry := NewMockRegistry()

	mock := &ServiceMockHandler{
		Running: true,
		Enabled: true,
	}

	registry.RegisterService("nginx", mock)

	retrieved := registry.GetServiceMock("nginx")
	if retrieved == nil {
		t.Fatal("GetServiceMock returned nil")
	}
	if !retrieved.Running {
		t.Error("Running should be true")
	}
	if !retrieved.Enabled {
		t.Error("Enabled should be true")
	}

	// Non-existent service should return nil
	retrieved = registry.GetServiceMock("apache")
	if retrieved != nil {
		t.Error("GetServiceMock should return nil for unregistered service")
	}
}

func TestMockRegistry_RegisterUser(t *testing.T) {
	registry := NewMockRegistry()

	mock := &UserMockHandler{
		Exists: true,
		UID:    1000,
		GID:    1000,
		Home:   "/home/testuser",
		Shell:  "/bin/bash",
	}

	registry.RegisterUser("testuser", mock)

	retrieved := registry.GetUserMock("testuser")
	if retrieved == nil {
		t.Fatal("GetUserMock returned nil")
	}
	if !retrieved.Exists {
		t.Error("Exists should be true")
	}
	if retrieved.UID != 1000 {
		t.Errorf("UID = %d, want 1000", retrieved.UID)
	}
	if retrieved.Home != "/home/testuser" {
		t.Errorf("Home = %q, want %q", retrieved.Home, "/home/testuser")
	}

	// Non-existent user should return nil
	retrieved = registry.GetUserMock("otheruser")
	if retrieved != nil {
		t.Error("GetUserMock should return nil for unregistered user")
	}
}

func TestMockRegistry_RegisterGroup(t *testing.T) {
	registry := NewMockRegistry()

	mock := &GroupMockHandler{
		Exists:  true,
		GID:     1000,
		Members: []string{"user1", "user2"},
	}

	registry.RegisterGroup("testgroup", mock)

	retrieved := registry.GetGroupMock("testgroup")
	if retrieved == nil {
		t.Fatal("GetGroupMock returned nil")
	}
	if !retrieved.Exists {
		t.Error("Exists should be true")
	}
	if retrieved.GID != 1000 {
		t.Errorf("GID = %d, want 1000", retrieved.GID)
	}
	if len(retrieved.Members) != 2 {
		t.Errorf("len(Members) = %d, want 2", len(retrieved.Members))
	}

	// Non-existent group should return nil
	retrieved = registry.GetGroupMock("othergroup")
	if retrieved != nil {
		t.Error("GetGroupMock should return nil for unregistered group")
	}
}

func TestMockRegistry_Clear(t *testing.T) {
	registry := NewMockRegistry()

	// Register various mocks
	registry.RegisterFile("/test", &FileMockHandler{})
	registry.RegisterPackage("pkg", &PackageMockHandler{})
	registry.RegisterService("svc", &ServiceMockHandler{})
	registry.RegisterUser("user", &UserMockHandler{})
	registry.RegisterGroup("group", &GroupMockHandler{})

	// Clear all
	registry.Clear()

	// Verify all are cleared
	if registry.GetFileMock("/test") != nil {
		t.Error("Files should be cleared")
	}
	if registry.GetPackageMock("pkg") != nil {
		t.Error("Packages should be cleared")
	}
	if registry.GetServiceMock("svc") != nil {
		t.Error("Services should be cleared")
	}
	if registry.GetUserMock("user") != nil {
		t.Error("Users should be cleared")
	}
	if registry.GetGroupMock("group") != nil {
		t.Error("Groups should be cleared")
	}
}

func TestNewMockBuilder(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)
	if builder == nil {
		t.Fatal("NewMockBuilder returned nil")
	}
	if builder.registry != registry {
		t.Error("Builder should reference the provided registry")
	}
}

func TestMockBuilder_ApplyMocks_Command(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "command",
			Command: &CommandMock{
				Pattern:  "echo.*",
				Stdout:   "mocked output",
				ExitCode: 0,
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks failed: %v", err)
	}

	mock := registry.GetCommandMock("echo hello")
	if mock == nil {
		t.Fatal("Command mock should be registered")
	}
	if mock.Stdout != "mocked output" {
		t.Errorf("Stdout = %q, want %q", mock.Stdout, "mocked output")
	}
}

func TestMockBuilder_ApplyMocks_File(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "file",
			File: &FileMock{
				Path:    "/etc/config.conf",
				Content: "config content",
				Mode:    "0644",
				Exists:  true,
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks failed: %v", err)
	}

	mock := registry.GetFileMock("/etc/config.conf")
	if mock == nil {
		t.Fatal("File mock should be registered")
	}
	if string(mock.Content) != "config content" {
		t.Errorf("Content = %q, want %q", string(mock.Content), "config content")
	}
}

func TestMockBuilder_ApplyMocks_HTTP(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "http",
			HTTP: &HTTPMock{
				URL:        "https://api.test.com/.*",
				StatusCode: 201,
				Body:       `{"created": true}`,
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks failed: %v", err)
	}

	mock := registry.GetHTTPMock("https://api.test.com/resource")
	if mock == nil {
		t.Fatal("HTTP mock should be registered")
	}
	if mock.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", mock.StatusCode)
	}
}

func TestMockBuilder_ApplyMocks_Package(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "package",
			Package: &PackageMock{
				Name:              "nginx",
				Installed:        true,
				Version:          "1.18.0",
				AvailableVersions: []string{"1.18.0", "1.19.0"},
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks failed: %v", err)
	}

	mock := registry.GetPackageMock("nginx")
	if mock == nil {
		t.Fatal("Package mock should be registered")
	}
	if mock.Version != "1.18.0" {
		t.Errorf("Version = %q, want %q", mock.Version, "1.18.0")
	}
}

func TestMockBuilder_ApplyMocks_Service(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "service",
			Service: &ServiceMock{
				Name:    "nginx",
				Running: true,
				Enabled: true,
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks failed: %v", err)
	}

	mock := registry.GetServiceMock("nginx")
	if mock == nil {
		t.Fatal("Service mock should be registered")
	}
	if !mock.Running {
		t.Error("Running should be true")
	}
}

func TestMockBuilder_ApplyMocks_EmptyType(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{Type: ""},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks should not fail for empty type: %v", err)
	}
}

func TestMockBuilder_ApplyMocks_NilConfig(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{Type: "command", Command: nil},
		{Type: "file", File: nil},
		{Type: "http", HTTP: nil},
		{Type: "package", Package: nil},
		{Type: "service", Service: nil},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err != nil {
		t.Fatalf("ApplyMocks should not fail for nil configs: %v", err)
	}
}

func TestMockBuilder_ApplyMocks_InvalidCommandPattern(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "command",
			Command: &CommandMock{
				Pattern: "[invalid",
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err == nil {
		t.Error("ApplyMocks should fail for invalid command pattern")
	}
}

func TestMockBuilder_ApplyMocks_InvalidHTTPPattern(t *testing.T) {
	registry := NewMockRegistry()
	builder := NewMockBuilder(registry)

	mocks := []MockConfig{
		{
			Type: "http",
			HTTP: &HTTPMock{
				URL: "[invalid",
			},
		},
	}

	err := builder.ApplyMocks(context.Background(), mocks)
	if err == nil {
		t.Error("ApplyMocks should fail for invalid HTTP URL pattern")
	}
}
