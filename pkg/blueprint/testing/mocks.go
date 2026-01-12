package testing

import (
	"context"
	"fmt"
	"regexp"
	"sync"
)

// MockRegistry manages mock implementations for testing.
type MockRegistry struct {
	mu sync.RWMutex

	// commands maps command patterns to mock outputs
	commands map[string]*CommandMockHandler

	// files maps file paths to mock contents
	files map[string]*FileMockHandler

	// http maps URL patterns to mock responses
	http map[string]*HTTPMockHandler

	// packages maps package names to mock states
	packages map[string]*PackageMockHandler

	// services maps service names to mock states
	services map[string]*ServiceMockHandler

	// users maps user names to mock states
	users map[string]*UserMockHandler

	// groups maps group names to mock states
	groups map[string]*GroupMockHandler
}

// NewMockRegistry creates a new mock registry.
func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		commands: make(map[string]*CommandMockHandler),
		files:    make(map[string]*FileMockHandler),
		http:     make(map[string]*HTTPMockHandler),
		packages: make(map[string]*PackageMockHandler),
		services: make(map[string]*ServiceMockHandler),
		users:    make(map[string]*UserMockHandler),
		groups:   make(map[string]*GroupMockHandler),
	}
}

// CommandMockHandler handles command mocking.
type CommandMockHandler struct {
	// Pattern is a regex pattern to match commands
	Pattern *regexp.Regexp

	// Stdout is the mock stdout output
	Stdout string

	// Stderr is the mock stderr output
	Stderr string

	// ExitCode is the mock exit code
	ExitCode int

	// Calls tracks how many times this mock was called
	Calls int

	// Error simulates a command error
	Error error
}

// FileMockHandler handles file mocking.
type FileMockHandler struct {
	// Content is the mock file content
	Content []byte

	// Mode is the mock file mode (e.g., "0644")
	Mode string

	// Owner is the mock file owner
	Owner string

	// Group is the mock file group
	Group string

	// Exists indicates if the mock file exists
	Exists bool

	// IsDir indicates if this is a directory
	IsDir bool
}

// HTTPMockHandler handles HTTP mocking.
type HTTPMockHandler struct {
	// Pattern is a regex pattern to match URLs
	Pattern *regexp.Regexp

	// StatusCode is the mock HTTP status code
	StatusCode int

	// Body is the mock response body
	Body []byte

	// Headers are mock response headers
	Headers map[string]string

	// Error simulates an HTTP error
	Error error

	// Calls tracks how many times this mock was called
	Calls int
}

// PackageMockHandler handles package mocking.
type PackageMockHandler struct {
	// Installed indicates if the package is installed
	Installed bool

	// Version is the installed version
	Version string

	// AvailableVersions are the versions available from repository
	AvailableVersions []string
}

// ServiceMockHandler handles service mocking.
type ServiceMockHandler struct {
	// Running indicates if the service is running
	Running bool

	// Enabled indicates if the service is enabled
	Enabled bool
}

// UserMockHandler handles user mocking.
type UserMockHandler struct {
	// Exists indicates if the user exists
	Exists bool

	// UID is the user ID
	UID int

	// GID is the primary group ID
	GID int

	// Home is the home directory
	Home string

	// Shell is the login shell
	Shell string
}

// GroupMockHandler handles group mocking.
type GroupMockHandler struct {
	// Exists indicates if the group exists
	Exists bool

	// GID is the group ID
	GID int

	// Members are the group members
	Members []string
}

// RegisterCommand registers a command mock.
func (r *MockRegistry) RegisterCommand(pattern string, mock *CommandMockHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	mock.Pattern = re
	r.commands[pattern] = mock
	return nil
}

// GetCommandMock returns a mock handler for the given command.
func (r *MockRegistry) GetCommandMock(cmd string) *CommandMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, mock := range r.commands {
		if mock.Pattern.MatchString(cmd) {
			return mock
		}
	}
	return nil
}

// RegisterFile registers a file mock.
func (r *MockRegistry) RegisterFile(path string, mock *FileMockHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[path] = mock
}

// GetFileMock returns a mock handler for the given path.
func (r *MockRegistry) GetFileMock(path string) *FileMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.files[path]
}

// RegisterHTTP registers an HTTP mock.
func (r *MockRegistry) RegisterHTTP(pattern string, mock *HTTPMockHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	mock.Pattern = re
	r.http[pattern] = mock
	return nil
}

// GetHTTPMock returns a mock handler for the given URL.
func (r *MockRegistry) GetHTTPMock(url string) *HTTPMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, mock := range r.http {
		if mock.Pattern.MatchString(url) {
			return mock
		}
	}
	return nil
}

// RegisterPackage registers a package mock.
func (r *MockRegistry) RegisterPackage(name string, mock *PackageMockHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.packages[name] = mock
}

// GetPackageMock returns a mock handler for the given package.
func (r *MockRegistry) GetPackageMock(name string) *PackageMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.packages[name]
}

// RegisterService registers a service mock.
func (r *MockRegistry) RegisterService(name string, mock *ServiceMockHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[name] = mock
}

// GetServiceMock returns a mock handler for the given service.
func (r *MockRegistry) GetServiceMock(name string) *ServiceMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services[name]
}

// RegisterUser registers a user mock.
func (r *MockRegistry) RegisterUser(name string, mock *UserMockHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[name] = mock
}

// GetUserMock returns a mock handler for the given user.
func (r *MockRegistry) GetUserMock(name string) *UserMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.users[name]
}

// RegisterGroup registers a group mock.
func (r *MockRegistry) RegisterGroup(name string, mock *GroupMockHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[name] = mock
}

// GetGroupMock returns a mock handler for the given group.
func (r *MockRegistry) GetGroupMock(name string) *GroupMockHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.groups[name]
}

// Clear removes all registered mocks.
func (r *MockRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.commands = make(map[string]*CommandMockHandler)
	r.files = make(map[string]*FileMockHandler)
	r.http = make(map[string]*HTTPMockHandler)
	r.packages = make(map[string]*PackageMockHandler)
	r.services = make(map[string]*ServiceMockHandler)
	r.users = make(map[string]*UserMockHandler)
	r.groups = make(map[string]*GroupMockHandler)
}

// MockBuilder provides a fluent interface for building mocks from test configuration.
type MockBuilder struct {
	registry *MockRegistry
}

// NewMockBuilder creates a new mock builder.
func NewMockBuilder(registry *MockRegistry) *MockBuilder {
	return &MockBuilder{registry: registry}
}

// ApplyMocks applies mock configurations to the registry.
func (b *MockBuilder) ApplyMocks(ctx context.Context, mocks []MockConfig) error {
	for _, mock := range mocks {
		if mock.Type == "" {
			continue
		}

		switch mock.Type {
		case "command":
			if mock.Command == nil {
				continue
			}
			handler := &CommandMockHandler{
				Stdout:   mock.Command.Stdout,
				Stderr:   mock.Command.Stderr,
				ExitCode: mock.Command.ExitCode,
			}
			if err := b.registry.RegisterCommand(mock.Command.Pattern, handler); err != nil {
				return err
			}

		case "file":
			if mock.File == nil {
				continue
			}
			handler := &FileMockHandler{
				Content: []byte(mock.File.Content),
				Mode:    mock.File.Mode,
				Owner:   mock.File.Owner,
				Group:   mock.File.Group,
				Exists:  mock.File.Exists,
				IsDir:   mock.File.IsDir,
			}
			b.registry.RegisterFile(mock.File.Path, handler)

		case "http":
			if mock.HTTP == nil {
				continue
			}
			handler := &HTTPMockHandler{
				StatusCode: mock.HTTP.StatusCode,
				Body:       []byte(mock.HTTP.Body),
				Headers:    mock.HTTP.Headers,
			}
			if err := b.registry.RegisterHTTP(mock.HTTP.URL, handler); err != nil {
				return err
			}

		case "package":
			if mock.Package == nil {
				continue
			}
			handler := &PackageMockHandler{
				Installed:         mock.Package.Installed,
				Version:           mock.Package.Version,
				AvailableVersions: mock.Package.AvailableVersions,
			}
			b.registry.RegisterPackage(mock.Package.Name, handler)

		case "service":
			if mock.Service == nil {
				continue
			}
			handler := &ServiceMockHandler{
				Running: mock.Service.Running,
				Enabled: mock.Service.Enabled,
			}
			b.registry.RegisterService(mock.Service.Name, handler)
		}
	}

	return nil
}
