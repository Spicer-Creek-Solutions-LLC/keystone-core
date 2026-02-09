package gnmi

import (
	"context"
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestAdapter_Capabilities(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.CapabilitiesFunc = func(_ *gnmipb.CapabilityRequest) (*gnmipb.CapabilityResponse, error) {
		return &gnmipb.CapabilityResponse{
			SupportedModels: []*gnmipb.ModelData{
				{Name: "openconfig-interfaces", Organization: "OpenConfig", Version: "3.0.0"},
				{Name: "openconfig-bgp", Organization: "OpenConfig", Version: "9.0.0"},
			},
			SupportedEncodings: []gnmipb.Encoding{gnmipb.Encoding_JSON_IETF, gnmipb.Encoding_PROTO},
			GNMIVersion:        "0.10.0",
		}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	result, err := adapter.Capabilities(ctx)
	if err != nil {
		t.Fatalf("capabilities failed: %v", err)
	}

	if result.GNMIVersion != "0.10.0" {
		t.Errorf("expected gNMI version 0.10.0, got %s", result.GNMIVersion)
	}
	if len(result.SupportedModels) != 2 {
		t.Errorf("expected 2 models, got %d", len(result.SupportedModels))
	}
	if result.SupportedModels[0].Name != "openconfig-interfaces" {
		t.Errorf("expected first model name openconfig-interfaces, got %s", result.SupportedModels[0].Name)
	}
	if len(result.SupportedEncodings) != 2 {
		t.Errorf("expected 2 encodings, got %d", len(result.SupportedEncodings))
	}
}

func TestAdapter_Capabilities_NotConnected(t *testing.T) {
	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	_, err := adapter.Capabilities(context.Background())
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapter_Get(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.GetFunc = func(req *gnmipb.GetRequest) (*gnmipb.GetResponse, error) {
		return &gnmipb.GetResponse{
			Notification: []*gnmipb.Notification{
				{
					Timestamp: time.Now().UnixNano(),
					Update: []*gnmipb.Update{
						{
							Path: req.GetPath()[0],
							Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"hostname":"router-01"}`)}},
						},
					},
				},
			},
		}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	paths := []protocols.GNMIPath{
		{Elements: []string{"system", "config", "hostname"}},
	}

	result, err := adapter.Get(ctx, paths, nil)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if len(result.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(result.Notifications))
	}
	if len(result.Notifications[0].Updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(result.Notifications[0].Updates))
	}
	if string(result.Notifications[0].Updates[0].Value) != `{"hostname":"router-01"}` {
		t.Errorf("unexpected value: %s", result.Notifications[0].Updates[0].Value)
	}
}

func TestAdapter_Get_WithOptions(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	var capturedReq *gnmipb.GetRequest
	mock.GetFunc = func(req *gnmipb.GetRequest) (*gnmipb.GetResponse, error) {
		capturedReq = req
		return &gnmipb.GetResponse{}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	paths := []protocols.GNMIPath{{Elements: []string{"interfaces"}}}
	opts := &protocols.GNMIGetOptions{
		Encoding: EncodingJSONIETF,
		DataType: DataTypeConfig,
	}

	_, err := adapter.Get(ctx, paths, opts)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if capturedReq.GetEncoding() != gnmipb.Encoding_JSON_IETF {
		t.Errorf("expected encoding JSON_IETF, got %s", capturedReq.GetEncoding())
	}
	if capturedReq.GetType() != gnmipb.GetRequest_CONFIG {
		t.Errorf("expected type CONFIG, got %s", capturedReq.GetType())
	}
}

func TestAdapter_Get_NotConnected(t *testing.T) {
	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	_, err := adapter.Get(context.Background(), nil, nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapter_Set(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SetFunc = func(req *gnmipb.SetRequest) (*gnmipb.SetResponse, error) {
		return &gnmipb.SetResponse{
			Timestamp: time.Now().UnixNano(),
			Response: []*gnmipb.UpdateResult{
				{
					Path: req.GetUpdate()[0].GetPath(),
					Op:   gnmipb.UpdateResult_UPDATE,
				},
			},
		}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	setReq := &protocols.GNMISetRequest{
		Update: []protocols.GNMIUpdate{
			{
				Path:  protocols.GNMIPath{Elements: []string{"system", "config", "hostname"}},
				Value: []byte(`"new-hostname"`),
			},
		},
	}

	result, err := adapter.Set(ctx, setReq)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Op != gnmipb.UpdateResult_UPDATE.String() {
		t.Errorf("expected op UPDATE, got %s", result.Results[0].Op)
	}
}

func TestAdapter_Set_Delete(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	var capturedReq *gnmipb.SetRequest
	mock.SetFunc = func(req *gnmipb.SetRequest) (*gnmipb.SetResponse, error) {
		capturedReq = req
		return &gnmipb.SetResponse{
			Response: []*gnmipb.UpdateResult{
				{Op: gnmipb.UpdateResult_DELETE},
			},
		}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	setReq := &protocols.GNMISetRequest{
		Delete: []protocols.GNMIPath{
			{Elements: []string{"interfaces", "interface[name=eth99]"}},
		},
	}

	_, err := adapter.Set(ctx, setReq)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if len(capturedReq.GetDelete()) != 1 {
		t.Errorf("expected 1 delete path, got %d", len(capturedReq.GetDelete()))
	}
}

func TestAdapter_Set_NotConnected(t *testing.T) {
	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	_, err := adapter.Set(context.Background(), &protocols.GNMISetRequest{})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapter_ExecuteCommand_Capabilities(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{Command: "capabilities"})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if len(result.Stdout) == 0 {
		t.Error("expected non-empty stdout")
	}
}

func TestAdapter_ExecuteCommand_Get(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.GetFunc = func(_ *gnmipb.GetRequest) (*gnmipb.GetResponse, error) {
		return &gnmipb.GetResponse{
			Notification: []*gnmipb.Notification{
				{Timestamp: 12345},
			},
		}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{Command: "get /interfaces/interface"})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (error: %s)", result.ExitCode, result.Error)
	}
}

func TestAdapter_ExecuteCommand_SetUpdate(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SetFunc = func(_ *gnmipb.SetRequest) (*gnmipb.SetResponse, error) {
		return &gnmipb.SetResponse{}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{
		Command: `set update /system/config/hostname "new-host"`,
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (error: %s)", result.ExitCode, result.Error)
	}
}

func TestAdapter_ExecuteCommand_SetDelete(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SetFunc = func(_ *gnmipb.SetRequest) (*gnmipb.SetResponse, error) {
		return &gnmipb.SetResponse{}, nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{
		Command: "set delete /interfaces/interface[name=eth99]",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (error: %s)", result.ExitCode, result.Error)
	}
}

func TestAdapter_ExecuteCommand_Errors(t *testing.T) {
	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}, connected: true}

	tests := []struct {
		name    string
		command string
	}{
		{"empty command", ""},
		{"unknown command", "unknown-cmd"},
		{"get missing path", "get"},
		{"set missing args", "set"},
		{"set missing value", "set update"},
		{"subscribe missing args", "subscribe"},
		{"subscribe missing path", "subscribe stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adapter.executeCommand(context.Background(), tt.command)
			if err == nil {
				t.Errorf("expected error for command %q", tt.command)
			}
		})
	}
}
