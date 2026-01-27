package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockPKCS11Interface implements PKCS11Interface for testing.
type MockPKCS11Interface struct {
	mu          sync.Mutex
	initialized bool
	sessions    map[SessionHandle]*mockSession
	objects     map[ObjectHandle]*mockObject
	loggedIn    bool
	nextSession SessionHandle
	nextObject  ObjectHandle

	failInitialize  bool
	failOpenSession bool
	failLogin       bool
	failEncrypt     bool
	failDecrypt     bool
	failSign        bool
}

type mockSession struct {
	handle SessionHandle
	slotID uint32
	flags  PKCS11SessionFlags
}

type mockObject struct {
	handle     ObjectHandle
	class      PKCS11ObjectClass
	keyType    PKCS11KeyType
	label      string
	keyMaterial []byte
}

func NewMockPKCS11Interface() *MockPKCS11Interface {
	return &MockPKCS11Interface{
		sessions: make(map[SessionHandle]*mockSession),
		objects:  make(map[ObjectHandle]*mockObject),
	}
}

func (m *MockPKCS11Interface) Initialize(ctx context.Context) error {
	if m.failInitialize {
		return errors.New("mock initialize failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initialized = true
	return nil
}

func (m *MockPKCS11Interface) Finalize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initialized = false
	return nil
}

func (m *MockPKCS11Interface) GetSlotList(ctx context.Context, tokenPresent bool) ([]uint32, error) {
	return []uint32{0, 1}, nil
}

func (m *MockPKCS11Interface) GetSlotInfo(ctx context.Context, slotID uint32) (*SlotInfo, error) {
	return &SlotInfo{
		SlotID:          slotID,
		SlotDescription: "Mock HSM Slot",
		ManufacturerID:  "Test",
		TokenPresent:    true,
	}, nil
}

func (m *MockPKCS11Interface) GetTokenInfo(ctx context.Context, slotID uint32) (*TokenInfo, error) {
	return &TokenInfo{
		Label:            "MockToken",
		ManufacturerID:   "Test",
		Model:            "Mock HSM",
		SerialNumber:     "12345",
		MaxSessionCount:  10,
		MaxRwSessionCount: 10,
	}, nil
}

func (m *MockPKCS11Interface) GetMechanismList(ctx context.Context, slotID uint32) ([]PKCS11Mechanism, error) {
	return []PKCS11Mechanism{
		CKM_AES_KEY_GEN,
		CKM_AES_GCM,
		CKM_AES_KEY_WRAP,
		CKM_RSA_PKCS,
		CKM_SHA256_RSA_PKCS,
	}, nil
}

func (m *MockPKCS11Interface) GetMechanismInfo(ctx context.Context, slotID uint32, mechanism PKCS11Mechanism) (*MechanismInfo, error) {
	return &MechanismInfo{
		Mechanism:  mechanism,
		MinKeySize: 128,
		MaxKeySize: 256,
	}, nil
}

func (m *MockPKCS11Interface) OpenSession(ctx context.Context, slotID uint32, flags PKCS11SessionFlags) (*Session, error) {
	if m.failOpenSession {
		return nil, errors.New("mock open session failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextSession++
	session := &mockSession{
		handle: m.nextSession,
		slotID: slotID,
		flags:  flags,
	}
	m.sessions[session.handle] = session

	return &Session{
		Handle:    session.handle,
		SlotID:    slotID,
		Flags:     uint32(flags),
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}, nil
}

func (m *MockPKCS11Interface) CloseSession(ctx context.Context, session SessionHandle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, session)
	return nil
}

func (m *MockPKCS11Interface) CloseAllSessions(ctx context.Context, slotID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for handle, session := range m.sessions {
		if session.slotID == slotID {
			delete(m.sessions, handle)
		}
	}
	return nil
}

func (m *MockPKCS11Interface) Login(ctx context.Context, session SessionHandle, userType PKCS11UserType, pin string) error {
	if m.failLogin {
		return errors.New("mock login failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loggedIn {
		return NewPKCS11Error(CKR_USER_ALREADY_LOGGED_IN)
	}
	if pin != "test-pin" {
		return NewPKCS11Error(CKR_PIN_INCORRECT)
	}
	m.loggedIn = true
	return nil
}

func (m *MockPKCS11Interface) Logout(ctx context.Context, session SessionHandle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loggedIn = false
	return nil
}

func (m *MockPKCS11Interface) GenerateKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, template map[string]interface{}) (ObjectHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextObject++
	key := make([]byte, 32)
	rand.Read(key)

	label, _ := template["CKA_LABEL"].(string)
	obj := &mockObject{
		handle:      m.nextObject,
		class:       CKO_SECRET_KEY,
		keyType:     CKK_AES,
		label:       label,
		keyMaterial: key,
	}
	m.objects[obj.handle] = obj

	return obj.handle, nil
}

func (m *MockPKCS11Interface) GenerateKeyPair(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, publicTemplate, privateTemplate map[string]interface{}) (publicKey, privateKey ObjectHandle, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextObject++
	pubObj := &mockObject{
		handle:  m.nextObject,
		class:   CKO_PUBLIC_KEY,
		keyType: CKK_RSA,
	}
	m.objects[pubObj.handle] = pubObj

	m.nextObject++
	privObj := &mockObject{
		handle:      m.nextObject,
		class:       CKO_PRIVATE_KEY,
		keyType:     CKK_RSA,
		keyMaterial: make([]byte, 256),
	}
	rand.Read(privObj.keyMaterial)
	m.objects[privObj.handle] = privObj

	return pubObj.handle, privObj.handle, nil
}

func (m *MockPKCS11Interface) FindObjectsInit(ctx context.Context, session SessionHandle, template map[string]interface{}) error {
	return nil
}

func (m *MockPKCS11Interface) FindObjects(ctx context.Context, session SessionHandle, maxObjects uint32) ([]ObjectHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	handles := make([]ObjectHandle, 0, len(m.objects))
	for handle := range m.objects {
		handles = append(handles, handle)
		if uint32(len(handles)) >= maxObjects {
			break
		}
	}
	return handles, nil
}

func (m *MockPKCS11Interface) FindObjectsFinal(ctx context.Context, session SessionHandle) error {
	return nil
}

func (m *MockPKCS11Interface) GetAttributeValue(ctx context.Context, session SessionHandle, object ObjectHandle, attributes []string) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	obj, ok := m.objects[object]
	if !ok {
		return nil, ErrKeyNotFound
	}

	attrs := map[string]interface{}{
		"CKA_CLASS":    obj.class,
		"CKA_KEY_TYPE": obj.keyType,
		"CKA_LABEL":    obj.label,
		"CKA_ENCRYPT":  true,
		"CKA_DECRYPT":  true,
		"CKA_SIGN":     true,
		"CKA_VERIFY":   true,
		"CKA_WRAP":     true,
		"CKA_UNWRAP":   true,
	}

	return attrs, nil
}

func (m *MockPKCS11Interface) Encrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	if m.failEncrypt {
		return nil, errors.New("mock encrypt failure")
	}
	m.mu.Lock()
	obj, ok := m.objects[key]
	m.mu.Unlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	block, err := aes.NewCipher(obj.keyMaterial)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func (m *MockPKCS11Interface) Decrypt(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	if m.failDecrypt {
		return nil, errors.New("mock decrypt failure")
	}
	m.mu.Lock()
	obj, ok := m.objects[key]
	m.mu.Unlock()

	if !ok {
		return nil, ErrKeyNotFound
	}

	block, err := aes.NewCipher(obj.keyMaterial)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(data) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (m *MockPKCS11Interface) Sign(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data []byte) ([]byte, error) {
	if m.failSign {
		return nil, errors.New("mock sign failure")
	}

	signature := make([]byte, 64)
	rand.Read(signature)
	return signature, nil
}

func (m *MockPKCS11Interface) Verify(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, key ObjectHandle, data, signature []byte) (bool, error) {
	return true, nil
}

func (m *MockPKCS11Interface) WrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, wrappingKey, keyToWrap ObjectHandle) ([]byte, error) {
	m.mu.Lock()
	wrapObj, wrapOk := m.objects[wrappingKey]
	targetObj, targetOk := m.objects[keyToWrap]
	m.mu.Unlock()

	if !wrapOk || !targetOk {
		return nil, ErrKeyNotFound
	}

	block, err := aes.NewCipher(wrapObj.keyMaterial)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)

	return gcm.Seal(nonce, nonce, targetObj.keyMaterial, nil), nil
}

func (m *MockPKCS11Interface) UnwrapKey(ctx context.Context, session SessionHandle, mechanism PKCS11Mechanism, unwrappingKey ObjectHandle, wrappedKey []byte, template map[string]interface{}) (ObjectHandle, error) {
	m.mu.Lock()
	wrapObj, ok := m.objects[unwrappingKey]
	m.mu.Unlock()

	if !ok {
		return 0, ErrKeyNotFound
	}

	block, err := aes.NewCipher(wrapObj.keyMaterial)
	if err != nil {
		return 0, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}

	if len(wrappedKey) < gcm.NonceSize() {
		return 0, ErrInvalidCiphertext
	}

	nonce, ciphertext := wrappedKey[:gcm.NonceSize()], wrappedKey[gcm.NonceSize():]
	keyMaterial, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextObject++
	label, _ := template["CKA_LABEL"].(string)
	obj := &mockObject{
		handle:      m.nextObject,
		class:       CKO_SECRET_KEY,
		keyType:     CKK_AES,
		label:       label,
		keyMaterial: keyMaterial,
	}
	m.objects[obj.handle] = obj

	return obj.handle, nil
}

func (m *MockPKCS11Interface) GenerateRandom(ctx context.Context, session SessionHandle, length int) ([]byte, error) {
	data := make([]byte, length)
	rand.Read(data)
	return data, nil
}

func (m *MockPKCS11Interface) DestroyObject(ctx context.Context, session SessionHandle, object ObjectHandle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, object)
	return nil
}

func (m *MockPKCS11Interface) AddTestKey(label string) ObjectHandle {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextObject++
	key := make([]byte, 32)
	rand.Read(key)

	obj := &mockObject{
		handle:      m.nextObject,
		class:       CKO_SECRET_KEY,
		keyType:     CKK_AES,
		label:       label,
		keyMaterial: key,
	}
	m.objects[obj.handle] = obj

	return obj.handle
}

func TestPKCS11Provider_BasicOperations(t *testing.T) {
	ctx := context.Background()
	mockIface := NewMockPKCS11Interface()

	config := &PKCS11Config{
		ProviderConfig: ProviderConfig{
			Name: "test-hsm",
			Type: ProviderTypePKCS11,
		},
		SlotID:      0,
		PIN:         "test-pin",
		MaxSessions: 5,
	}

	provider, err := NewPKCS11Provider(ctx, config, mockIface)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()

	if provider.Type() != ProviderTypePKCS11 {
		t.Errorf("Expected type %s, got %s", ProviderTypePKCS11, provider.Type())
	}

	if provider.Name() != "test-hsm" {
		t.Errorf("Expected name test-hsm, got %s", provider.Name())
	}

	if !provider.Healthy(ctx) {
		t.Error("Provider should be healthy")
	}
}

func TestPKCS11Provider_EncryptDecrypt(t *testing.T) {
	ctx := context.Background()
	mockIface := NewMockPKCS11Interface()
	keyHandle := mockIface.AddTestKey("test-key")

	config := &PKCS11Config{
		ProviderConfig: ProviderConfig{
			Name: "test-hsm",
			Type: ProviderTypePKCS11,
		},
		SlotID:      0,
		PIN:         "test-pin",
		MaxSessions: 5,
	}

	provider, err := NewPKCS11Provider(ctx, config, mockIface)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()

	plaintext := []byte("secret message for HSM encryption test")

	encResp, err := provider.Encrypt(ctx, &EncryptRequest{
		KeyID:     string(rune(keyHandle)),
		Plaintext: plaintext,
	})
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if len(encResp.Ciphertext) == 0 {
		t.Error("Ciphertext should not be empty")
	}

	decResp, err := provider.Decrypt(ctx, &DecryptRequest{
		KeyID:      string(rune(keyHandle)),
		Ciphertext: encResp.Ciphertext,
	})
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if string(decResp.Plaintext) != string(plaintext) {
		t.Errorf("Decrypted plaintext mismatch: got %s, want %s", decResp.Plaintext, plaintext)
	}
}

func TestPKCS11Provider_GenerateDataKey(t *testing.T) {
	ctx := context.Background()
	mockIface := NewMockPKCS11Interface()
	mockIface.AddTestKey("master-key")

	config := &PKCS11Config{
		ProviderConfig: ProviderConfig{
			Name: "test-hsm",
			Type: ProviderTypePKCS11,
		},
		SlotID:      0,
		PIN:         "test-pin",
		MaxSessions: 5,
	}

	provider, err := NewPKCS11Provider(ctx, config, mockIface)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()

	dk, err := provider.GenerateDataKey(ctx, &GenerateDataKeyRequest{
		KeyID:         "master-key",
		NumberOfBytes: 32,
	})
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if len(dk.Plaintext) != 32 {
		t.Errorf("Expected 32 bytes plaintext, got %d", len(dk.Plaintext))
	}

	if len(dk.Ciphertext) == 0 {
		t.Error("Ciphertext should not be empty")
	}

	if dk.Provider != ProviderTypePKCS11 {
		t.Errorf("Expected provider %s, got %s", ProviderTypePKCS11, dk.Provider)
	}

	dk.Zero()
	for _, b := range dk.Plaintext {
		if b != 0 {
			t.Error("Plaintext should be zeroed")
			break
		}
	}
}

func TestHSMSessionPool_BasicOperations(t *testing.T) {
	ctx := context.Background()
	mockIface := NewMockPKCS11Interface()

	config := &HSMSessionConfig{
		MinSessions:         2,
		MaxSessions:         5,
		IdleTimeout:         1 * time.Minute,
		HealthCheckInterval: 10 * time.Second,
		AcquireTimeout:      5 * time.Second,
	}

	pool, err := NewHSMSessionPool(ctx, config, mockIface, 0, "test-pin")
	if err != nil {
		t.Fatalf("Failed to create session pool: %v", err)
	}
	defer pool.Close()

	stats := pool.Stats()
	if stats.CurrentTotal < 2 {
		t.Errorf("Expected at least 2 sessions, got %d", stats.CurrentTotal)
	}

	session, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire session: %v", err)
	}

	if session.State != HSMSessionStateActive {
		t.Errorf("Expected active state, got %v", session.State)
	}

	session.Release()

	if session.State != HSMSessionStateIdle {
		t.Errorf("Expected idle state after release, got %v", session.State)
	}
}

func TestHSMSessionPool_Concurrency(t *testing.T) {
	ctx := context.Background()
	mockIface := NewMockPKCS11Interface()

	config := &HSMSessionConfig{
		MinSessions:    2,
		MaxSessions:    10,
		AcquireTimeout: 5 * time.Second,
	}

	pool, err := NewHSMSessionPool(ctx, config, mockIface, 0, "test-pin")
	if err != nil {
		t.Fatalf("Failed to create session pool: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	errCount := int32(0)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := pool.Acquire(ctx)
			if err != nil {
				atomic.AddInt32(&errCount, 1)
				return
			}
			time.Sleep(10 * time.Millisecond)
			session.Release()
		}()
	}

	wg.Wait()

	if errCount > 0 {
		t.Errorf("Got %d errors during concurrent access", errCount)
	}
}

func TestHSMCluster_LoadBalancing(t *testing.T) {
	tests := []struct {
		name     string
		strategy LoadBalancingStrategy
	}{
		{"RoundRobin", LBStrategyRoundRobin},
		{"Random", LBStrategyRandom},
		{"LeastConnections", LBStrategyLeastConnections},
		{"Weighted", LBStrategyWeighted},
		{"LatencyBased", LBStrategyLatencyBased},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &HSMClusterConfig{
				Name:              "test-cluster",
				Strategy:          tt.strategy,
				FailoverThreshold: 3,
				RetryAttempts:     2,
				RequestTimeout:    5 * time.Second,
			}

			cluster := NewHSMCluster(config)
			defer cluster.Close()

			for i := 0; i < 3; i++ {
				mockProvider := &MockProvider{name: string(rune('A' + i))}
				node := NewHSMNode(mockProvider.name, mockProvider, i+1, i)
				cluster.AddNode(node)
			}

			ctx := context.Background()
			nodeCounts := make(map[string]int)

			for i := 0; i < 10; i++ {
				node, err := cluster.SelectNode(ctx)
				if err != nil {
					t.Fatalf("Failed to select node: %v", err)
				}
				nodeCounts[node.Name]++
			}

			if len(nodeCounts) == 0 {
				t.Error("No nodes were selected")
			}
		})
	}
}

func TestHSMCluster_Failover(t *testing.T) {
	config := &HSMClusterConfig{
		Name:                  "failover-cluster",
		Strategy:              LBStrategyRoundRobin,
		FailoverThreshold:     2,
		RecoveryThreshold:     2,
		CircuitBreakerTimeout: 100 * time.Millisecond,
		RetryAttempts:         3,
		RetryDelay:            10 * time.Millisecond,
		RequestTimeout:        1 * time.Second,
	}

	cluster := NewHSMCluster(config)
	defer cluster.Close()

	failingProvider := &MockProvider{name: "failing", failEncrypt: true}
	workingProvider := &MockProvider{name: "working"}

	cluster.AddNode(NewHSMNode("failing", failingProvider, 1, 0))
	cluster.AddNode(NewHSMNode("working", workingProvider, 1, 0))

	ctx := context.Background()
	_, err := cluster.Encrypt(ctx, &EncryptRequest{
		KeyID:     "test-key",
		Plaintext: []byte("test data"),
	})
	if err != nil {
		t.Fatalf("Encryption should succeed with failover: %v", err)
	}

	stats := cluster.Stats()
	var failingStats, workingStats HSMNodeStats
	for _, s := range stats {
		if s.Name == "failing" {
			failingStats = s
		} else if s.Name == "working" {
			workingStats = s
		}
	}

	if failingStats.TotalErrors == 0 {
		t.Error("Failing node should have errors")
	}
	if workingStats.TotalRequests == 0 {
		t.Error("Working node should have requests")
	}
}

func TestHSMCluster_CircuitBreaker(t *testing.T) {
	config := &HSMClusterConfig{
		Name:                  "circuit-breaker-cluster",
		Strategy:              LBStrategyRoundRobin,
		FailoverThreshold:     2,
		CircuitBreakerTimeout: 50 * time.Millisecond,
		RetryAttempts:         0,
		RequestTimeout:        1 * time.Second,
	}

	cluster := NewHSMCluster(config)
	defer cluster.Close()

	failingProvider := &MockProvider{name: "failing", failEncrypt: true}
	node := NewHSMNode("failing", failingProvider, 1, 0)
	cluster.AddNode(node)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		cluster.Encrypt(ctx, &EncryptRequest{
			KeyID:     "test-key",
			Plaintext: []byte("test"),
		})
	}

	if node.State() != HSMNodeStateCircuitOpen {
		t.Errorf("Expected circuit open state, got %v", node.State())
	}

	time.Sleep(100 * time.Millisecond)

	if !node.TryRecovery(config.CircuitBreakerTimeout) {
		t.Error("Recovery should be allowed after timeout")
	}

	if node.State() != HSMNodeStateDegraded {
		t.Errorf("Expected degraded state after recovery, got %v", node.State())
	}
}

func TestHSMNode_Statistics(t *testing.T) {
	mockProvider := &MockProvider{name: "stats-test"}
	node := NewHSMNode("stats-test", mockProvider, 1, 0)

	node.RecordSuccess(10 * time.Millisecond)
	node.RecordSuccess(20 * time.Millisecond)
	node.RecordSuccess(30 * time.Millisecond)

	stats := node.Stats()

	if stats.TotalRequests != 3 {
		t.Errorf("Expected 3 requests, got %d", stats.TotalRequests)
	}

	if stats.TotalErrors != 0 {
		t.Errorf("Expected 0 errors, got %d", stats.TotalErrors)
	}

	expectedAvg := 20 * time.Millisecond
	if stats.AverageLatency != expectedAvg {
		t.Errorf("Expected average latency %v, got %v", expectedAvg, stats.AverageLatency)
	}
}

func TestPKCS11Types(t *testing.T) {
	tests := []struct {
		mechanism PKCS11Mechanism
		expected  string
	}{
		{CKM_AES_GCM, "CKM_AES_GCM"},
		{CKM_RSA_PKCS, "CKM_RSA_PKCS"},
		{CKM_SHA256_RSA_PKCS, "CKM_SHA256_RSA_PKCS"},
		{PKCS11Mechanism(0xFFFFFFFF), "CKM_UNKNOWN_0xFFFFFFFF"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mechanism.String(); got != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestPKCS11Error(t *testing.T) {
	tests := []struct {
		code    PKCS11ReturnValue
		wantErr bool
	}{
		{CKR_OK, false},
		{CKR_GENERAL_ERROR, true},
		{CKR_PIN_INCORRECT, true},
	}

	for _, tt := range tests {
		t.Run(tt.code.Error(), func(t *testing.T) {
			err := NewPKCS11Error(tt.code)
			if tt.wantErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected nil, got %v", err)
			}
		})
	}
}

func TestWithRetry(t *testing.T) {
	ctx := context.Background()
	mockIface := NewMockPKCS11Interface()

	config := &HSMSessionConfig{
		MinSessions:   1,
		MaxSessions:   5,
		RetryAttempts: 2,
		RetryDelay:    10 * time.Millisecond,
	}

	pool, err := NewHSMSessionPool(ctx, config, mockIface, 0, "test-pin")
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	attempts := 0
	err = WithRetry(ctx, pool, config, func(session *HSMPooledSession) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		t.Errorf("WithRetry should succeed: %v", err)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

// MockProvider implements Provider for testing.
type MockProvider struct {
	name        string
	failEncrypt bool
	failDecrypt bool
}

func (m *MockProvider) Type() ProviderType {
	return ProviderTypePKCS11
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Healthy(ctx context.Context) bool {
	return true
}

func (m *MockProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	return &KeyMetadata{
		KeyID:    keyID,
		Provider: ProviderTypePKCS11,
		Enabled:  true,
	}, nil
}

func (m *MockProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	if m.failEncrypt {
		return nil, errors.New("mock encrypt failure")
	}
	ciphertext := make([]byte, len(req.Plaintext)+16)
	copy(ciphertext, req.Plaintext)
	return &EncryptResponse{
		Ciphertext: ciphertext,
		KeyID:      req.KeyID,
	}, nil
}

func (m *MockProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	if m.failDecrypt {
		return nil, errors.New("mock decrypt failure")
	}
	return &DecryptResponse{
		Plaintext: req.Ciphertext[:len(req.Ciphertext)-16],
		KeyID:     req.KeyID,
	}, nil
}

func (m *MockProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	plaintext := make([]byte, 32)
	rand.Read(plaintext)
	return &DataKey{
		Plaintext:   plaintext,
		Ciphertext:  append([]byte("wrapped:"), plaintext...),
		KeyID:       req.KeyID,
		Provider:    ProviderTypePKCS11,
		GeneratedAt: time.Now(),
	}, nil
}

func (m *MockProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	return &WrapKeyResponse{
		WrappedKey:   append([]byte("wrapped:"), req.KeyToWrap...),
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

func (m *MockProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	return &UnwrapKeyResponse{
		PlaintextKey: req.WrappedKey[8:],
		WrapperKeyID: req.WrapperKeyID,
	}, nil
}

func (m *MockProvider) Close() error {
	return nil
}
