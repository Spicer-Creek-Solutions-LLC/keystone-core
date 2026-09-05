// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"fmt"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Marshal renders a StateFile back to YAML that Parse accepts.
//
// It exists because a state file sometimes has to travel after being
// manipulated in memory: the blueprint executor renders parameters,
// filters by feature and applies a namespace, and the result then has
// to reach an agent. The agent must receive a state *file*, not
// resolved declarations, because it is the agent that renders
// `.Facts` -- so the in-memory form has to become YAML again.
//
// What round-trips: metadata, includes, variables, and every
// declaration's module, name, state and params. What does not:
// comments, key order within a module block, and the original
// scalar formatting (a value written as `0644` comes back as the
// string it parsed to). None of those change what the file means to
// Parse, which is the contract this guarantees -- Marshal then Parse
// yields an equivalent StateFile, not a byte-identical document.
//
// Declarations are grouped back into module sections, and both
// sections and the resources within them are emitted in sorted order.
// Parse produces declarations in document order, so preserving that
// exactly would mean tracking source positions; sorting instead makes
// the output deterministic, which matters more for something that
// crosses a wire and lands in run history.
func Marshal(sf *StateFile) ([]byte, error) {
	if sf == nil {
		return nil, fmt.Errorf("statemgmt: marshal: nil state file")
	}

	root := &yaml.Node{Kind: yaml.MappingNode}

	if sf.Metadata.Name != "" || sf.Metadata.Version != "" {
		meta := &yaml.Node{Kind: yaml.MappingNode}
		if sf.Metadata.Name != "" {
			appendPair(meta, "name", scalar(sf.Metadata.Name))
		}
		if sf.Metadata.Version != "" {
			// Quoted: an unquoted "1.0" parses back as a float, and
			// Metadata.Version is a string.
			appendPair(meta, "version", quoted(sf.Metadata.Version))
		}
		appendPair(root, keyMetadata, meta)
	}

	if len(sf.Includes) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, inc := range sf.Includes {
			seq.Content = append(seq.Content, scalar(inc))
		}
		appendPair(root, keyIncludes, seq)
	}

	if len(sf.Variables) > 0 {
		vars, err := anyNode(sf.Variables)
		if err != nil {
			return nil, fmt.Errorf("statemgmt: marshal: variables: %w", err)
		}
		appendPair(root, keyVariables, vars)
	}

	// Group declarations by module, preserving each module's resources.
	byModule := map[string][]*Declaration{}
	for _, d := range sf.Declarations {
		if d == nil {
			continue
		}
		if d.Module == "" {
			return nil, fmt.Errorf("statemgmt: marshal: declaration %q has no module", d.Name)
		}
		byModule[d.Module] = append(byModule[d.Module], d)
	}
	modules := make([]string, 0, len(byModule))
	for m := range byModule {
		modules = append(modules, m)
	}
	sort.Strings(modules)

	for _, module := range modules {
		decls := byModule[module]
		sort.SliceStable(decls, func(i, j int) bool { return decls[i].Name < decls[j].Name })

		section := &yaml.Node{Kind: yaml.MappingNode}
		for _, d := range decls {
			body := &yaml.Node{Kind: yaml.MappingNode}
			if d.State != "" {
				appendPair(body, keyState, scalar(d.State))
			}
			// Params sorted for the same determinism reason as modules.
			keys := make([]string, 0, len(d.Params))
			for k := range d.Params {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v, err := anyNode(d.Params[k])
				if err != nil {
					return nil, fmt.Errorf("statemgmt: marshal: %s %q param %q: %w",
						module, d.Name, k, err)
				}
				appendPair(body, k, v)
			}
			appendPair(section, d.Name, body)
		}
		appendPair(root, module, section)
	}

	// An entirely empty state file is a valid document; emit one.
	if len(root.Content) == 0 {
		return []byte("{}\n"), nil
	}

	out, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}})
	if err != nil {
		return nil, fmt.Errorf("statemgmt: marshal: %w", err)
	}
	return out, nil
}

func appendPair(m *yaml.Node, key string, val *yaml.Node) {
	m.Content = append(m.Content, scalar(key), val)
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: s}
}

// quoted forces double quotes, for strings whose unquoted form would
// parse back as another type.
func quoted(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: s, Style: yaml.DoubleQuotedStyle, Tag: "!!str"}
}

// anyNode encodes an arbitrary decoded-YAML value back to a node.
//
// Strings get an explicit !!str tag so a value like "0644", "true" or
// "1.0" survives the round trip as a string rather than being
// reinterpreted as an int, bool or float on the way back in. Module
// params routinely hold exactly those.
func anyNode(v any) (*yaml.Node, error) {
	if s, ok := v.(string); ok {
		return quoted(s), nil
	}
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	// Encode wraps scalars in a document-ish node in some versions;
	// normalise to the value node itself.
	if n.Kind == yaml.DocumentNode && len(n.Content) == 1 {
		return n.Content[0], nil
	}
	if err := requoteStrings(&n); err != nil {
		return nil, err
	}
	return &n, nil
}

// requoteStrings walks a node tree and forces !!str scalars to be
// quoted, so nested map and sequence values round-trip as strings for
// the same reason anyNode quotes top-level ones.
func requoteStrings(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag == "!!str" {
			n.Style = yaml.DoubleQuotedStyle
		}
	case yaml.SequenceNode, yaml.MappingNode, yaml.DocumentNode:
		for _, c := range n.Content {
			if err := requoteStrings(c); err != nil {
				return err
			}
		}
	}
	return nil
}
