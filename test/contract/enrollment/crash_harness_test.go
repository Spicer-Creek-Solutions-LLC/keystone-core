//go:build contract

package enrollmentcontract

import (
	"encoding/json"
	"os"
	"testing"
)

type crashHarnessSurface struct {
	Version              int                `json:"version"`
	KillSignal           string             `json:"kill_signal"`
	AgentFaultRecords    faultRecordSurface `json:"agent_fault_records"`
	ServerFaultRecords   faultRecordSurface `json:"server_fault_records"`
	OrderingObservations []struct {
		Case      string `json:"case"`
		Mechanism string `json:"mechanism"`
	} `json:"ordering_observations"`
	Boundaries []struct {
		Case     string `json:"case"`
		Process  string `json:"process"`
		Fault    string `json:"fault"`
		Arrival  string `json:"arrival"`
		Recovery string `json:"recovery"`
	} `json:"boundaries"`
}

type faultRecordSurface struct {
	PathConfig          string `json:"path_config"`
	Store               string `json:"store"`
	Format              string `json:"format"`
	EnabledActionPrefix string `json:"enabled_action_prefix"`
	ReachedActionPrefix string `json:"reached_action_prefix"`
	Rule                string `json:"rule"`
}

func loadCrashHarnessSurface(t *testing.T) crashHarnessSurface {
	t.Helper()
	b, err := os.ReadFile("crash-harness.json")
	if err != nil {
		t.Fatal(err)
	}
	var surface crashHarnessSurface
	if err := json.Unmarshal(b, &surface); err != nil {
		t.Fatal(err)
	}
	return surface
}

// This self-test freezes the harness inputs before production behavior exists.
// C05-I supplies the process driver; it may not replace these observations with
// implementation markers or collapse two boundaries into one run.
func TestCrashHarnessSurfaceIsComplete(t *testing.T) {
	surface := loadCrashHarnessSurface(t)
	if surface.Version != 1 || surface.KillSignal != "SIGKILL" {
		t.Fatalf("unexpected harness header: version=%d signal=%q", surface.Version, surface.KillSignal)
	}
	if surface.AgentFaultRecords.PathConfig != agentTableFaults+"."+agentKeyFaultRecordPath ||
		surface.AgentFaultRecords.Format == "" || surface.AgentFaultRecords.Rule == "" {
		t.Fatalf("agent fault record is not frozen: %+v", surface.AgentFaultRecords)
	}
	if surface.ServerFaultRecords.Store != "audit" ||
		surface.ServerFaultRecords.EnabledActionPrefix != "fault.enabled:" ||
		surface.ServerFaultRecords.ReachedActionPrefix != "fault.reached:" ||
		surface.ServerFaultRecords.Rule == "" {
		t.Fatalf("server fault record is not frozen: %+v", surface.ServerFaultRecords)
	}

	wantOrdering := map[string]bool{"ORD-1": false, "OBS-1": false, "S6-1": false}
	for _, observation := range surface.OrderingObservations {
		if _, ok := wantOrdering[observation.Case]; !ok || observation.Mechanism == "" || wantOrdering[observation.Case] {
			t.Fatalf("bad ordering observation: %+v", observation)
		}
		wantOrdering[observation.Case] = true
	}
	for id, found := range wantOrdering {
		if !found {
			t.Errorf("%s has no independent observation", id)
		}
	}

	wantBoundaries := map[string]string{
		"CRASH-1": faultAgentBeforeRequest,
		"CRASH-2": faultAgentAfterReply,
		"CRASH-3": faultAgentAfterIdentityWrite,
		"CRASH-4": faultAgentAfterProof,
		"CRASH-5": faultAgentBeforeConfirmation,
		"CRASH-6": faultServerAfterActivation,
	}
	seen := make(map[string]bool, len(wantBoundaries))
	for _, boundary := range surface.Boundaries {
		wantFault, ok := wantBoundaries[boundary.Case]
		if !ok || seen[boundary.Case] || boundary.Fault != wantFault ||
			boundary.Process == "" || boundary.Arrival == "" || boundary.Recovery == "" {
			t.Fatalf("bad crash boundary: %+v", boundary)
		}
		seen[boundary.Case] = true
	}
	for id := range wantBoundaries {
		if !seen[id] {
			t.Errorf("%s has no crash boundary", id)
		}
	}
}
