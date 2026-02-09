package gnmi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/protocols"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// ParseStringPath parses a slash-delimited gNMI path string into a GNMIPath.
// Supports key selectors like /interfaces/interface[name=eth0]/state/counters.
func ParseStringPath(path string) protocols.GNMIPath {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return protocols.GNMIPath{}
	}

	// Strip leading /
	path = strings.TrimPrefix(path, "/")

	// Check for origin prefix (e.g., "openconfig:/interfaces/...")
	var origin string
	if idx := strings.Index(path, ":"); idx > 0 && !strings.Contains(path[:idx], "/") && !strings.Contains(path[:idx], "[") {
		origin = path[:idx]
		path = path[idx+1:]
		path = strings.TrimPrefix(path, "/")
	}

	if path == "" {
		return protocols.GNMIPath{Origin: origin}
	}

	elements := splitPathElements(path)

	return protocols.GNMIPath{
		Elements: elements,
		Origin:   origin,
	}
}

// splitPathElements splits a path string by '/' while respecting brackets.
func splitPathElements(path string) []string {
	var elements []string
	var current strings.Builder
	depth := 0

	for _, ch := range path {
		switch ch {
		case '[':
			depth++
			current.WriteRune(ch)
		case ']':
			depth--
			current.WriteRune(ch)
		case '/':
			if depth == 0 {
				if current.Len() > 0 {
					elements = append(elements, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		elements = append(elements, current.String())
	}

	return elements
}

// PathToString converts a GNMIPath to its slash-delimited string representation.
func PathToString(path protocols.GNMIPath) string {
	if len(path.Elements) == 0 && path.Origin == "" {
		return "/"
	}

	var sb strings.Builder
	if path.Origin != "" {
		sb.WriteString(path.Origin)
		sb.WriteString(":")
	}
	sb.WriteString("/")
	sb.WriteString(strings.Join(path.Elements, "/"))

	return sb.String()
}

// ToProtoPath converts a GNMIPath to a protobuf gnmi.Path.
func ToProtoPath(path protocols.GNMIPath) *gnmipb.Path {
	protoPath := &gnmipb.Path{
		Origin: path.Origin,
		Target: path.Target,
	}

	for _, elem := range path.Elements {
		name, keys := parsePathElement(elem)
		protoPath.Elem = append(protoPath.Elem, &gnmipb.PathElem{
			Name: name,
			Key:  keys,
		})
	}

	return protoPath
}

// parsePathElement parses a path element like "interface[name=eth0][type=ethernet]"
// into a name and key map.
func parsePathElement(elem string) (string, map[string]string) {
	idx := strings.Index(elem, "[")
	if idx < 0 {
		return elem, nil
	}

	name := elem[:idx]
	rest := elem[idx:]
	keys := make(map[string]string)

	for rest != "" {
		if !strings.HasPrefix(rest, "[") {
			break
		}
		end := strings.Index(rest, "]")
		if end < 0 {
			break
		}
		kv := rest[1:end]
		eqIdx := strings.Index(kv, "=")
		if eqIdx > 0 {
			keys[kv[:eqIdx]] = kv[eqIdx+1:]
		}
		rest = rest[end+1:]
	}

	return name, keys
}

// FromProtoPath converts a protobuf gnmi.Path to a GNMIPath.
func FromProtoPath(path *gnmipb.Path) protocols.GNMIPath {
	if path == nil {
		return protocols.GNMIPath{}
	}

	var elements []string
	for _, elem := range path.GetElem() {
		var sb strings.Builder
		sb.WriteString(elem.GetName())
		for k, v := range elem.GetKey() {
			sb.WriteString(fmt.Sprintf("[%s=%s]", k, v))
		}
		elements = append(elements, sb.String())
	}

	return protocols.GNMIPath{
		Elements: elements,
		Origin:   path.GetOrigin(),
		Target:   path.GetTarget(),
	}
}

// ToProtoValue converts a JSON byte slice to a gnmi.TypedValue.
func ToProtoValue(data []byte) *gnmipb.TypedValue {
	if data == nil {
		return nil
	}
	return &gnmipb.TypedValue{
		Value: &gnmipb.TypedValue_JsonIetfVal{
			JsonIetfVal: data,
		},
	}
}

// FromProtoValue extracts JSON bytes from a gnmi.TypedValue.
func FromProtoValue(tv *gnmipb.TypedValue) ([]byte, error) {
	if tv == nil {
		return nil, nil
	}

	switch v := tv.GetValue().(type) {
	case *gnmipb.TypedValue_JsonIetfVal:
		return v.JsonIetfVal, nil
	case *gnmipb.TypedValue_JsonVal:
		return v.JsonVal, nil
	case *gnmipb.TypedValue_StringVal:
		return []byte(v.StringVal), nil
	case *gnmipb.TypedValue_IntVal:
		return json.Marshal(v.IntVal)
	case *gnmipb.TypedValue_UintVal:
		return json.Marshal(v.UintVal)
	case *gnmipb.TypedValue_BoolVal:
		return json.Marshal(v.BoolVal)
	case *gnmipb.TypedValue_FloatVal:
		return json.Marshal(v.FloatVal)
	case *gnmipb.TypedValue_BytesVal:
		return v.BytesVal, nil
	case *gnmipb.TypedValue_AsciiVal:
		return []byte(v.AsciiVal), nil
	default:
		return nil, fmt.Errorf("unsupported TypedValue type: %T", v)
	}
}

// FromProtoNotification converts a protobuf gnmi.Notification to a GNMINotification.
func FromProtoNotification(n *gnmipb.Notification) protocols.GNMINotification {
	if n == nil {
		return protocols.GNMINotification{}
	}

	notif := protocols.GNMINotification{
		Timestamp: n.GetTimestamp(),
		Prefix:    FromProtoPath(n.GetPrefix()),
	}

	for _, u := range n.GetUpdate() {
		val, _ := FromProtoValue(u.GetVal())
		notif.Updates = append(notif.Updates, protocols.GNMIUpdate{
			Path:  FromProtoPath(u.GetPath()),
			Value: val,
		})
	}

	for _, d := range n.GetDelete() {
		notif.Deletes = append(notif.Deletes, FromProtoPath(d))
	}

	return notif
}
