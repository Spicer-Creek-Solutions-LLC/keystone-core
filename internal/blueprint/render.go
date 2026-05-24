// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// RenderContext is the "." root exposed to blueprint state templates.
// Params holds the resolved (coerced + defaulted) parameter values;
// Features holds the evaluated feature flags. Both are addressable as
// `.Params.foo` / `.Features.bar` inside a template.
type RenderContext struct {
	Params   map[string]any
	Features map[string]bool
}

// NewRenderContext assembles a RenderContext from resolved parameters
// and an evaluated feature map (see ResolveParams / EvaluateFeatures).
func NewRenderContext(rp ResolvedParams, features map[string]bool) RenderContext {
	return RenderContext{Params: rp.Values, Features: features}
}

// RenderState evaluates a blueprint state-file template against ctx.
//
// It delegates to statemgmt's Renderer so blueprints and hand-written
// state files share one template dialect: the fixed §4.8 FuncMap
// (upper, lower, title, trim, join, split, default) and
// missingkey=error — a typo'd parameter fails loudly instead of
// silently rendering "<no value>" (PROJECT-DETAILS §4.17 gotcha:
// invalid input must surface clearly).
func RenderState(text string, ctx RenderContext) (string, error) {
	out, err := statemgmt.NewRenderer().RenderString(text, ctx)
	if err != nil {
		return "", fmt.Errorf("blueprint: render state: %w", err)
	}
	return out, nil
}
