package gnmi

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/protocols"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

func TestParseStringPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected protocols.GNMIPath
	}{
		{
			name:     "empty path",
			input:    "",
			expected: protocols.GNMIPath{},
		},
		{
			name:     "root path",
			input:    "/",
			expected: protocols.GNMIPath{},
		},
		{
			name:  "simple path",
			input: "/interfaces/interface",
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
			},
		},
		{
			name:  "path with keys",
			input: "/interfaces/interface[name=eth0]/state/counters",
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface[name=eth0]", "state", "counters"},
			},
		},
		{
			name:  "path with multiple keys",
			input: "/network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]",
			expected: protocols.GNMIPath{
				Elements: []string{
					"network-instances",
					"network-instance[name=default]",
					"protocols",
					"protocol[identifier=BGP][name=bgp]",
				},
			},
		},
		{
			name:  "path with origin",
			input: "openconfig:/interfaces/interface",
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
				Origin:   "openconfig",
			},
		},
		{
			name:  "no leading slash",
			input: "interfaces/interface",
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
			},
		},
		{
			name:  "single element",
			input: "/interfaces",
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces"},
			},
		},
		{
			name:  "whitespace trimming",
			input: "  /interfaces/interface  ",
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
			},
		},
		{
			name:     "origin only",
			input:    "openconfig:",
			expected: protocols.GNMIPath{Origin: "openconfig"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseStringPath(tt.input)
			if result.Origin != tt.expected.Origin {
				t.Errorf("Origin: expected %q, got %q", tt.expected.Origin, result.Origin)
			}
			if len(result.Elements) != len(tt.expected.Elements) {
				t.Fatalf("Elements length: expected %d, got %d (%v)", len(tt.expected.Elements), len(result.Elements), result.Elements)
			}
			for i := range result.Elements {
				if result.Elements[i] != tt.expected.Elements[i] {
					t.Errorf("Element[%d]: expected %q, got %q", i, tt.expected.Elements[i], result.Elements[i])
				}
			}
		})
	}
}

func TestPathToString(t *testing.T) {
	tests := []struct {
		name     string
		input    protocols.GNMIPath
		expected string
	}{
		{
			name:     "empty path",
			input:    protocols.GNMIPath{},
			expected: "/",
		},
		{
			name: "simple path",
			input: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
			},
			expected: "/interfaces/interface",
		},
		{
			name: "path with keys",
			input: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface[name=eth0]"},
			},
			expected: "/interfaces/interface[name=eth0]",
		},
		{
			name: "path with origin",
			input: protocols.GNMIPath{
				Elements: []string{"interfaces"},
				Origin:   "openconfig",
			},
			expected: "openconfig:/interfaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PathToString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseStringPath_RoundTrip(t *testing.T) {
	paths := []string{
		"/interfaces/interface",
		"/interfaces/interface[name=eth0]/state/counters",
		"/system/config/hostname",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			parsed := ParseStringPath(p)
			result := PathToString(parsed)
			if result != p {
				t.Errorf("round-trip failed: input %q, output %q", p, result)
			}
		})
	}
}

func TestToProtoPath(t *testing.T) {
	tests := []struct {
		name     string
		input    protocols.GNMIPath
		expected *gnmipb.Path
	}{
		{
			name:     "empty path",
			input:    protocols.GNMIPath{},
			expected: &gnmipb.Path{},
		},
		{
			name: "simple path",
			input: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
			},
			expected: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{
					{Name: "interfaces"},
					{Name: "interface"},
				},
			},
		},
		{
			name: "path with keys",
			input: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface[name=eth0]"},
			},
			expected: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": "eth0"}},
				},
			},
		},
		{
			name: "path with origin and target",
			input: protocols.GNMIPath{
				Elements: []string{"interfaces"},
				Origin:   "openconfig",
				Target:   "router-01",
			},
			expected: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{
					{Name: "interfaces"},
				},
				Origin: "openconfig",
				Target: "router-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToProtoPath(tt.input)
			if result.GetOrigin() != tt.expected.GetOrigin() {
				t.Errorf("Origin: expected %q, got %q", tt.expected.GetOrigin(), result.GetOrigin())
			}
			if result.GetTarget() != tt.expected.GetTarget() {
				t.Errorf("Target: expected %q, got %q", tt.expected.GetTarget(), result.GetTarget())
			}
			if len(result.GetElem()) != len(tt.expected.GetElem()) {
				t.Fatalf("Elem length: expected %d, got %d", len(tt.expected.GetElem()), len(result.GetElem()))
			}
			for i, elem := range result.GetElem() {
				exp := tt.expected.GetElem()[i]
				if elem.GetName() != exp.GetName() {
					t.Errorf("Elem[%d].Name: expected %q, got %q", i, exp.GetName(), elem.GetName())
				}
				for k, v := range exp.GetKey() {
					if elem.GetKey()[k] != v {
						t.Errorf("Elem[%d].Key[%s]: expected %q, got %q", i, k, v, elem.GetKey()[k])
					}
				}
			}
		})
	}
}

func TestFromProtoPath(t *testing.T) {
	tests := []struct {
		name     string
		input    *gnmipb.Path
		expected protocols.GNMIPath
	}{
		{
			name:     "nil path",
			input:    nil,
			expected: protocols.GNMIPath{},
		},
		{
			name: "simple path",
			input: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{
					{Name: "interfaces"},
					{Name: "interface"},
				},
			},
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface"},
			},
		},
		{
			name: "path with keys",
			input: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": "eth0"}},
				},
			},
			expected: protocols.GNMIPath{
				Elements: []string{"interfaces", "interface[name=eth0]"},
			},
		},
		{
			name: "path with origin",
			input: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{
					{Name: "system"},
				},
				Origin: "openconfig",
			},
			expected: protocols.GNMIPath{
				Elements: []string{"system"},
				Origin:   "openconfig",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromProtoPath(tt.input)
			if result.Origin != tt.expected.Origin {
				t.Errorf("Origin: expected %q, got %q", tt.expected.Origin, result.Origin)
			}
			if len(result.Elements) != len(tt.expected.Elements) {
				t.Fatalf("Elements length: expected %d, got %d", len(tt.expected.Elements), len(result.Elements))
			}
			for i := range result.Elements {
				if result.Elements[i] != tt.expected.Elements[i] {
					t.Errorf("Element[%d]: expected %q, got %q", i, tt.expected.Elements[i], result.Elements[i])
				}
			}
		})
	}
}

func TestProtoPathRoundTrip(t *testing.T) {
	original := protocols.GNMIPath{
		Elements: []string{"interfaces", "interface[name=eth0]", "state"},
		Origin:   "openconfig",
		Target:   "router-01",
	}

	protoPath := ToProtoPath(original)
	result := FromProtoPath(protoPath)

	if result.Origin != original.Origin {
		t.Errorf("Origin: expected %q, got %q", original.Origin, result.Origin)
	}
	if result.Target != original.Target {
		t.Errorf("Target: expected %q, got %q", original.Target, result.Target)
	}
	if len(result.Elements) != len(original.Elements) {
		t.Fatalf("Elements length: expected %d, got %d", len(original.Elements), len(result.Elements))
	}
	for i := range result.Elements {
		if result.Elements[i] != original.Elements[i] {
			t.Errorf("Element[%d]: expected %q, got %q", i, original.Elements[i], result.Elements[i])
		}
	}
}

func TestToProtoValue(t *testing.T) {
	data := []byte(`{"enabled": true}`)
	tv := ToProtoValue(data)

	if tv == nil {
		t.Fatal("expected non-nil TypedValue")
	}

	jsonVal, ok := tv.GetValue().(*gnmipb.TypedValue_JsonIetfVal)
	if !ok {
		t.Fatalf("expected JsonIetfVal, got %T", tv.GetValue())
	}
	if string(jsonVal.JsonIetfVal) != string(data) {
		t.Errorf("expected %s, got %s", data, jsonVal.JsonIetfVal)
	}
}

func TestToProtoValue_Nil(t *testing.T) {
	tv := ToProtoValue(nil)
	if tv != nil {
		t.Error("expected nil TypedValue for nil input")
	}
}

func TestFromProtoValue(t *testing.T) {
	tests := []struct {
		name     string
		input    *gnmipb.TypedValue
		expected string
		wantErr  bool
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "",
		},
		{
			name: "json_ietf value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"enabled":true}`)},
			},
			expected: `{"enabled":true}`,
		},
		{
			name: "json value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_JsonVal{JsonVal: []byte(`"hello"`)},
			},
			expected: `"hello"`,
		},
		{
			name: "string value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_StringVal{StringVal: "hostname"},
			},
			expected: "hostname",
		},
		{
			name: "int value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_IntVal{IntVal: 42},
			},
			expected: "42",
		},
		{
			name: "uint value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_UintVal{UintVal: 100},
			},
			expected: "100",
		},
		{
			name: "bool value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_BoolVal{BoolVal: true},
			},
			expected: "true",
		},
		{
			name: "ascii value",
			input: &gnmipb.TypedValue{
				Value: &gnmipb.TypedValue_AsciiVal{AsciiVal: "text-output"},
			},
			expected: "text-output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromProtoValue(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error: expected %v, got %v", tt.wantErr, err)
			}
			if tt.expected == "" && result == nil {
				return
			}
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestFromProtoNotification(t *testing.T) {
	t.Run("nil notification", func(t *testing.T) {
		result := FromProtoNotification(nil)
		if result.Timestamp != 0 {
			t.Error("expected zero timestamp for nil notification")
		}
	})

	t.Run("notification with updates and deletes", func(t *testing.T) {
		n := &gnmipb.Notification{
			Timestamp: 1234567890,
			Prefix: &gnmipb.Path{
				Elem: []*gnmipb.PathElem{{Name: "interfaces"}},
			},
			Update: []*gnmipb.Update{
				{
					Path: &gnmipb.Path{
						Elem: []*gnmipb.PathElem{{Name: "interface", Key: map[string]string{"name": "eth0"}}},
					},
					Val: &gnmipb.TypedValue{
						Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"enabled":true}`)},
					},
				},
			},
			Delete: []*gnmipb.Path{
				{Elem: []*gnmipb.PathElem{{Name: "interface", Key: map[string]string{"name": "eth99"}}}},
			},
		}

		result := FromProtoNotification(n)

		if result.Timestamp != 1234567890 {
			t.Errorf("Timestamp: expected 1234567890, got %d", result.Timestamp)
		}
		if len(result.Prefix.Elements) != 1 || result.Prefix.Elements[0] != "interfaces" {
			t.Errorf("unexpected prefix: %v", result.Prefix)
		}
		if len(result.Updates) != 1 {
			t.Fatalf("expected 1 update, got %d", len(result.Updates))
		}
		if string(result.Updates[0].Value) != `{"enabled":true}` {
			t.Errorf("unexpected update value: %s", result.Updates[0].Value)
		}
		if len(result.Deletes) != 1 {
			t.Fatalf("expected 1 delete, got %d", len(result.Deletes))
		}
	})
}

func TestParsePathElement(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantName   string
		wantKeys   map[string]string
	}{
		{
			name:     "no keys",
			input:    "interfaces",
			wantName: "interfaces",
			wantKeys: nil,
		},
		{
			name:     "single key",
			input:    "interface[name=eth0]",
			wantName: "interface",
			wantKeys: map[string]string{"name": "eth0"},
		},
		{
			name:     "multiple keys",
			input:    "protocol[identifier=BGP][name=bgp]",
			wantName: "protocol",
			wantKeys: map[string]string{"identifier": "BGP", "name": "bgp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, keys := parsePathElement(tt.input)
			if name != tt.wantName {
				t.Errorf("name: expected %q, got %q", tt.wantName, name)
			}
			if tt.wantKeys == nil {
				if keys != nil {
					t.Errorf("expected nil keys, got %v", keys)
				}
				return
			}
			for k, v := range tt.wantKeys {
				if keys[k] != v {
					t.Errorf("key[%s]: expected %q, got %q", k, v, keys[k])
				}
			}
		})
	}
}
