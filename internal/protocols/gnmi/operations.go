package gnmi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

// Capabilities retrieves the gNMI capabilities from the target.
func (a *Adapter) Capabilities(ctx context.Context) (*protocols.GNMICapabilitiesResult, error) {
	a.mu.RLock()
	client := a.gnmiClient
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	resp, err := client.Capabilities(ctx, &gnmipb.CapabilityRequest{})
	if err != nil {
		return nil, fmt.Errorf("capabilities RPC failed: %w", err)
	}

	result := &protocols.GNMICapabilitiesResult{
		GNMIVersion: resp.GetGNMIVersion(),
	}

	for _, m := range resp.GetSupportedModels() {
		result.SupportedModels = append(result.SupportedModels, protocols.GNMIModelData{
			Name:         m.GetName(),
			Organization: m.GetOrganization(),
			Version:      m.GetVersion(),
		})
	}

	for _, e := range resp.GetSupportedEncodings() {
		result.SupportedEncodings = append(result.SupportedEncodings, e.String())
	}

	return result, nil
}

// Get retrieves data from the specified paths.
func (a *Adapter) Get(ctx context.Context, paths []protocols.GNMIPath, opts *protocols.GNMIGetOptions) (*protocols.GNMIGetResult, error) {
	a.mu.RLock()
	client := a.gnmiClient
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	req := &gnmipb.GetRequest{}
	for _, p := range paths {
		req.Path = append(req.Path, ToProtoPath(p))
	}

	if opts != nil {
		if opts.Encoding != "" {
			enc, ok := encodingMap[opts.Encoding]
			if ok {
				req.Encoding = enc
			}
		}
		if opts.DataType != "" {
			dt, ok := dataTypeMap[opts.DataType]
			if ok {
				req.Type = dt
			}
		}
	}

	resp, err := client.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get RPC failed: %w", err)
	}

	result := &protocols.GNMIGetResult{}
	for _, n := range resp.GetNotification() {
		result.Notifications = append(result.Notifications, FromProtoNotification(n))
	}

	return result, nil
}

// Set modifies data on the target.
func (a *Adapter) Set(ctx context.Context, req *protocols.GNMISetRequest) (*protocols.GNMISetResult, error) {
	a.mu.RLock()
	client := a.gnmiClient
	a.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	protoReq := &gnmipb.SetRequest{}

	for _, d := range req.Delete {
		protoReq.Delete = append(protoReq.Delete, ToProtoPath(d))
	}

	for _, r := range req.Replace {
		protoReq.Replace = append(protoReq.Replace, &gnmipb.Update{
			Path: ToProtoPath(r.Path),
			Val:  ToProtoValue(r.Value),
		})
	}

	for _, u := range req.Update {
		protoReq.Update = append(protoReq.Update, &gnmipb.Update{
			Path: ToProtoPath(u.Path),
			Val:  ToProtoValue(u.Value),
		})
	}

	resp, err := client.Set(ctx, protoReq)
	if err != nil {
		return nil, fmt.Errorf("set RPC failed: %w", err)
	}

	result := &protocols.GNMISetResult{
		Timestamp: resp.GetTimestamp(),
	}

	for _, r := range resp.GetResponse() {
		result.Results = append(result.Results, protocols.GNMIUpdateResult{
			Path: FromProtoPath(r.GetPath()),
			Op:   r.GetOp().String(),
		})
	}

	return result, nil
}

// executeCommand parses and executes a command string.
func (a *Adapter) executeCommand(ctx context.Context, cmd string) ([]byte, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	switch parts[0] {
	case "capabilities":
		return a.execCapabilities(ctx)
	case "get":
		if len(parts) < 2 {
			return nil, fmt.Errorf("get requires a path argument")
		}
		return a.execGet(ctx, parts[1:])
	case "set":
		if len(parts) < 3 {
			return nil, fmt.Errorf("set requires: set <update|replace|delete> <path> [value]")
		}
		return a.execSet(ctx, parts[1:])
	case "subscribe":
		if len(parts) < 3 {
			return nil, fmt.Errorf("subscribe requires: subscribe <stream|once|poll> <path> [interval]")
		}
		return a.execSubscribe(ctx, parts[1:])
	default:
		return nil, fmt.Errorf("unknown command: %s (supported: capabilities, get, set, subscribe)", parts[0])
	}
}

func (a *Adapter) execCapabilities(ctx context.Context) ([]byte, error) {
	result, err := a.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (a *Adapter) execGet(ctx context.Context, args []string) ([]byte, error) {
	path := ParseStringPath(args[0])
	paths := []protocols.GNMIPath{path}

	var opts *protocols.GNMIGetOptions
	if len(args) > 1 {
		opts = &protocols.GNMIGetOptions{Encoding: args[1]}
	}

	result, err := a.Get(ctx, paths, opts)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (a *Adapter) execSet(ctx context.Context, args []string) ([]byte, error) {
	op := args[0]
	path := ParseStringPath(args[1])

	req := &protocols.GNMISetRequest{}

	switch op {
	case "delete":
		req.Delete = []protocols.GNMIPath{path}
	case "replace":
		if len(args) < 3 {
			return nil, fmt.Errorf("set replace requires a value")
		}
		req.Replace = []protocols.GNMIUpdate{{Path: path, Value: []byte(args[2])}}
	case "update":
		if len(args) < 3 {
			return nil, fmt.Errorf("set update requires a value")
		}
		req.Update = []protocols.GNMIUpdate{{Path: path, Value: []byte(args[2])}}
	default:
		return nil, fmt.Errorf("unknown set operation: %s (supported: update, replace, delete)", op)
	}

	result, err := a.Set(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (a *Adapter) execSubscribe(ctx context.Context, args []string) ([]byte, error) {
	mode := args[0]
	path := ParseStringPath(args[1])

	var sampleInterval int64
	if len(args) > 2 {
		d, err := time.ParseDuration(args[2])
		if err == nil {
			sampleInterval = d.Nanoseconds()
		}
	}

	req := &protocols.GNMISubscribeRequest{
		Paths: []protocols.GNMIPath{path},
		Mode:  mode,
	}
	if sampleInterval > 0 {
		req.StreamMode = StreamModeSample
		req.SampleInterval = sampleInterval
	}

	if mode == SubscriptionModeOnce {
		sub, err := a.Subscribe(ctx, req)
		if err != nil {
			return nil, err
		}
		defer sub.Close()

		var notifications []protocols.GNMINotification
		timeout := time.After(30 * time.Second)
		for {
			select {
			case n, ok := <-sub.Notifications():
				if !ok {
					return json.Marshal(notifications)
				}
				notifications = append(notifications, n)
			case <-sub.SyncComplete():
				return json.Marshal(notifications)
			case err, ok := <-sub.Errors():
				if ok {
					return nil, err
				}
			case <-timeout:
				return json.Marshal(notifications)
			case <-ctx.Done():
				return json.Marshal(notifications)
			}
		}
	}

	return nil, fmt.Errorf("stream and poll modes are not supported via Execute; use Subscribe() directly")
}

// Encoding and DataType lookup maps.
var encodingMap = map[string]gnmipb.Encoding{
	EncodingJSON:     gnmipb.Encoding_JSON,
	EncodingBytes:    gnmipb.Encoding_BYTES,
	EncodingProto:    gnmipb.Encoding_PROTO,
	EncodingASCII:    gnmipb.Encoding_ASCII,
	EncodingJSONIETF: gnmipb.Encoding_JSON_IETF,
}

var dataTypeMap = map[string]gnmipb.GetRequest_DataType{
	DataTypeAll:         gnmipb.GetRequest_ALL,
	DataTypeConfig:      gnmipb.GetRequest_CONFIG,
	DataTypeState:       gnmipb.GetRequest_STATE,
	DataTypeOperational: gnmipb.GetRequest_OPERATIONAL,
}
