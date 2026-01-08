// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"fmt"
	"strings"
)

// Subject categories for proxy operations.
const (
	// SubjectPrefix is the base prefix for all proxy subjects.
	SubjectPrefix = "kscore"

	// SubjectCategoryProxy is the category for proxy operations.
	SubjectCategoryProxy = "proxy"

	// SubjectCategoryDevice is the category for device operations.
	SubjectCategoryDevice = "device"

	// SubjectCategoryCredential is the category for credential operations.
	SubjectCategoryCredential = "credential"
)

// Subject operations for proxy.
const (
	// Device operations
	SubjectOpRegister   = "register"
	SubjectOpUnregister = "unregister"
	SubjectOpHeartbeat  = "heartbeat"
	SubjectOpStatus     = "status"
	SubjectOpCommand    = "command"
	SubjectOpResult     = "result"
	SubjectOpHealth     = "health"

	// Credential operations
	SubjectOpFetch   = "fetch"
	SubjectOpRefresh = "refresh"
)

// ProxySubjectBuilder builds NATS subjects for proxy operations.
type ProxySubjectBuilder struct {
	cluster string
}

// NewProxySubjectBuilder creates a new subject builder.
func NewProxySubjectBuilder(cluster string) *ProxySubjectBuilder {
	if cluster == "" {
		cluster = "default"
	}
	return &ProxySubjectBuilder{cluster: cluster}
}

// DeviceRegister returns the subject for device registration.
// Format: kscore.{cluster}.proxy.device.register
func (b *ProxySubjectBuilder) DeviceRegister() string {
	return fmt.Sprintf("%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, SubjectCategoryDevice, SubjectOpRegister)
}

// DeviceUnregister returns the subject for device unregistration.
// Format: kscore.{cluster}.proxy.device.unregister
func (b *ProxySubjectBuilder) DeviceUnregister() string {
	return fmt.Sprintf("%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, SubjectCategoryDevice, SubjectOpUnregister)
}

// DeviceHeartbeat returns the subject for device heartbeat.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.heartbeat
func (b *ProxySubjectBuilder) DeviceHeartbeat(proxyAgentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), SubjectOpHeartbeat)
}

// DeviceStatus returns the subject for device status updates.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.{deviceID}.status
func (b *ProxySubjectBuilder) DeviceStatus(proxyAgentID, deviceID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), sanitizeID(deviceID), SubjectOpStatus)
}

// DeviceCommand returns the subject for sending commands to a device.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.{deviceID}.command
func (b *ProxySubjectBuilder) DeviceCommand(proxyAgentID, deviceID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), sanitizeID(deviceID), SubjectOpCommand)
}

// DeviceResult returns the subject for command results from a device.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.{deviceID}.result
func (b *ProxySubjectBuilder) DeviceResult(proxyAgentID, deviceID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), sanitizeID(deviceID), SubjectOpResult)
}

// DeviceHealth returns the subject for device health updates.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.{deviceID}.health
func (b *ProxySubjectBuilder) DeviceHealth(proxyAgentID, deviceID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), sanitizeID(deviceID), SubjectOpHealth)
}

// CredentialFetch returns the subject for fetching credentials.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.credential.fetch
func (b *ProxySubjectBuilder) CredentialFetch(proxyAgentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), SubjectCategoryCredential, SubjectOpFetch)
}

// CredentialRefresh returns the subject for refreshing credentials.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.credential.refresh
func (b *ProxySubjectBuilder) CredentialRefresh(proxyAgentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID), SubjectCategoryCredential, SubjectOpRefresh)
}

// ProxyAgentWildcard returns a wildcard subject for all proxy agent operations.
// Format: kscore.{cluster}.proxy.{proxyAgentID}.>
func (b *ProxySubjectBuilder) ProxyAgentWildcard(proxyAgentID string) string {
	return fmt.Sprintf("%s.%s.%s.%s.>",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, sanitizeID(proxyAgentID))
}

// AllDevicesWildcard returns a wildcard subject for all device operations.
// Format: kscore.{cluster}.proxy.*.*.command
func (b *ProxySubjectBuilder) AllDevicesCommandWildcard() string {
	return fmt.Sprintf("%s.%s.%s.*.*.%s",
		SubjectPrefix, b.cluster, SubjectCategoryProxy, SubjectOpCommand)
}

// AllProxyAgentsWildcard returns a wildcard subject for all proxy agents.
// Format: kscore.{cluster}.proxy.>
func (b *ProxySubjectBuilder) AllProxyAgentsWildcard() string {
	return fmt.Sprintf("%s.%s.%s.>",
		SubjectPrefix, b.cluster, SubjectCategoryProxy)
}

// ParseDeviceSubject parses a device subject and extracts proxyAgentID, deviceID, and operation.
func ParseDeviceSubject(subject string) (proxyAgentID, deviceID, operation string, err error) {
	parts := strings.Split(subject, ".")
	// Expected format: kscore.{cluster}.proxy.{proxyAgentID}.{deviceID}.{operation}
	if len(parts) < 6 {
		return "", "", "", fmt.Errorf("invalid device subject: %s", subject)
	}

	if parts[0] != SubjectPrefix || parts[2] != SubjectCategoryProxy {
		return "", "", "", fmt.Errorf("invalid device subject prefix: %s", subject)
	}

	proxyAgentID = parts[3]
	deviceID = parts[4]
	operation = parts[5]

	return proxyAgentID, deviceID, operation, nil
}

// ParseProxySubject parses a proxy subject and extracts cluster, proxyAgentID, and remaining parts.
func ParseProxySubject(subject string) (cluster, proxyAgentID string, remaining []string, err error) {
	parts := strings.Split(subject, ".")
	// Expected format: kscore.{cluster}.proxy.{proxyAgentID}...
	if len(parts) < 4 {
		return "", "", nil, fmt.Errorf("invalid proxy subject: %s", subject)
	}

	if parts[0] != SubjectPrefix || parts[2] != SubjectCategoryProxy {
		return "", "", nil, fmt.Errorf("invalid proxy subject prefix: %s", subject)
	}

	cluster = parts[1]
	proxyAgentID = parts[3]
	if len(parts) > 4 {
		remaining = parts[4:]
	}

	return cluster, proxyAgentID, remaining, nil
}

// sanitizeID sanitizes an ID for use in NATS subjects.
// NATS subjects cannot contain spaces or certain special characters.
func sanitizeID(id string) string {
	// Replace problematic characters
	replacer := strings.NewReplacer(
		" ", "_",
		".", "_",
		">", "_",
		"*", "_",
		"/", "-",
	)
	return replacer.Replace(id)
}

// FullDeviceID returns the full device ID as seen by the control plane.
// Format: {proxyAgentID}/{deviceID}
func FullDeviceID(proxyAgentID, deviceID string) string {
	return fmt.Sprintf("%s/%s", proxyAgentID, deviceID)
}

// ParseFullDeviceID parses a full device ID into proxyAgentID and deviceID.
func ParseFullDeviceID(fullID string) (proxyAgentID, deviceID string, err error) {
	parts := strings.SplitN(fullID, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid full device ID: %s", fullID)
	}
	return parts[0], parts[1], nil
}
