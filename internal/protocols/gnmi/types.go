// Package gnmi implements the gNMI protocol adapter for streaming telemetry
// and configuration management on modern network devices.
package gnmi

import "time"

// DefaultPort is the default gNMI gRPC port.
const DefaultPort = 9339

// Encoding constants for gNMI data serialization.
const (
	EncodingJSON     = "json"
	EncodingBytes    = "bytes"
	EncodingProto    = "proto"
	EncodingASCII    = "ascii"
	EncodingJSONIETF = "json_ietf"
)

// DataType constants for Get request filtering.
const (
	DataTypeAll         = "all"
	DataTypeConfig      = "config"
	DataTypeState       = "state"
	DataTypeOperational = "operational"
)

// SubscriptionMode constants for Subscribe RPC.
const (
	SubscriptionModeStream = "stream"
	SubscriptionModeOnce   = "once"
	SubscriptionModePoll   = "poll"
)

// StreamMode constants for streaming subscriptions.
const (
	StreamModeTargetDefined = "target_defined"
	StreamModeOnChange      = "on_change"
	StreamModeSample        = "sample"
)

// DefaultTimeout is the default gRPC call timeout.
const DefaultTimeout = 30 * time.Second

// DefaultSubscriptionBuffer is the default buffer size for notification channels.
const DefaultSubscriptionBuffer = 100
