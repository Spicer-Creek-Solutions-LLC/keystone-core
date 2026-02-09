package bootstrap

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestConfigurePhaseVerbose(t *testing.T) {
	buf := new(bytes.Buffer)
	state := &State{
		Output:  buf,
		Verbose: true,
		BootstrapConfig: &Config{
			Mode:        "demo",
			ClusterName: "keystone",
			NodeRole:    "both",
		},
	}

	if err := configurePhase(context.Background(), state); err != nil {
		t.Fatalf("configurePhase returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bootstrap configuration") {
		t.Fatalf("expected configuration output, got %s", output)
	}
}
