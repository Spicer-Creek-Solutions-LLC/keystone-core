// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// reserved top-level YAML keys. Every other top-level key is treated
// as a module name.
const (
	keyMetadata  = "metadata"
	keyIncludes  = "includes"
	keyVariables = "variables"
	keyState     = "state"
)

// Parse decodes a state file from YAML. Declarations preserve source
// order so error messages and the resolver's stable tie-breaker (Task
// 5) stay deterministic. The parser performs no semantic validation
// beyond shape — module existence, parameter validity, and requisite
// reference resolution are the validator's job (Task 4) and the
// resolver's job (Task 5). Include expansion + variable merge are
// deferred (see doc.go).
func Parse(data []byte) (*StateFile, error) {
	sf := &StateFile{}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("statemgmt: parse: %w", err)
	}

	// Empty document → zero StateFile.
	if root.Kind == 0 || len(root.Content) == 0 {
		return sf, nil
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, fmt.Errorf("statemgmt: parse: expected a single YAML document")
	}
	top := root.Content[0]
	// An explicit "---" (or any document whose root is a null scalar)
	// is a valid empty document; treat it the same as zero bytes.
	if top.Kind == yaml.ScalarNode && (top.Tag == "!!null" || top.Value == "") {
		return sf, nil
	}
	if top.Kind != yaml.MappingNode {
		return nil, parseErr(top, "expected a mapping at the top level, got %s", kindName(top.Kind))
	}

	for i := 0; i < len(top.Content); i += 2 {
		keyNode := top.Content[i]
		valNode := top.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, parseErr(keyNode, "top-level key must be a scalar")
		}
		key := keyNode.Value

		switch key {
		case keyMetadata:
			if err := decodeMetadata(valNode, &sf.Metadata); err != nil {
				return nil, err
			}
		case keyIncludes:
			incs, err := decodeIncludes(valNode)
			if err != nil {
				return nil, err
			}
			sf.Includes = incs
		case keyVariables:
			vars, err := decodeMapping(valNode, "variables")
			if err != nil {
				return nil, err
			}
			sf.Variables = vars
		default:
			decls, err := decodeModuleSection(key, valNode)
			if err != nil {
				return nil, err
			}
			sf.Declarations = append(sf.Declarations, decls...)
		}
	}

	return sf, nil
}

func decodeMetadata(n *yaml.Node, out *Metadata) error {
	if n.Kind != yaml.MappingNode {
		return parseErr(n, "metadata must be a mapping")
	}
	type metaYAML struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	}
	var m metaYAML
	if err := n.Decode(&m); err != nil {
		return parseErr(n, "metadata: %v", err)
	}
	out.Name = m.Name
	out.Version = m.Version
	return nil
}

func decodeIncludes(n *yaml.Node) ([]string, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, parseErr(n, "includes must be a sequence of strings")
	}
	out := make([]string, 0, len(n.Content))
	for _, child := range n.Content {
		if child.Kind != yaml.ScalarNode {
			return nil, parseErr(child, "includes entries must be strings")
		}
		out = append(out, child.Value)
	}
	return out, nil
}

func decodeMapping(n *yaml.Node, label string) (map[string]any, error) {
	if n.Kind != yaml.MappingNode {
		return nil, parseErr(n, "%s must be a mapping", label)
	}
	var out map[string]any
	if err := n.Decode(&out); err != nil {
		return nil, parseErr(n, "%s: %v", label, err)
	}
	return out, nil
}

// decodeModuleSection turns one top-level non-reserved entry into a
// slice of Declarations preserving source order:
//
//	<module>:
//	  <resource-name-1>:
//	    state: <state>
//	    <other params>
//	  <resource-name-2>:
//	    ...
func decodeModuleSection(module string, n *yaml.Node) ([]*Declaration, error) {
	if n.Kind != yaml.MappingNode {
		return nil, parseErr(n, "module section %q must be a mapping of resource declarations", module)
	}
	decls := make([]*Declaration, 0, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, parseErr(keyNode, "module %q: resource name must be a scalar", module)
		}
		name := keyNode.Value

		params, err := decodeMapping(valNode, fmt.Sprintf("module %q resource %q", module, name))
		if err != nil {
			return nil, err
		}

		// Promote `state` to a typed field; everything else stays in Params.
		state := ""
		if raw, ok := params[keyState]; ok {
			s, ok := raw.(string)
			if !ok {
				return nil, parseErr(valNode, "module %q resource %q: state must be a string, got %T", module, name, raw)
			}
			state = s
			delete(params, keyState)
		}

		decls = append(decls, &Declaration{
			ID:     module + ":" + name,
			Module: module,
			State:  state,
			Name:   name,
			Params: params,
		})
	}
	return decls, nil
}

// parseErr wraps a formatted message with the offending node's line
// number when one is available. yaml.v3 populates Line/Column on every
// decoded node so error messages can point at the source.
func parseErr(n *yaml.Node, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if n != nil && n.Line > 0 {
		return fmt.Errorf("statemgmt: parse: line %d: %s", n.Line, msg)
	}
	return fmt.Errorf("statemgmt: parse: %s", msg)
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("yaml.Kind(%d)", k)
	}
}
