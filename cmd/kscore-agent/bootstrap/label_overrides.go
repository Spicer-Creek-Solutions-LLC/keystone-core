package bootstrap

import (
	"fmt"
	"sort"
	"strings"
)

func applyNodeLabelOverrides(opts *Options) error {
	if len(opts.NodeLabelArgs) == 0 {
		return nil
	}
	if opts.NodeLabels == nil {
		opts.NodeLabels = make(map[string]string)
	}
	for _, spec := range opts.NodeLabelArgs {
		key, value, err := splitKeyValue(spec)
		if err != nil {
			return fmt.Errorf("invalid node label %q: %w", spec, err)
		}
		if value == "" {
			return fmt.Errorf("invalid node label %q: empty value", spec)
		}
		opts.NodeLabels[key] = value
	}
	return nil
}

func formatNodeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, labels[key]))
	}
	return strings.Join(parts, ",")
}
