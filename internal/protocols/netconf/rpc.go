package netconf

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Hello is the NETCONF hello message exchanged during session setup.
type Hello struct {
	XMLName      xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 hello"`
	Capabilities []string `xml:"capabilities>capability"`
	SessionID    uint32   `xml:"session-id,omitempty"`
}

// rpcEnvelope is the outer <rpc> element.
type rpcEnvelope struct {
	XMLName   xml.Name    `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 rpc"`
	MessageID string      `xml:"message-id,attr"`
	Body      interface{} // inner operation element
}

// rpcReplyRaw is used for initial parsing of <rpc-reply>.
type rpcReplyRaw struct {
	XMLName   xml.Name   `xml:"rpc-reply"`
	MessageID string     `xml:"message-id,attr"`
	OK        *struct{}  `xml:"ok"`
	Errors    []RPCError `xml:"rpc-error"`
	Data      innerXML   `xml:"data"`
}

// innerXML captures raw inner XML without further parsing.
type innerXML struct {
	Content []byte `xml:",innerxml"`
}

// RPCReply is the parsed NETCONF rpc-reply.
type RPCReply struct {
	MessageID string
	OK        bool
	Data      []byte
	Errors    RPCErrors
}

// rpc operation structs

type rpcGetConfig struct {
	XMLName xml.Name       `xml:"get-config"`
	Source  rpcDatastore   `xml:"source"`
	Filter  *rpcFilter     `xml:"filter,omitempty"`
}

type rpcGet struct {
	XMLName xml.Name   `xml:"get"`
	Filter  *rpcFilter `xml:"filter,omitempty"`
}

type rpcEditConfig struct {
	XMLName          xml.Name     `xml:"edit-config"`
	Target           rpcDatastore `xml:"target"`
	DefaultOperation string       `xml:"default-operation,omitempty"`
	TestOption       string       `xml:"test-option,omitempty"`
	ErrorOption      string       `xml:"error-option,omitempty"`
	Config           rawXML       `xml:"config"`
}

type rpcCopyConfig struct {
	XMLName xml.Name     `xml:"copy-config"`
	Target  rpcDatastore `xml:"target"`
	Source  rpcDatastore `xml:"source"`
}

type rpcDeleteConfig struct {
	XMLName xml.Name     `xml:"delete-config"`
	Target  rpcDatastore `xml:"target"`
}

type rpcLock struct {
	XMLName xml.Name     `xml:"lock"`
	Target  rpcDatastore `xml:"target"`
}

type rpcUnlock struct {
	XMLName xml.Name     `xml:"unlock"`
	Target  rpcDatastore `xml:"target"`
}

type rpcCommit struct {
	XMLName xml.Name `xml:"commit"`
}

type rpcDiscardChanges struct {
	XMLName xml.Name `xml:"discard-changes"`
}

type rpcValidate struct {
	XMLName xml.Name     `xml:"validate"`
	Source  rpcDatastore `xml:"source"`
}

type rpcCloseSession struct {
	XMLName xml.Name `xml:"close-session"`
}

type rpcKillSession struct {
	XMLName   xml.Name `xml:"kill-session"`
	SessionID uint32   `xml:"session-id"`
}

// helper types

type rpcDatastore struct {
	Running   *struct{} `xml:"running,omitempty"`
	Candidate *struct{} `xml:"candidate,omitempty"`
	Startup   *struct{} `xml:"startup,omitempty"`
}

func datastoreElement(ds Datastore) rpcDatastore {
	switch ds {
	case Candidate:
		return rpcDatastore{Candidate: &struct{}{}}
	case Startup:
		return rpcDatastore{Startup: &struct{}{}}
	default:
		return rpcDatastore{Running: &struct{}{}}
	}
}

type rpcFilter struct {
	Type    string `xml:"type,attr"`
	Content string `xml:",innerxml"`
}

func filterElement(f *Filter) *rpcFilter {
	if f == nil {
		return nil
	}
	return &rpcFilter{
		Type:    f.Type,
		Content: f.Content,
	}
}

// rawXML embeds pre-formed XML without re-encoding.
type rawXML struct {
	Content string `xml:",innerxml"`
}

// encodeRPC marshals an RPC message with the given message ID and operation body.
func encodeRPC(messageID string, body interface{}) ([]byte, error) {
	env := rpcEnvelope{
		MessageID: messageID,
		Body:      body,
	}
	data, err := xml.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc: %w", err)
	}
	return data, nil
}

// decodeReply parses an rpc-reply XML message.
func decodeReply(data []byte) (*RPCReply, error) {
	var raw rpcReplyRaw
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal rpc-reply: %w", err)
	}

	reply := &RPCReply{
		MessageID: raw.MessageID,
		OK:        raw.OK != nil,
		Errors:    RPCErrors(raw.Errors),
	}

	content := strings.TrimSpace(string(raw.Data.Content))
	if content != "" {
		reply.Data = []byte(content)
	}

	return reply, nil
}

// newHello creates a client hello message with the given capabilities.
func newHello(caps []Capability) *Hello {
	strs := make([]string, len(caps))
	for i, c := range caps {
		strs[i] = string(c)
	}
	return &Hello{Capabilities: strs}
}

// encodeHello marshals a hello message with the XML declaration.
func encodeHello(h *Hello) ([]byte, error) {
	data, err := xml.Marshal(h)
	if err != nil {
		return nil, fmt.Errorf("marshal hello: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

// parseHello parses a hello message from raw XML bytes.
func parseHello(data []byte) (*Hello, error) {
	var h Hello
	if err := xml.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("unmarshal hello: %w", err)
	}
	if len(h.Capabilities) == 0 {
		return nil, fmt.Errorf("hello contains no capabilities")
	}
	return &h, nil
}
