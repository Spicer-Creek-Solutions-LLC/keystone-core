// SPDX-License-Identifier: Apache-2.0

package semver

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"plain", "1.2.3", false},
		{"v-prefix", "v1.2.3", false},
		{"prerelease", "1.2.3-alpha.1", false},
		{"metadata", "1.2.3+build.42", false},
		{"prerelease+metadata", "1.2.3-rc.1+build.42", false},
		{"empty", "", true},
		{"garbage", "not.a.version", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	v := MustParse("1.2.3")
	if v.String() != "1.2.3" {
		t.Errorf("MustParse(\"1.2.3\").String() = %q, want 1.2.3", v.String())
	}
}

func TestMustParse_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParse(\"bad\") did not panic")
		}
	}()
	MustParse("bad")
}

func TestVersion_Components(t *testing.T) {
	v := MustParse("v1.2.3-rc.1+build.42")
	if got := v.Original(); got != "v1.2.3-rc.1+build.42" {
		t.Errorf("Original() = %q", got)
	}
	if got := v.String(); got != "1.2.3-rc.1+build.42" {
		t.Errorf("String() = %q", got)
	}
	if v.Major() != 1 || v.Minor() != 2 || v.Patch() != 3 {
		t.Errorf("Major/Minor/Patch = %d/%d/%d", v.Major(), v.Minor(), v.Patch())
	}
	if got := v.Prerelease(); got != "rc.1" {
		t.Errorf("Prerelease() = %q", got)
	}
	if got := v.Metadata(); got != "build.42" {
		t.Errorf("Metadata() = %q", got)
	}
}

func TestVersion_Compare(t *testing.T) {
	a := MustParse("1.2.3")
	b := MustParse("1.2.4")
	if got := a.Compare(b); got >= 0 {
		t.Errorf("1.2.3.Compare(1.2.4) = %d, want < 0", got)
	}
	if !a.LessThan(b) {
		t.Error("1.2.3.LessThan(1.2.4) = false")
	}
	if !b.GreaterThan(a) {
		t.Error("1.2.4.GreaterThan(1.2.3) = false")
	}
	if !a.Equal(MustParse("1.2.3")) {
		t.Error("Equal(1.2.3, 1.2.3) = false")
	}
}

func TestVersion_Equal_IgnoresMetadata(t *testing.T) {
	a := MustParse("1.2.3+build1")
	b := MustParse("1.2.3+build2")
	if !a.Equal(b) {
		t.Error("Equal should ignore build metadata per SemVer 2.0.0 §10")
	}
}

func TestVersion_Next(t *testing.T) {
	v := MustParse("1.2.3")
	if got := v.NextMajor(); got.String() != "2.0.0" {
		t.Errorf("NextMajor = %q, want 2.0.0", got)
	}
	if got := v.NextMinor(); got.String() != "1.3.0" {
		t.Errorf("NextMinor = %q, want 1.3.0", got)
	}
	if got := v.NextPatch(); got.String() != "1.2.4" {
		t.Errorf("NextPatch = %q, want 1.2.4", got)
	}
}

func TestSort(t *testing.T) {
	vs := []Version{
		MustParse("2.0.0"),
		MustParse("1.0.0"),
		MustParse("1.2.0"),
		MustParse("1.0.1"),
	}
	Sort(vs)
	got := make([]string, len(vs))
	for i, v := range vs {
		got[i] = v.String()
	}
	want := []string{"1.0.0", "1.0.1", "1.2.0", "2.0.0"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Sort = %v, want %v", got, want)
	}
}
