// SPDX-License-Identifier: Apache-2.0

package steps

import (
	"context"
	"fmt"

	"go.keystone-core.io/keystone-core/internal/runbook"
)

// notificationStep emits a notification via the injected Notifier.
// config: channel (required), message (required), severity (default
// "info"), fields (optional map passed through).
func (d Deps) notificationStep(ctx context.Context, sc runbook.StepContext) (runbook.StepOutput, error) {
	if d.Notifier == nil {
		return runbook.StepOutput{}, fmt.Errorf("%w: notification", ErrStepNotConfigured)
	}
	channel, err := cfgString(sc.Config, "channel")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	message, err := cfgString(sc.Config, "message")
	if err != nil {
		return runbook.StepOutput{}, err
	}
	var fields map[string]any
	if v, ok := sc.Config["fields"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			return runbook.StepOutput{}, fmt.Errorf("%w: fields must be a map, got %T", ErrStepConfig, v)
		}
		fields = m
	}
	n := Notification{
		Channel:  channel,
		Message:  message,
		Severity: cfgStringOpt(sc.Config, "severity", "info"),
		Fields:   fields,
	}
	if err := d.Notifier.Notify(ctx, n); err != nil {
		return runbook.StepOutput{}, fmt.Errorf("steps: notify: %w", err)
	}
	return runbook.StepOutput{Outputs: map[string]any{"channel": channel, "delivered": true}}, nil
}
