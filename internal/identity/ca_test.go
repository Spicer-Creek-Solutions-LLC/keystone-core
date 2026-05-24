// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

// ---- helpers -----------------------------------------------------

// newFastCAConfig is DefaultCAConfig with shorter TTLs so the tests
// stay fast and rotation boundaries are reachable inside a unit
// test budget.
func newFastCAConfig(td string) CAConfig {
	c := DefaultCAConfig(td)
	c.RootCATTL = 10 * time.Hour
	c.SigningCATTL = 2 * time.Hour
	c.RotateBefore = 30 * time.Minute
	c.DefaultSVIDTTL = 15 * time.Minute
	c.MaxSVIDTTL = 1 * time.Hour
	return c
}

func newTempStorage(t *testing.T) *FileCAStorage {
	t.Helper()
	s, err := NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	return s
}

func newInitializedManager(t *testing.T, cfg CAConfig) (*CAManager, CAStorage) {
	t.Helper()
	storage := newTempStorage(t)
	m, err := NewCAManager(cfg, storage)
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return m, storage
}

// ---- config / constructor ---------------------------------------

func TestDefaultCAConfig(t *testing.T) {
	t.Parallel()
	c := DefaultCAConfig(DefaultTrustDomain)
	if c.TrustDomain != DefaultTrustDomain {
		t.Errorf("TrustDomain = %q", c.TrustDomain)
	}
	if c.KeyType != KeyTypeECDSAP256 {
		t.Errorf("KeyType = %q, want ecdsa-p256", c.KeyType)
	}
	if c.RootCATTL != defaultRootCATTL {
		t.Errorf("RootCATTL = %s", c.RootCATTL)
	}
	if c.SigningCATTL != defaultSigningCATTL {
		t.Errorf("SigningCATTL = %s", c.SigningCATTL)
	}
	if c.RotateBefore != defaultRotateBefore {
		t.Errorf("RotateBefore = %s", c.RotateBefore)
	}
	if c.MaxSVIDTTL != maxSVIDTTLDefault {
		t.Errorf("MaxSVIDTTL = %s", c.MaxSVIDTTL)
	}
}

func TestNewCAManager_RejectsNilStorage(t *testing.T) {
	t.Parallel()
	_, err := NewCAManager(DefaultCAConfig(DefaultTrustDomain), nil)
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewCAManager_RejectsEmptyTrustDomain(t *testing.T) {
	t.Parallel()
	c := DefaultCAConfig("")
	_, err := NewCAManager(c, newTempStorage(t))
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewCAManager_RejectsBadKeyType(t *testing.T) {
	t.Parallel()
	c := DefaultCAConfig(DefaultTrustDomain)
	c.KeyType = CAKeyType("aes-256") // not a signing algorithm
	_, err := NewCAManager(c, newTempStorage(t))
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewCAManager_RejectsRotateBeforeTooLarge(t *testing.T) {
	t.Parallel()
	c := DefaultCAConfig(DefaultTrustDomain)
	c.RotateBefore = c.SigningCATTL + time.Hour
	_, err := NewCAManager(c, newTempStorage(t))
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewCAManager_RejectsMaxSVIDTTLTooLarge(t *testing.T) {
	t.Parallel()
	c := DefaultCAConfig(DefaultTrustDomain)
	c.MaxSVIDTTL = c.SigningCATTL + time.Hour
	_, err := NewCAManager(c, newTempStorage(t))
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

// ---- Initialize -------------------------------------------------

func TestInitialize_GeneratesNewCAs(t *testing.T) {
	t.Parallel()
	storage := newTempStorage(t)
	m, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), storage)
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := m.GetTrustChain(); len(got) != 1 {
		t.Fatalf("GetTrustChain len = %d, want 1", len(got))
	}
	if has, err := storage.HasRootCA(); err != nil || !has {
		t.Errorf("HasRootCA: has=%v err=%v", has, err)
	}
	if has, err := storage.HasSigningCA(); err != nil || !has {
		t.Errorf("HasSigningCA: has=%v err=%v", has, err)
	}
}

func TestInitialize_Idempotent(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	root1 := m.GetTrustChain()[0]
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
	root2 := m.GetTrustChain()[0]
	if !root1.Equal(root2) {
		t.Error("second Initialize replaced root cert")
	}
}

func TestInitialize_LoadsExisting(t *testing.T) {
	t.Parallel()
	storage := newTempStorage(t)

	first, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), storage)
	if err != nil {
		t.Fatalf("first NewCAManager: %v", err)
	}
	if err := first.Initialize(context.Background()); err != nil {
		t.Fatalf("first Initialize: %v", err)
	}
	rootBefore := first.GetTrustChain()[0]

	// Second manager against the SAME storage must reuse the root.
	second, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), storage)
	if err != nil {
		t.Fatalf("second NewCAManager: %v", err)
	}
	if err := second.Initialize(context.Background()); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
	rootAfter := second.GetTrustChain()[0]
	if !rootBefore.Equal(rootAfter) {
		t.Error("second manager generated a new root instead of loading")
	}
}

func TestInitialize_AllKeyTypes(t *testing.T) {
	t.Parallel()
	for _, kt := range []CAKeyType{KeyTypeECDSAP256, KeyTypeECDSAP384, KeyTypeRSA2048, KeyTypeRSA4096} {
		kt := kt
		t.Run(string(kt), func(t *testing.T) {
			t.Parallel()
			c := newFastCAConfig(DefaultTrustDomain)
			c.KeyType = kt
			m, _ := newInitializedManager(t, c)
			if got := m.GetTrustChain(); len(got) != 1 {
				t.Fatalf("GetTrustChain len = %d", len(got))
			}
		})
	}
}

// failingStorage forces a chosen method to return err. Used to
// exercise Initialize's error wrapping.
type failingStorage struct {
	CAStorage
	failMethod string
	err        error
}

func (f *failingStorage) HasRootCA() (bool, error) {
	if f.failMethod == "HasRootCA" {
		return false, f.err
	}
	return f.CAStorage.HasRootCA()
}

func (f *failingStorage) HasSigningCA() (bool, error) {
	if f.failMethod == "HasSigningCA" {
		return false, f.err
	}
	return f.CAStorage.HasSigningCA()
}

func (f *failingStorage) SaveRootCA(c *x509.Certificate, k crypto.Signer) error {
	if f.failMethod == "SaveRootCA" {
		return f.err
	}
	return f.CAStorage.SaveRootCA(c, k)
}

func (f *failingStorage) SaveSigningCA(c *x509.Certificate, k crypto.Signer) error {
	if f.failMethod == "SaveSigningCA" {
		return f.err
	}
	return f.CAStorage.SaveSigningCA(c, k)
}

func (f *failingStorage) LoadRootCA() (*x509.Certificate, crypto.Signer, error) {
	if f.failMethod == "LoadRootCA" {
		return nil, nil, f.err
	}
	return f.CAStorage.LoadRootCA()
}

func (f *failingStorage) LoadSigningCA() (*x509.Certificate, crypto.Signer, error) {
	if f.failMethod == "LoadSigningCA" {
		return nil, nil, f.err
	}
	return f.CAStorage.LoadSigningCA()
}

func TestInitialize_WrapsStorageErrors(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"HasRootCA", "SaveRootCA", "HasSigningCA", "SaveSigningCA"} {
		method := method
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			fs := &failingStorage{
				CAStorage:  newTempStorage(t),
				failMethod: method,
				err:        errors.New("synthetic disk error"),
			}
			m, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), fs)
			if err != nil {
				t.Fatalf("NewCAManager: %v", err)
			}
			err = m.Initialize(context.Background())
			if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestInitialize_WrapsLoadErrors(t *testing.T) {
	t.Parallel()
	// Pre-seed root via normal Initialize, then force LoadRootCA to
	// fail on a fresh manager.
	storage := newTempStorage(t)
	m1, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), storage)
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if err := m1.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize seed: %v", err)
	}

	for _, method := range []string{"LoadRootCA", "LoadSigningCA"} {
		method := method
		t.Run(method, func(t *testing.T) {
			fs := &failingStorage{CAStorage: storage, failMethod: method, err: errors.New("read fail")}
			m2, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), fs)
			if err != nil {
				t.Fatalf("NewCAManager: %v", err)
			}
			err = m2.Initialize(context.Background())
			if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// ---- IssueCertificate -------------------------------------------

func TestIssueCertificate_HappyPath(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	id, _ := AgentID(DefaultTrustDomain, "agent-issue")
	subjKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	res, err := m.IssueCertificate(IssueRequest{
		ID:        id,
		PublicKey: &subjKey.PublicKey,
		TTL:       30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("IssueCertificate: %v", err)
	}
	if len(res.Chain) != 2 {
		t.Fatalf("Chain len = %d, want 2 (leaf + signing CA)", len(res.Chain))
	}
	if res.Leaf != res.Chain[0] {
		t.Error("Leaf != Chain[0]")
	}
	if len(res.Leaf.URIs) != 1 || res.Leaf.URIs[0].String() != id.String() {
		t.Errorf("URI SAN = %v, want [%s]", res.Leaf.URIs, id)
	}
	if got := res.Leaf.NotAfter.Sub(res.Leaf.NotBefore); got != 30*time.Minute {
		t.Errorf("validity = %s, want 30m", got)
	}

	// Wraps cleanly into X509SVID (integration with task 2).
	svid, err := NewX509SVID(id, res.Chain, subjKey, "")
	if err != nil {
		t.Fatalf("NewX509SVID: %v", err)
	}
	if !svid.SPIFFEID().Equal(id) {
		t.Errorf("SVID id = %q, want %q", svid.SPIFFEID(), id)
	}
}

func TestIssueCertificate_VerifiesAgainstTrustBundle(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	bundle, err := m.BuildTrustBundle()
	if err != nil {
		t.Fatalf("BuildTrustBundle: %v", err)
	}
	id, _ := AgentID(DefaultTrustDomain, "agent-verify")
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	res, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &subjKey.PublicKey})
	if err != nil {
		t.Fatalf("IssueCertificate: %v", err)
	}
	// *TrustBundle satisfies x509bundle.Source (task 4).
	gotID, chains, err := x509svid.Verify(res.Chain, bundle)
	if err != nil {
		t.Fatalf("x509svid.Verify: %v", err)
	}
	if gotID.String() != id.String() {
		t.Errorf("verified id = %q, want %q", gotID, id)
	}
	if len(chains) == 0 {
		t.Error("no verified chains")
	}
}

func TestIssueCertificate_ClampsTTL(t *testing.T) {
	t.Parallel()
	c := newFastCAConfig(DefaultTrustDomain)
	m, _ := newInitializedManager(t, c)
	id, _ := AgentID(DefaultTrustDomain, "agent-ttl")
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	// TTL > Max → capped.
	res, err := m.IssueCertificate(IssueRequest{
		ID: id, PublicKey: &subjKey.PublicKey,
		TTL: 10 * c.MaxSVIDTTL,
	})
	if err != nil {
		t.Fatalf("IssueCertificate: %v", err)
	}
	if got := res.Leaf.NotAfter.Sub(res.Leaf.NotBefore); got != c.MaxSVIDTTL {
		t.Errorf("validity = %s, want capped to %s", got, c.MaxSVIDTTL)
	}

	// TTL == 0 → falls back to Default.
	res2, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &subjKey.PublicKey})
	if err != nil {
		t.Fatalf("IssueCertificate default: %v", err)
	}
	if got := res2.Leaf.NotAfter.Sub(res2.Leaf.NotBefore); got != c.DefaultSVIDTTL {
		t.Errorf("default validity = %s, want %s", got, c.DefaultSVIDTTL)
	}
}

func TestIssueCertificate_DNSAndIP(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	id, _ := AgentID(DefaultTrustDomain, "agent-dns")
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	res, err := m.IssueCertificate(IssueRequest{
		ID:          id,
		PublicKey:   &subjKey.PublicKey,
		DNSNames:    []string{"agent.kscore.local"},
		IPAddresses: []net.IP{net.ParseIP("10.0.0.1")},
	})
	if err != nil {
		t.Fatalf("IssueCertificate: %v", err)
	}
	if len(res.Leaf.DNSNames) != 1 || res.Leaf.DNSNames[0] != "agent.kscore.local" {
		t.Errorf("DNSNames = %v", res.Leaf.DNSNames)
	}
	if len(res.Leaf.IPAddresses) != 1 || !res.Leaf.IPAddresses[0].Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("IPAddresses = %v", res.Leaf.IPAddresses)
	}
}

func TestIssueCertificate_BeforeInitialize(t *testing.T) {
	t.Parallel()
	m, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), newTempStorage(t))
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	id, _ := AgentID(DefaultTrustDomain, "agent-x")
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	_, err = m.IssueCertificate(IssueRequest{ID: id, PublicKey: &subjKey.PublicKey})
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestIssueCertificate_RejectsZeroID(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	_, err := m.IssueCertificate(IssueRequest{PublicKey: &subjKey.PublicKey})
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestIssueCertificate_RejectsNilPublicKey(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	id, _ := AgentID(DefaultTrustDomain, "agent-x")
	_, err := m.IssueCertificate(IssueRequest{ID: id})
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Fatalf("err = %v", err)
	}
}

func TestIssueCertificate_ConcurrentDistinctSerials(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	id, _ := AgentID(DefaultTrustDomain, "agent-c")

	const N = 20
	serials := make(chan string, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			res, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &k.PublicKey})
			if err != nil {
				t.Errorf("issue: %v", err)
				serials <- ""
				return
			}
			serials <- res.Leaf.SerialNumber.String()
		}()
	}
	wg.Wait()
	close(serials)

	seen := make(map[string]struct{}, N)
	for s := range serials {
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			t.Errorf("duplicate serial %s", s)
		}
		seen[s] = struct{}{}
	}
	if len(seen) != N {
		t.Errorf("got %d distinct serials, want %d", len(seen), N)
	}
}

// ---- ShouldRotateSigningCA + RotateSigningCA --------------------

func TestShouldRotateSigningCA(t *testing.T) {
	t.Parallel()
	c := newFastCAConfig(DefaultTrustDomain)
	frozen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Clock = func() time.Time { return frozen }
	m, _ := newInitializedManager(t, c)

	// Signing CA NotAfter = frozen + SigningCATTL (2h);
	// RotateBefore = 30m. Rotation window opens at frozen + 1h30m.
	if m.ShouldRotateSigningCA(frozen) {
		t.Error("ShouldRotate at issuance: want false")
	}
	if m.ShouldRotateSigningCA(frozen.Add(time.Hour + 29*time.Minute)) {
		t.Error("ShouldRotate at 1h29m: want false")
	}
	if !m.ShouldRotateSigningCA(frozen.Add(time.Hour + 30*time.Minute)) {
		t.Error("ShouldRotate at 1h30m (boundary): want true")
	}
	if !m.ShouldRotateSigningCA(frozen.Add(2 * time.Hour)) {
		t.Error("ShouldRotate at expiry: want true")
	}
}

func TestShouldRotateSigningCA_BeforeInitialize(t *testing.T) {
	t.Parallel()
	m, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), newTempStorage(t))
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if m.ShouldRotateSigningCA(time.Now()) {
		t.Error("uninitialized ShouldRotate: want false")
	}
}

func TestRotateSigningCA_GeneratesNew(t *testing.T) {
	t.Parallel()
	m, storage := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	id, _ := AgentID(DefaultTrustDomain, "agent-rot")
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	before, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &subjKey.PublicKey})
	if err != nil {
		t.Fatalf("issue before: %v", err)
	}
	rootBefore := m.GetTrustChain()[0]

	if err := m.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}

	after, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &subjKey.PublicKey})
	if err != nil {
		t.Fatalf("issue after: %v", err)
	}
	if before.Chain[1].Equal(after.Chain[1]) {
		t.Error("signing CA unchanged after Rotate")
	}
	if !m.GetTrustChain()[0].Equal(rootBefore) {
		t.Error("Rotate changed the root cert")
	}

	// Storage now holds the NEW signing CA.
	persistedSigning, _, err := storage.LoadSigningCA()
	if err != nil {
		t.Fatalf("LoadSigningCA: %v", err)
	}
	if !persistedSigning.Equal(after.Chain[1]) {
		t.Error("storage signing CA != active signing CA after Rotate")
	}
}

func TestRotateSigningCA_OldLeavesStillVerify(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	bundle, _ := m.BuildTrustBundle()

	id, _ := AgentID(DefaultTrustDomain, "agent-grace")
	subjKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	before, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &subjKey.PublicKey})
	if err != nil {
		t.Fatalf("issue before: %v", err)
	}

	if err := m.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// Old leaf's Chain still includes the old signing CA cert, and
	// the root in the bundle is unchanged → verification still
	// succeeds.
	if _, _, err := x509svid.Verify(before.Chain, bundle); err != nil {
		t.Errorf("old leaf failed verification after rotation: %v", err)
	}
}

func TestRotateSigningCA_BeforeInitialize(t *testing.T) {
	t.Parallel()
	m, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), newTempStorage(t))
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	if err := m.RotateSigningCA(context.Background()); err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Errorf("err = %v", err)
	}
}

func TestRotateSigningCA_StorageError(t *testing.T) {
	t.Parallel()
	storage := newTempStorage(t)
	m, _ := NewCAManager(newFastCAConfig(DefaultTrustDomain), storage)
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Swap in failing storage AFTER init so the swap doesn't break
	// load. The CAManager holds a reference to the original storage
	// — we have to mutate the underlying field. Workaround: build a
	// new manager pointed at a failing-on-save wrapper.
	failing := &failingStorage{CAStorage: storage, failMethod: "SaveSigningCA", err: errors.New("save fail")}
	m2, _ := NewCAManager(newFastCAConfig(DefaultTrustDomain), failing)
	if err := m2.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize m2: %v", err)
	}
	if err := m2.RotateSigningCA(context.Background()); err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Errorf("err = %v", err)
	}
}

// ---- BuildTrustBundle -------------------------------------------

func TestBuildTrustBundle(t *testing.T) {
	t.Parallel()
	m, _ := newInitializedManager(t, newFastCAConfig(DefaultTrustDomain))
	b, err := m.BuildTrustBundle()
	if err != nil {
		t.Fatalf("BuildTrustBundle: %v", err)
	}
	if b.TrustDomain() != DefaultTrustDomain {
		t.Errorf("TrustDomain = %q", b.TrustDomain())
	}
	if got := b.X509Authorities(); len(got) != 1 {
		t.Errorf("X509Authorities = %d, want 1", len(got))
	}
}

func TestBuildTrustBundle_BeforeInitialize(t *testing.T) {
	t.Parallel()
	m, err := NewCAManager(newFastCAConfig(DefaultTrustDomain), newTempStorage(t))
	if err != nil {
		t.Fatalf("NewCAManager: %v", err)
	}
	_, err = m.BuildTrustBundle()
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Errorf("err = %v", err)
	}
}

// ---- generateKey + randomSerial direct edge cases ---------------

func TestGenerateKey_RejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := generateKey(CAKeyType("aes"))
	if err == nil || !errors.Is(err, ErrInvalidCAConfig) {
		t.Errorf("err = %v", err)
	}
}

func TestGetTrustChain_BeforeInitialize(t *testing.T) {
	t.Parallel()
	m, _ := NewCAManager(newFastCAConfig(DefaultTrustDomain), newTempStorage(t))
	if got := m.GetTrustChain(); got != nil {
		t.Errorf("GetTrustChain before Initialize = %v, want nil", got)
	}
}

func TestIssueCertificate_MessageMentionsContext(t *testing.T) {
	// Spot-check the error message format on the most common
	// before-Initialize call so the operator-facing string stays
	// stable.
	t.Parallel()
	m, _ := NewCAManager(newFastCAConfig(DefaultTrustDomain), newTempStorage(t))
	id, _ := AgentID(DefaultTrustDomain, "agent-x")
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	_, err := m.IssueCertificate(IssueRequest{ID: id, PublicKey: &k.PublicKey})
	if err == nil || !strings.Contains(err.Error(), "before Initialize") {
		t.Errorf("err = %v, want \"before Initialize\" mention", err)
	}
}
