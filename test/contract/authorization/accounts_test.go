//go:build contract

package authorizationcontract

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nats-io/jwt/v2"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// ARCH-NATS-001. Two accounts, and no Keystone principal in the system one.
func TestGEN1TwoAccountsAndNoSystemPrincipals(t *testing.T) {
	d, dir := writtenDeployment(t)

	operator, err := jwt.DecodeOperatorClaims(d.OperatorJWT)
	if err != nil {
		t.Fatalf("decode operator: %v", err)
	}
	if operator.SystemAccount != d.System.PublicKey {
		t.Fatalf("operator names system account %q, want %q", operator.SystemAccount, d.System.PublicKey)
	}
	if d.System.PublicKey == d.Keystone.PublicKey {
		t.Fatal("the system account and the Keystone account are the same account")
	}

	// The configuration a broker reads is the artifact that decides this, so it
	// is what gets counted rather than the Go struct beside it.
	config := readConfig(t, dir)
	accounts := 0
	for _, key := range []string{d.System.PublicKey, d.Keystone.PublicKey} {
		if !strings.Contains(config, key) {
			t.Errorf("%s does not preload account %s", natsauth.ServerConfigFile, key)
		}
		accounts++
	}
	if got := strings.Count(config, "resolver_preload"); got != 1 {
		t.Errorf("resolver_preload appears %d times, want 1", got)
	}
	if accounts != 2 {
		t.Fatalf("preloaded %d accounts, want 2", accounts)
	}

	// Every Keystone principal is issued by the Keystone account. One issued by
	// the system account would hold administrative reach by construction.
	for _, n := range everyIdentity(t, d) {
		claims := userClaims(t, n.identity.JWT)
		if claims.IssuerAccount != d.Keystone.PublicKey {
			t.Errorf("%s is issued by %q, want the Keystone account %q",
				n.name, claims.IssuerAccount, d.Keystone.PublicKey)
		}
		if claims.IssuerAccount == d.System.PublicKey {
			t.Errorf("%s is a principal in the system account", n.name)
		}
	}
}

// ADR-0002 section 9: no limit may be absent or infinite. Every numeric field
// is walked rather than listed, so a limit added later is covered without this
// case being edited -- and no field is exempt, which is why the command
// stream's per-subject limit is set rather than left at a benign zero.
func TestGEN5AccountLimitsAreFinite(t *testing.T) {
	d, _ := writtenDeployment(t)

	var walk func(path string, v reflect.Value)
	walk = func(path string, v reflect.Value) {
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(path+"."+v.Type().Field(i).Name, v.Field(i))
			}
		case reflect.Slice:
			if v.Len() == 0 {
				t.Errorf("%s is empty; ADR-0002 section 9 requires an explicit schedule", path)
			}
			for i := 0; i < v.Len(); i++ {
				walk(path, v.Index(i))
			}
		case reflect.Int64, reflect.Int:
			if v.Int() <= 0 {
				t.Errorf("%s is %d; no limit may be absent or infinite", path, v.Int())
			}
		default:
			t.Errorf("%s has unexpected kind %s; the walk cannot judge it", path, v.Kind())
		}
	}
	walk("Limits", reflect.ValueOf(d.Limits))

	// The account JWT is where the account-scoped limits actually bind, and
	// jwt.NoLimit is -1 rather than absent, so both are rejected.
	account, err := jwt.DecodeAccountClaims(d.Keystone.JWT)
	if err != nil {
		t.Fatalf("decode keystone account: %v", err)
	}
	for name, value := range map[string]int64{
		"Conn":        account.Limits.Conn,
		"Subs":        account.Limits.Subs,
		"Payload":     account.Limits.Payload,
		"Data":        account.Limits.Data,
		"DiskStorage": account.Limits.DiskStorage,
	} {
		if value <= 0 {
			t.Errorf("account limit %s is %d; ADR-0002 section 9 requires it present and finite", name, value)
		}
	}
}

// ADR-0002 section 2 holds the operator seed outside every Keystone process.
// Every byte the deployment writes is searched for it, so the rule is checked
// against the artifacts rather than asserted in a comment beside the writer.
func TestGEN7OperatorSeedIsOutsideDeploymentProcesses(t *testing.T) {
	d, dir := writtenDeployment(t)

	if len(d.OperatorSeed) == 0 {
		t.Fatal("no operator seed was generated; the search below would pass by checking nothing")
	}
	seed := string(d.OperatorSeed)

	files := 0
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		files++
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(content), seed) {
			rel, _ := filepath.Rel(dir, path)
			t.Errorf("%s contains the operator seed", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk deployment: %v", err)
	}
	if files == 0 {
		t.Fatal("the deployment wrote no files; the search read nothing")
	}

	// The account signing key IS the server's, per ADR-0002 section 2. Asserting
	// its presence is what stops this case passing against a directory that
	// simply holds no secrets at all.
	signing := filepath.Join(dir, natsauth.CredentialsDir, "account-signing.nk")
	content, err := os.ReadFile(signing)
	if err != nil {
		t.Fatalf("read account signing key: %v", err)
	}
	if string(content) != string(d.Keystone.SigningSeed) {
		t.Error("the account signing key the server needs was not written")
	}
}

func readConfig(t *testing.T, dir string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, natsauth.ServerConfigFile))
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	return string(content)
}
