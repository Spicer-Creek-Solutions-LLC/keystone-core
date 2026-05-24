// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func signHMAC(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return hex.EncodeToString(m.Sum(nil))
}

func authReq(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestNoneAuthenticator(t *testing.T) {
	t.Parallel()
	var a Authenticator = NoneAuthenticator{}
	if a.Method() != AuthNone {
		t.Errorf("Method() = %q, want none", a.Method())
	}
	if err := a.Authenticate(authReq(t, nil), []byte("anything")); err != nil {
		t.Errorf("None.Authenticate = %v, want nil", err)
	}
}

func TestHMACAuthenticator(t *testing.T) {
	t.Parallel()
	const secret, body = "s3cr3t", `{"hello":"world"}`
	good := signHMAC(secret, body)
	a := HMACAuthenticator{Secret: []byte(secret), SignatureHeader: "X-Hub-Signature-256", Prefix: "sha256="}

	if a.Method() != AuthHMAC {
		t.Fatalf("Method() = %q, want hmac", a.Method())
	}
	cases := []struct {
		name    string
		header  map[string]string
		body    string
		wantErr bool
	}{
		{"valid", map[string]string{"X-Hub-Signature-256": "sha256=" + good}, body, false},
		{"valid no prefix in value still ok via TrimPrefix noop", map[string]string{"X-Hub-Signature-256": good}, body, false},
		// Phase B5 finding H3: senders may emit uppercase hex per
		// RFC; the pre-fix comparison was case-sensitive and
		// silently rejected these. Now valid.
		{"valid uppercase hex", map[string]string{"X-Hub-Signature-256": "sha256=" + strings.ToUpper(good)}, body, false},
		// Phase B5 finding H3: a mixed-case hex value should still
		// authenticate identically; the comparison is on raw MAC
		// bytes after hex.DecodeString.
		{"valid mixed-case hex", map[string]string{"X-Hub-Signature-256": "sha256=" + strings.ToUpper(good[:8]) + good[8:]}, body, false},
		{"wrong signature", map[string]string{"X-Hub-Signature-256": "sha256=" + good}, body + "tampered", true},
		{"missing header", nil, body, true},
		{"garbage header", map[string]string{"X-Hub-Signature-256": "sha256=zzzz"}, body, true},
		// Phase B5 finding H3: odd-length hex (cannot decode) maps
		// to ErrUnauthenticated, indistinguishable from a wrong
		// signature.
		{"odd-length hex", map[string]string{"X-Hub-Signature-256": "sha256=abc"}, body, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := a.Authenticate(authReq(t, tc.header), []byte(tc.body))
			if tc.wantErr {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("err = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBearerAuthenticator(t *testing.T) {
	t.Parallel()
	raw := BearerAuthenticator{Header: "X-Gitlab-Token", Token: []byte("tok")}
	scheme := BearerAuthenticator{Header: "Authorization", Token: []byte("tok"), RequireScheme: true}

	if raw.Method() != AuthBearer {
		t.Fatalf("Method() = %q, want bearer", raw.Method())
	}
	cases := []struct {
		name    string
		a       BearerAuthenticator
		header  map[string]string
		wantErr bool
	}{
		{"raw valid", raw, map[string]string{"X-Gitlab-Token": "tok"}, false},
		{"raw wrong", raw, map[string]string{"X-Gitlab-Token": "nope"}, true},
		{"raw missing", raw, nil, true},
		{"scheme valid", scheme, map[string]string{"Authorization": "Bearer tok"}, false},
		{"scheme missing prefix", scheme, map[string]string{"Authorization": "tok"}, true},
		{"scheme wrong token", scheme, map[string]string{"Authorization": "Bearer no"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.a.Authenticate(authReq(t, tc.header), nil)
			if tc.wantErr {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("err = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
