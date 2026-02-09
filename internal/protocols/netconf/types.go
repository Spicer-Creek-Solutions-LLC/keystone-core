// Package netconf implements a NETCONF protocol adapter (RFC 6241) for managing
// network devices over SSH subsystem transport.
package netconf

import (
	"fmt"
	"strings"
)

// Datastore identifies a NETCONF configuration datastore.
type Datastore string

const (
	// Running is the active configuration datastore.
	Running Datastore = "running"
	// Candidate is the uncommitted configuration datastore.
	Candidate Datastore = "candidate"
	// Startup is the boot configuration datastore.
	Startup Datastore = "startup"
)

// Valid returns true if the datastore name is recognized.
func (d Datastore) Valid() bool {
	switch d {
	case Running, Candidate, Startup:
		return true
	default:
		return false
	}
}

// Capability represents a NETCONF capability URI.
type Capability string

const (
	BaseCapability10      Capability = "urn:ietf:params:netconf:base:1.0"
	BaseCapability11      Capability = "urn:ietf:params:netconf:base:1.1"
	WritableRunning       Capability = "urn:ietf:params:netconf:capability:writable-running:1.0"
	CandidateCapability   Capability = "urn:ietf:params:netconf:capability:candidate:1.0"
	ConfirmedCommit10     Capability = "urn:ietf:params:netconf:capability:confirmed-commit:1.0"
	ConfirmedCommit11     Capability = "urn:ietf:params:netconf:capability:confirmed-commit:1.1"
	RollbackOnError       Capability = "urn:ietf:params:netconf:capability:rollback-on-error:1.0"
	Validate10            Capability = "urn:ietf:params:netconf:capability:validate:1.0"
	Validate11            Capability = "urn:ietf:params:netconf:capability:validate:1.1"
	StartupCapability     Capability = "urn:ietf:params:netconf:capability:startup:1.0"
	URLCapability         Capability = "urn:ietf:params:netconf:capability:url:1.0"
	XPathCapability       Capability = "urn:ietf:params:netconf:capability:xpath:1.0"
	NotificationCapbility Capability = "urn:ietf:params:netconf:notification:1.0"
)

// EditOperation specifies the default operation for edit-config.
type EditOperation string

const (
	EditMerge   EditOperation = "merge"
	EditReplace EditOperation = "replace"
	EditCreate  EditOperation = "create"
	EditDelete  EditOperation = "delete"
	EditRemove  EditOperation = "remove"
	EditNone    EditOperation = "none"
)

// ErrorType classifies the layer that generated a NETCONF error.
type ErrorType string

const (
	ErrorTypeTransport   ErrorType = "transport"
	ErrorTypeRPC         ErrorType = "rpc"
	ErrorTypeProtocol    ErrorType = "protocol"
	ErrorTypeApplication ErrorType = "application"
)

// ErrorSeverity indicates the severity of a NETCONF error.
type ErrorSeverity string

const (
	SeverityError   ErrorSeverity = "error"
	SeverityWarning ErrorSeverity = "warning"
)

// RPCError represents a NETCONF RPC error element (RFC 6241 Section 4.3).
type RPCError struct {
	Type     ErrorType     `xml:"error-type"`
	Tag      string        `xml:"error-tag"`
	Severity ErrorSeverity `xml:"error-severity"`
	AppTag   string        `xml:"error-app-tag,omitempty"`
	Path     string        `xml:"error-path,omitempty"`
	Message  string        `xml:"error-message,omitempty"`
	Info     string        `xml:"error-info,omitempty"`
}

// Error implements the error interface for RPCError.
func (e *RPCError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "netconf rpc-error: [%s] %s", e.Severity, e.Tag)
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	if e.Path != "" {
		fmt.Fprintf(&b, " (path: %s)", e.Path)
	}
	return b.String()
}

// RPCErrors is a collection of NETCONF RPC errors.
type RPCErrors []RPCError

// Error implements the error interface.
func (errs RPCErrors) Error() string {
	if len(errs) == 1 {
		return errs[0].Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d netconf rpc-errors:", len(errs))
	for i := range errs {
		fmt.Fprintf(&b, "\n  %d: %s", i+1, errs[i].Error())
	}
	return b.String()
}

// HasError returns true if any error has severity "error" (not just "warning").
func (errs RPCErrors) HasError() bool {
	for i := range errs {
		if errs[i].Severity == SeverityError {
			return true
		}
	}
	return false
}

// EditOptions configures edit-config operation behavior.
type EditOptions struct {
	DefaultOperation EditOperation
	TestOption       string // "test-then-set", "set", "test-only"
	ErrorOption      string // "stop-on-error", "continue-on-error", "rollback-on-error"
}

// Filter specifies a filter for get and get-config operations.
type Filter struct {
	Type    string // "subtree" or "xpath"
	Content string // XML subtree or XPath expression
}

// FramingMode represents the NETCONF message framing mode.
type FramingMode int

const (
	// FramingEOM uses the end-of-message delimiter ]]>]]> (NETCONF 1.0).
	FramingEOM FramingMode = iota
	// FramingChunked uses RFC 6242 chunked framing (NETCONF 1.1).
	FramingChunked
)

// YANGModel holds basic metadata about a YANG module extracted from capabilities.
type YANGModel struct {
	Module     string
	Revision   string
	Namespace  string
	Features   []string
	Deviations []string
}

// DefaultPort is the standard NETCONF over SSH port.
const DefaultPort = 830

// ClientCapabilities returns the default client capability set.
func ClientCapabilities() []Capability {
	return []Capability{
		BaseCapability10,
		BaseCapability11,
	}
}
