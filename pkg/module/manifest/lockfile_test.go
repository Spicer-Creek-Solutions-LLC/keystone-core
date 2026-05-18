package manifest

import (
	"strings"
	"testing"
)

func sampleLock() *LockFile {
	return &LockFile{
		SchemaVersion: LockFileSchemaVersion,
		Modules: map[string]LockedModule{
			"vendor/pkg_apt":    {Version: "1.2.3", Hash: "sha256:" + strings.Repeat("a", 64)},
			"vendor/pkg_common": {Version: "1.0.5", Hash: "sha256:" + strings.Repeat("b", 64)},
		},
	}
}

func TestLockFile_RoundTripAndValidate(t *testing.T) {
	lf := sampleLock()
	if err := lf.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	b, err := MarshalLockFile(lf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalLockFile(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("validate round-trip: %v", err)
	}
	if got.SchemaVersion != 1 || len(got.Modules) != 2 ||
		got.Modules["vendor/pkg_apt"].Version != "1.2.3" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestLockFile_Deterministic(t *testing.T) {
	// Same logical content, different map insertion order → identical
	// bytes (sorted keys), and stable across repeated marshals.
	a := &LockFile{SchemaVersion: 1, Modules: map[string]LockedModule{}}
	for _, n := range []string{"z/last", "a/first", "m/mid"} {
		a.Modules[n] = LockedModule{Version: "1.0.0", Hash: "sha256:" + strings.Repeat("c", 64)}
	}
	b := &LockFile{SchemaVersion: 1, Modules: map[string]LockedModule{}}
	for _, n := range []string{"a/first", "m/mid", "z/last"} {
		b.Modules[n] = LockedModule{Version: "1.0.0", Hash: "sha256:" + strings.Repeat("c", 64)}
	}
	ab, _ := MarshalLockFile(a)
	bb, _ := MarshalLockFile(b)
	if string(ab) != string(bb) {
		t.Fatalf("not order-independent:\n%s\n---\n%s", ab, bb)
	}
	ab2, _ := MarshalLockFile(a)
	if string(ab) != string(ab2) {
		t.Fatalf("not stable across marshals")
	}
	if !strings.Contains(string(ab), "a/first") ||
		strings.Index(string(ab), "a/first") > strings.Index(string(ab), "z/last") {
		t.Fatalf("keys not sorted:\n%s", ab)
	}
}

func TestLockFile_ValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*LockFile)
	}{
		{"bad schema", func(l *LockFile) { l.SchemaVersion = 2 }},
		{"bad module name", func(l *LockFile) {
			l.Modules = map[string]LockedModule{"bare": {Version: "1.0.0", Hash: "sha256:" + strings.Repeat("a", 64)}}
		}},
		{"bad version", func(l *LockFile) {
			l.Modules = map[string]LockedModule{"v/p": {Version: "x", Hash: "sha256:" + strings.Repeat("a", 64)}}
		}},
		{"short hash", func(l *LockFile) {
			l.Modules = map[string]LockedModule{"v/p": {Version: "1.0.0", Hash: "sha256:abc"}}
		}},
		{"no prefix hash", func(l *LockFile) {
			l.Modules = map[string]LockedModule{"v/p": {Version: "1.0.0", Hash: strings.Repeat("a", 64)}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lf := sampleLock()
			tc.mut(lf)
			if err := lf.Validate(); err == nil {
				t.Fatalf("%s: want error", tc.name)
			}
		})
	}
	var nilLF *LockFile
	if err := nilLF.Validate(); err == nil {
		t.Fatal("nil Validate: want error")
	}
	if _, err := MarshalLockFile(nil); err == nil {
		t.Fatal("nil Marshal: want error")
	}
}
