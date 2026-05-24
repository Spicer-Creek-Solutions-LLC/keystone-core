// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// renderRoot is the "." root for runbook templates. Steps maps a
// completed step name to {"outputs": map, "status": string}; using
// plain maps (not a struct) keeps the lowercase spec syntax
// `{{ .steps.<name>.outputs.<field> }}` working.
type renderRoot struct {
	inputs map[string]any
	steps  map[string]any
}

func (rr renderRoot) data() map[string]any {
	return map[string]any{"inputs": rr.inputs, "steps": rr.steps}
}

// renderString evaluates one template string against the run context.
// It delegates to statemgmt's renderer (one product-wide dialect;
// missingkey=error makes a typo'd `{{ .steps.x.outputs.y }}` fail
// loudly — §4.17: silent variables must not cross steps).
func renderString(text string, rr renderRoot) (string, error) {
	out, err := statemgmt.NewRenderer().RenderString(text, rr.data())
	if err != nil {
		return "", fmt.Errorf("runbook: render: %w", err)
	}
	return out, nil
}

// renderConfig deep-copies cfg with every string leaf rendered.
// Non-string leaves pass through. Maps and slices are recursed.
func renderConfig(cfg map[string]any, rr renderRoot) (map[string]any, error) {
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		rv, err := renderValue(v, rr)
		if err != nil {
			return nil, fmt.Errorf("config.%s: %w", k, err)
		}
		out[k] = rv
	}
	return out, nil
}

func renderValue(v any, rr renderRoot) (any, error) {
	switch t := v.(type) {
	case string:
		return renderString(t, rr)
	case map[string]any:
		return renderConfig(t, rr)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			rv, err := renderValue(e, rr)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = rv
		}
		return out, nil
	default:
		return v, nil
	}
}

// truthy interprets a rendered Condition. Empty and the canonical
// false-ish tokens are false; anything else is true.
func truthy(rendered string) bool {
	switch strings.ToLower(strings.TrimSpace(rendered)) {
	case "", "false", "0", "no", "off":
		return false
	default:
		return true
	}
}
