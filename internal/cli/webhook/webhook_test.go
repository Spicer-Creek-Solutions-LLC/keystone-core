// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

func runCmd(t *testing.T, d Deps, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCommand(d)
	var out, errb bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func deterministicDeps(t *testing.T) Deps {
	t.Helper()
	var n int32
	return Deps{
		IDGen: func() string { return "sub-" + string(rune('a'+int(atomic.AddInt32(&n, 1)-1))) },
		Now:   func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) },
	}
}

func TestOutbound_Create_AcceptanceLine111(t *testing.T) {
	t.Parallel()
	// Acceptance 111: `kscore-webhook outbound create --name slack
	// --url https://hooks/... --events 'state.drift,policy.violation'
	// --secret xxx` succeeds.
	store := filepath.Join(t.TempDir(), "wh.db")
	d := deterministicDeps(t)
	out, _, err := runCmd(t, d, "outbound", "create",
		"--name", "slack",
		"--url", "https://hooks.slack.com/x",
		"--events", "state.drift,policy.violation",
		"--secret", "xxx",
		"--headers", "X-Source=keystone",
		"--store", store,
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("create: %v out=%s", err, out)
	}
	var sub outbound.Subscription
	if err := json.Unmarshal([]byte(out), &sub); err != nil {
		t.Fatalf("decode create out: %v\n%s", err, out)
	}
	// Cleartext secret echoed once on creation (§4.14 contract).
	if sub.Secret != "xxx" {
		t.Errorf("create response Secret = %q, want cleartext xxx", sub.Secret)
	}
	if sub.Name != "slack" || sub.URL != "https://hooks.slack.com/x" ||
		len(sub.Events) != 2 || sub.Headers["X-Source"] != "keystone" {
		t.Errorf("create did not capture flags: %+v", sub)
	}

	// Read back via show — secret must be masked.
	out2, _, err := runCmd(t, d, "outbound", "show", sub.ID, "--store", store, "--output", "json")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	var shown outbound.Subscription
	if err := json.Unmarshal([]byte(out2), &shown); err != nil {
		t.Fatalf("decode show: %v\n%s", err, out2)
	}
	if shown.Secret != "***" {
		t.Errorf("show Secret = %q, want masked", shown.Secret)
	}
}

func TestOutbound_ListShowDelete(t *testing.T) {
	t.Parallel()
	store := filepath.Join(t.TempDir(), "wh.db")
	d := deterministicDeps(t)
	_, _, err := runCmd(t, d, "outbound", "create",
		"--name", "n1", "--url", "https://u1", "--events", "a.*", "--store", store)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = runCmd(t, d, "outbound", "create",
		"--name", "n2", "--url", "https://u2", "--events", "b.*", "--store", store)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	out, _, err := runCmd(t, d, "outbound", "list", "--store", store, "--output", "json")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var got []outbound.Subscription
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != 2 {
		t.Fatalf("list got %d entries: %v\n%s", len(got), err, out)
	}

	// Delete one and re-list.
	_, _, err = runCmd(t, d, "outbound", "delete", got[0].ID, "--store", store)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	out, _, _ = runCmd(t, d, "outbound", "list", "--store", store, "--output", "json")
	_ = json.Unmarshal([]byte(out), &got)
	if len(got) != 1 {
		t.Errorf("after delete, list = %d, want 1", len(got))
	}

	// show missing → error
	_, _, err = runCmd(t, d, "outbound", "show", "missing", "--store", store)
	if err == nil {
		t.Error("show missing = nil, want not-found error")
	}
}

func TestOutbound_Test_DispatchesToReceiver_Acceptance112(t *testing.T) {
	t.Parallel()
	// Acceptance 112: POST /api/v1/webhooks/subscriptions/{id}/test
	// delivers a synthetic payload. The CLI test command exercises
	// the same dispatch path (HTTPDispatcher) — verify it actually
	// POSTs to the configured URL.
	var (
		gotMethod string
		gotBody   []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	store := filepath.Join(t.TempDir(), "wh.db")
	d := deterministicDeps(t)
	_, _, err := runCmd(t, d, "outbound", "create",
		"--name", "t", "--url", srv.URL, "--events", "ignored.*", "--store", store)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// We don't capture the ID directly — list to find it.
	listOut, _, _ := runCmd(t, d, "outbound", "list", "--store", store, "--output", "json")
	var subs []outbound.Subscription
	_ = json.Unmarshal([]byte(listOut), &subs)
	if len(subs) != 1 {
		t.Fatalf("expected 1 sub, got %d", len(subs))
	}

	out, _, err := runCmd(t, d, "outbound", "test", subs[0].ID, "--store", store, "--output", "json")
	if err != nil {
		t.Fatalf("test: %v\n%s", err, out)
	}
	var rec outbound.DeliveryRecord
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("decode test out: %v\n%s", err, out)
	}
	if rec.Status != outbound.DeliverySuccess || rec.StatusCode != 200 || rec.EventType != "webhook.test" {
		t.Errorf("test record = %+v", rec)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("receiver got %s, want POST", gotMethod)
	}
	if !strings.Contains(string(gotBody), `"event":"webhook.test"`) {
		t.Errorf("receiver body = %s, want synthetic ping JSON", gotBody)
	}
}

func TestOutbound_History_AcceptanceLine116(t *testing.T) {
	t.Parallel()
	// Acceptance 116: GET /api/v1/webhooks/subscriptions/{id}/deliveries
	// lists delivery history. The CLI `history` subcommand exercises
	// the same store path. Seed deliveries directly so we don't
	// depend on a live receiver.
	store := filepath.Join(t.TempDir(), "wh.db")
	d := deterministicDeps(t)
	_, _, err := runCmd(t, d, "outbound", "create",
		"--name", "h", "--url", "https://h", "--events", "any.*", "--store", store)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	listOut, _, _ := runCmd(t, d, "outbound", "list", "--store", store, "--output", "json")
	var subs []outbound.Subscription
	_ = json.Unmarshal([]byte(listOut), &subs)
	sub := subs[0]

	s, err := outbound.NewSQLiteStore(store)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for i := 0; i < 3; i++ {
		_ = s.SaveDelivery(context.Background(), &outbound.DeliveryRecord{
			ID:             "d" + string(rune('0'+i)),
			SubscriptionID: sub.ID,
			EventType:      "state.drift",
			Status:         outbound.DeliverySuccess,
			StatusCode:     200,
			Attempt:        1,
			DeliveredAt:    time.Now(),
		})
	}
	_ = s.Close()

	out, _, err := runCmd(t, d, "outbound", "history", sub.ID, "--store", store, "--output", "json")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var list []outbound.DeliveryRecord
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("decode history: %v\n%s", err, out)
	}
	if len(list) != 3 {
		t.Errorf("history len = %d, want 3", len(list))
	}
}

func TestOutbound_CreateRequiresNameAndURL(t *testing.T) {
	t.Parallel()
	store := filepath.Join(t.TempDir(), "wh.db")
	_, _, err := runCmd(t, deterministicDeps(t), "outbound", "create", "--name", "x", "--store", store)
	if err == nil {
		t.Error("create without --url = nil error")
	}
}

func TestOutbound_HeaderCSVParse(t *testing.T) {
	t.Parallel()
	got, err := parseHeaderCSV("a=1, b=2 ,c=3=trailing")
	if err != nil || got["a"] != "1" || got["b"] != "2" || got["c"] != "3=trailing" {
		t.Errorf("parseHeaderCSV: got=%v err=%v", got, err)
	}
	if _, err := parseHeaderCSV("invalid"); err == nil {
		t.Error("malformed entry = nil error")
	}
	if got, _ := parseHeaderCSV(""); got != nil {
		t.Errorf("empty = %v, want nil", got)
	}
}
