package gnmi

import (
	"context"
	"io"
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestAdapter_Subscribe_Once(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SubscribeFunc = func(stream gnmipb.GNMI_SubscribeServer) error {
		// Read the subscribe request
		_, err := stream.Recv()
		if err != nil {
			return err
		}

		// Send a notification
		if err := stream.Send(&gnmipb.SubscribeResponse{
			Response: &gnmipb.SubscribeResponse_Update{
				Update: &gnmipb.Notification{
					Timestamp: time.Now().UnixNano(),
					Update: []*gnmipb.Update{
						{
							Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "hostname"}}},
							Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_StringVal{StringVal: "router-01"}},
						},
					},
				},
			},
		}); err != nil {
			return err
		}

		// Send sync response
		if err := stream.Send(&gnmipb.SubscribeResponse{
			Response: &gnmipb.SubscribeResponse_SyncResponse{SyncResponse: true},
		}); err != nil {
			return err
		}

		return nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	sub, err := adapter.Subscribe(ctx, &protocols.GNMISubscribeRequest{
		Paths: []protocols.GNMIPath{{Elements: []string{"system", "config", "hostname"}}},
		Mode:  SubscriptionModeOnce,
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Close()

	// Should receive notification
	select {
	case n := <-sub.Notifications():
		if len(n.Updates) != 1 {
			t.Fatalf("expected 1 update, got %d", len(n.Updates))
		}
		if string(n.Updates[0].Value) != "router-01" {
			t.Errorf("expected value router-01, got %s", n.Updates[0].Value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	// Should receive sync complete
	select {
	case <-sub.SyncComplete():
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sync complete")
	}
}

func TestAdapter_Subscribe_Stream(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SubscribeFunc = func(stream gnmipb.GNMI_SubscribeServer) error {
		// Read subscribe request
		_, err := stream.Recv()
		if err != nil {
			return err
		}

		// Send 3 notifications
		for i := 0; i < 3; i++ {
			if err := stream.Send(&gnmipb.SubscribeResponse{
				Response: &gnmipb.SubscribeResponse_Update{
					Update: &gnmipb.Notification{
						Timestamp: time.Now().UnixNano(),
						Update: []*gnmipb.Update{
							{
								Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "counter"}}},
								Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_IntVal{IntVal: int64(i)}},
							},
						},
					},
				},
			}); err != nil {
				return err
			}
		}

		// Send sync
		if err := stream.Send(&gnmipb.SubscribeResponse{
			Response: &gnmipb.SubscribeResponse_SyncResponse{SyncResponse: true},
		}); err != nil {
			return err
		}

		// Keep alive until client disconnects
		<-stream.Context().Done()
		return nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	sub, err := adapter.Subscribe(ctx, &protocols.GNMISubscribeRequest{
		Paths:      []protocols.GNMIPath{{Elements: []string{"counters"}}},
		Mode:       SubscriptionModeStream,
		StreamMode: StreamModeSample,
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Close()

	// Collect 3 notifications
	received := 0
	timeout := time.After(5 * time.Second)
	for received < 3 {
		select {
		case <-sub.Notifications():
			received++
		case <-timeout:
			t.Fatalf("timeout after receiving %d notifications", received)
		}
	}

	if received != 3 {
		t.Errorf("expected 3 notifications, got %d", received)
	}
}

func TestAdapter_Subscribe_ServerError(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SubscribeFunc = func(stream gnmipb.GNMI_SubscribeServer) error {
		_, err := stream.Recv()
		if err != nil {
			return err
		}
		return status.Errorf(codes.Internal, "server error")
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	sub, err := adapter.Subscribe(ctx, &protocols.GNMISubscribeRequest{
		Paths: []protocols.GNMIPath{{Elements: []string{"system"}}},
		Mode:  SubscriptionModeOnce,
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Close()

	// Should receive an error
	select {
	case err := <-sub.Errors():
		if err == nil {
			t.Error("expected non-nil error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error")
	}

	// Done channel should close
	select {
	case <-sub.Done():
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for done")
	}
}

func TestAdapter_Subscribe_Close(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SubscribeFunc = func(stream gnmipb.GNMI_SubscribeServer) error {
		_, err := stream.Recv()
		if err != nil {
			return err
		}
		<-stream.Context().Done()
		return nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	sub, err := adapter.Subscribe(ctx, &protocols.GNMISubscribeRequest{
		Paths: []protocols.GNMIPath{{Elements: []string{"system"}}},
		Mode:  SubscriptionModeStream,
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Close the subscription
	sub.Close()

	// Done should eventually close (stream recv gets cancelled)
	select {
	case <-sub.Done():
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for done after close")
	}
}

func TestAdapter_Subscribe_NotConnected(t *testing.T) {
	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	_, err := adapter.Subscribe(context.Background(), &protocols.GNMISubscribeRequest{})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapter_ExecuteCommand_SubscribeOnce(t *testing.T) {
	certs := generateTestCerts(t)
	mock := startMockServer(t, certs)

	mock.SubscribeFunc = func(stream gnmipb.GNMI_SubscribeServer) error {
		_, err := stream.Recv()
		if err != nil {
			return err
		}

		// Send one notification then sync
		if err := stream.Send(&gnmipb.SubscribeResponse{
			Response: &gnmipb.SubscribeResponse_Update{
				Update: &gnmipb.Notification{
					Timestamp: 12345,
				},
			},
		}); err != nil {
			return err
		}

		if err := stream.Send(&gnmipb.SubscribeResponse{
			Response: &gnmipb.SubscribeResponse_SyncResponse{SyncResponse: true},
		}); err != nil {
			return err
		}

		return nil
	}

	adapter := &Adapter{config: DefaultConfig(), metrics: &protocols.AdapterMetrics{}}
	ctx := context.Background()

	if err := adapter.Connect(ctx, makeTestDevice(mock.addr), makeTestCredential(certs)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer adapter.Disconnect(ctx) //nolint:errcheck

	result, err := adapter.Execute(ctx, &protocols.ExecuteRequest{
		Command: "subscribe once /system/config/hostname",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (error: %s)", result.ExitCode, result.Error)
	}
}

func TestAdapter_ExecuteCommand_SubscribeStreamUnsupported(t *testing.T) {
	adapter := &Adapter{
		config:    DefaultConfig(),
		metrics:   &protocols.AdapterMetrics{},
		connected: true,
	}

	// Stream mode via Execute is not supported
	_, err := adapter.executeCommand(context.Background(), "subscribe stream /some/path")
	if err == nil {
		t.Error("expected error for stream mode via Execute")
	}
}

func TestGNMISubscription_Methods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sub := protocols.NewGNMISubscription(cancel)

	// Test channels are accessible
	if sub.Notifications() == nil {
		t.Error("expected non-nil notifications channel")
	}
	if sub.Errors() == nil {
		t.Error("expected non-nil errors channel")
	}
	if sub.SyncComplete() == nil {
		t.Error("expected non-nil sync channel")
	}
	if sub.Done() == nil {
		t.Error("expected non-nil done channel")
	}

	// Test SendNotification
	sub.SendNotification(protocols.GNMINotification{Timestamp: 1234})
	select {
	case n := <-sub.Notifications():
		if n.Timestamp != 1234 {
			t.Errorf("expected timestamp 1234, got %d", n.Timestamp)
		}
	default:
		t.Error("expected notification to be available")
	}

	// Test SendError
	sub.SendError(io.EOF)
	select {
	case err := <-sub.Errors():
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
	default:
		t.Error("expected error to be available")
	}

	// Test Close
	sub.Close()
	<-ctx.Done() // Context should be cancelled
}
