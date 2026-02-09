package gnmi

import (
	"context"
	"fmt"
	"io"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

// Subscribe creates a gNMI subscription for streaming telemetry.
func (a *Adapter) Subscribe(ctx context.Context, req *protocols.GNMISubscribeRequest) (*protocols.GNMISubscription, error) {
	a.mu.RLock()
	client := a.gnmiClient
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	subCtx, cancel := context.WithCancel(ctx)

	stream, err := client.Subscribe(subCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe RPC failed: %w", err)
	}

	var subscriptions []*gnmipb.Subscription
	for _, p := range req.Paths {
		sub := &gnmipb.Subscription{
			Path: ToProtoPath(p),
		}
		if req.StreamMode != "" {
			if sm, ok := streamModeMap[req.StreamMode]; ok {
				sub.Mode = sm
			}
		}
		if req.SampleInterval > 0 {
			//nolint:gosec // G115: sample interval is bounded by duration semantics
			sub.SampleInterval = uint64(req.SampleInterval)
		}
		subscriptions = append(subscriptions, sub)
	}

	subMode, ok := subscriptionModeMap[req.Mode]
	if !ok {
		subMode = gnmipb.SubscriptionList_STREAM
	}

	subReq := &gnmipb.SubscribeRequest{
		Request: &gnmipb.SubscribeRequest_Subscribe{
			Subscribe: &gnmipb.SubscriptionList{
				Subscription: subscriptions,
				Mode:         subMode,
			},
		},
	}

	if req.Encoding != "" {
		if enc, ok := encodingMap[req.Encoding]; ok {
			subReq.GetSubscribe().Encoding = enc
		}
	}

	if err := stream.Send(subReq); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to send subscribe request: %w", err)
	}

	sub := protocols.NewGNMISubscription(cancel)

	go a.receiveSubscription(stream, sub)

	return sub, nil
}

// receiveSubscription reads messages from the gNMI subscribe stream.
func (a *Adapter) receiveSubscription(stream gnmipb.GNMI_SubscribeClient, sub *protocols.GNMISubscription) {
	defer sub.CloseDone()

	syncClosed := false

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				sub.SendError(err)
			}
			return
		}

		switch r := resp.GetResponse().(type) {
		case *gnmipb.SubscribeResponse_Update:
			notif := FromProtoNotification(r.Update)
			sub.SendNotification(notif)

		case *gnmipb.SubscribeResponse_SyncResponse:
			if !syncClosed {
				sub.CloseSyncComplete()
				syncClosed = true
			}
		}
	}
}

var streamModeMap = map[string]gnmipb.SubscriptionMode{
	StreamModeTargetDefined: gnmipb.SubscriptionMode_TARGET_DEFINED,
	StreamModeOnChange:      gnmipb.SubscriptionMode_ON_CHANGE,
	StreamModeSample:        gnmipb.SubscriptionMode_SAMPLE,
}

var subscriptionModeMap = map[string]gnmipb.SubscriptionList_Mode{
	SubscriptionModeStream: gnmipb.SubscriptionList_STREAM,
	SubscriptionModeOnce:   gnmipb.SubscriptionList_ONCE,
	SubscriptionModePoll:   gnmipb.SubscriptionList_POLL,
}
