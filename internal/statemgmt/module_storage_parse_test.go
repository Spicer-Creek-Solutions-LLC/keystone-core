package statemgmt

import "testing"

func TestDiskModuleParseConfigDefaults(t *testing.T) {
	module := NewDiskModule()

	decl := &StateDeclaration{
		ID: "disk1",
		Parameters: map[string]interface{}{
			"device": "/dev/sda",
			"flags":  []interface{}{"boot", "lvm"},
		},
	}

	config, err := module.parseConfig(decl)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.Device != "/dev/sda" {
		t.Errorf("unexpected device: %s", config.Device)
	}
	if config.PartitionNumber != 1 {
		t.Errorf("unexpected partition number: %d", config.PartitionNumber)
	}
	if config.Start != "0%" {
		t.Errorf("unexpected start: %s", config.Start)
	}
	if config.Type != "primary" {
		t.Errorf("unexpected type: %s", config.Type)
	}
	if config.Unit != "MiB" {
		t.Errorf("unexpected unit: %s", config.Unit)
	}
	if config.TableType != "gpt" {
		t.Errorf("unexpected table type: %s", config.TableType)
	}
	if len(config.Flags) != 2 || config.Flags[0] != "boot" || config.Flags[1] != "lvm" {
		t.Errorf("unexpected flags: %#v", config.Flags)
	}
}

func TestDiskModuleParseConfigMissingDevice(t *testing.T) {
	module := NewDiskModule()
	_, err := module.parseConfig(&StateDeclaration{ID: "disk1", Parameters: map[string]interface{}{}})
	if err == nil {
		t.Error("expected error for missing device")
	}
}

func TestFilesystemModuleParseConfig(t *testing.T) {
	module := NewFilesystemModule()

	_, err := module.parseConfig(&StateDeclaration{
		ID:         "/dev/sda1",
		State:      "present",
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Error("expected error for missing fstype on present state")
	}

	config, err := module.parseConfig(&StateDeclaration{
		ID:    "/dev/sda1",
		State: "present",
		Parameters: map[string]interface{}{
			"fstype": "ext4",
			"opts":   []interface{}{"-L", "data"},
		},
	})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.Device != "/dev/sda1" {
		t.Errorf("unexpected device: %s", config.Device)
	}
	if config.FSType != "ext4" {
		t.Errorf("unexpected fstype: %s", config.FSType)
	}
	if len(config.Options) != 2 || config.Options[0] != "-L" || config.Options[1] != "data" {
		t.Errorf("unexpected options: %#v", config.Options)
	}
}

func TestMountModuleParseConfig(t *testing.T) {
	module := NewMountModule()

	_, err := module.parseConfig(&StateDeclaration{
		ID:         "/mnt/data",
		Parameters: map[string]interface{}{},
	})
	if err == nil {
		t.Error("expected error for missing device")
	}

	config, err := module.parseConfig(&StateDeclaration{
		ID: "/mnt/data",
		Parameters: map[string]interface{}{
			"device": "/dev/sdb1",
			"opts":   "noatime,ro",
		},
	})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.Path != "/mnt/data" {
		t.Errorf("unexpected path: %s", config.Path)
	}
	if config.Device != "/dev/sdb1" {
		t.Errorf("unexpected device: %s", config.Device)
	}
	if len(config.Options) != 2 || config.Options[0] != "noatime" || config.Options[1] != "ro" {
		t.Errorf("unexpected options: %#v", config.Options)
	}
	if !config.Persist || !config.CreatePath {
		t.Errorf("unexpected defaults: persist=%v create_path=%v", config.Persist, config.CreatePath)
	}
	if config.Mode != "0755" {
		t.Errorf("unexpected mode: %s", config.Mode)
	}
}

func TestSwapModuleParseConfig(t *testing.T) {
	module := NewSwapModule()

	_, err := module.parseConfig(&StateDeclaration{ID: "", Parameters: map[string]interface{}{}})
	if err == nil {
		t.Error("expected error for missing path")
	}

	config, err := module.parseConfig(&StateDeclaration{
		ID:         "/swapfile",
		Parameters: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.Path != "/swapfile" {
		t.Errorf("unexpected path: %s", config.Path)
	}
	if config.Priority != -1 {
		t.Errorf("unexpected priority: %d", config.Priority)
	}
	if !config.Persist {
		t.Errorf("expected persist default true")
	}
}

func TestSystemdTimerParseConfig(t *testing.T) {
	module := NewSystemdTimerModule()

	_, err := module.parseConfig(&StateDeclaration{
		ID:         "cleanup",
		Parameters: map[string]interface{}{"on_calendar": "*-*-* *:*:00"},
	})
	if err == nil {
		t.Error("expected error for missing exec_start")
	}

	_, err = module.parseConfig(&StateDeclaration{
		ID:         "cleanup",
		Parameters: map[string]interface{}{"exec_start": "echo ok"},
	})
	if err == nil {
		t.Error("expected error for missing trigger")
	}

	config, err := module.parseConfig(&StateDeclaration{
		ID: "cleanup",
		Parameters: map[string]interface{}{
			"exec_start":  "/usr/bin/echo ok",
			"on_calendar": "*-*-* *:*:00",
			"environment": map[string]interface{}{
				"FOO": "bar",
			},
			"user_unit": true,
		},
	})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.ExecStart != "/usr/bin/echo ok" {
		t.Errorf("unexpected exec_start: %s", config.ExecStart)
	}
	if config.OnCalendar != "*-*-* *:*:00" {
		t.Errorf("unexpected on_calendar: %s", config.OnCalendar)
	}
	if config.Environment["FOO"] != "bar" {
		t.Errorf("unexpected environment: %#v", config.Environment)
	}
	if !config.UserUnit {
		t.Errorf("expected user_unit true")
	}
}
