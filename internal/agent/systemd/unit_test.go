// SPDX-License-Identifier: Apache-2.0

package systemd

import (
	"strings"
	"testing"
)

func TestRender_DefaultsHappyPath(t *testing.T) {
	body, err := Render(Params{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	s := string(body)

	wantContain := []string{
		"Description=Keystone Core Agent",
		"ExecStart=/usr/bin/kscore-agent --config /etc/kscore/agent.yaml",
		"Type=exec",
		"Restart=on-failure",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"ProtectHome=true",
		"PrivateTmp=true",
		"ReadWritePaths=/var/lib/kscore-agent /var/log/kscore-agent",
		"WantedBy=multi-user.target",
		"StateDirectory=kscore-agent",
	}
	for _, w := range wantContain {
		if !strings.Contains(s, w) {
			t.Errorf("rendered unit missing %q\n---\n%s", w, s)
		}
	}

	// Defaults to root — User/Group lines must NOT appear.
	if strings.Contains(s, "\nUser=") {
		t.Errorf("default render leaked User= line:\n%s", s)
	}
}

func TestRender_WithUserGroup(t *testing.T) {
	body, err := Render(Params{
		User:  "keystone-core",
		Group: "keystone-core",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "User=keystone-core") {
		t.Error("missing User=keystone-core")
	}
	if !strings.Contains(s, "Group=keystone-core") {
		t.Error("missing Group=keystone-core")
	}
}

func TestRender_WithExtraEnv(t *testing.T) {
	body, err := Render(Params{
		ExtraEnv: []string{
			"KSCORE_LOG_LEVEL=info",
			"KSCORE_FOO=bar",
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	s := string(body)
	for _, w := range []string{
		"Environment=KSCORE_LOG_LEVEL=info",
		"Environment=KSCORE_FOO=bar",
	} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q\n---\n%s", w, s)
		}
	}
}

func TestRender_WithEnvironmentFile(t *testing.T) {
	body, err := Render(Params{
		EnvironmentFile: "/etc/kscore/agent.env",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(body), "EnvironmentFile=/etc/kscore/agent.env") {
		t.Errorf("missing EnvironmentFile= line:\n%s", body)
	}
}

func TestRender_RejectsRelativeBinary(t *testing.T) {
	_, err := Render(Params{BinaryPath: "kscore-agent"})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v, want mention of absolute", err)
	}
}

func TestRender_RejectsRelativeConfig(t *testing.T) {
	_, err := Render(Params{ConfigPath: "agent.yaml"})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("err = %v, want mention of absolute", err)
	}
}

func TestRender_RejectsHalfSetUserGroup(t *testing.T) {
	_, err := Render(Params{User: "keystone-core"})
	if err == nil || !strings.Contains(err.Error(), "Group") {
		t.Errorf("err = %v, want User+Group together", err)
	}
}

func TestRender_RejectsBadExtraEnv(t *testing.T) {
	_, err := Render(Params{ExtraEnv: []string{"NO_EQUALS_HERE"}})
	if err == nil || !strings.Contains(err.Error(), "KEY=VALUE") {
		t.Errorf("err = %v, want KEY=VALUE format error", err)
	}
}

func TestRender_OverrideReadWritePaths(t *testing.T) {
	body, err := Render(Params{
		ReadWritePaths: []string{"/srv/keystone", "/data/keystone"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(body), "ReadWritePaths=/srv/keystone /data/keystone") {
		t.Errorf("RW path override missing:\n%s", body)
	}
}
