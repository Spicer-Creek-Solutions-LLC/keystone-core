package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

// mask returns a shallow copy of sub with the cleartext secret
// replaced by "***" — every CLI text/json output passes through this
// (except `outbound create`, which echoes the cleartext once so the
// operator can record it; §4.14 contract).
func mask(sub *outbound.Subscription) *outbound.Subscription {
	if sub == nil {
		return nil
	}
	cp := *sub
	if cp.Secret != "" {
		cp.Secret = "***"
	}
	return &cp
}

func formatSubscription(w io.Writer, output string, sub *outbound.Subscription) error {
	switch strings.ToLower(output) {
	case "", "text":
		fmt.Fprintf(w, "id=%s name=%s url=%s enabled=%v\n", sub.ID, sub.Name, sub.URL, sub.Enabled)
		fmt.Fprintf(w, "  events=%v\n", sub.Events)
		fmt.Fprintf(w, "  secret=%s max_retries=%d timeout_sec=%d\n", sub.Secret, sub.MaxRetries, sub.TimeoutSec)
		if len(sub.Headers) > 0 {
			fmt.Fprintf(w, "  headers=%v\n", sub.Headers)
		}
		return nil
	case "json":
		return writeJSON(w, sub)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

func formatSubscriptionList(w io.Writer, output string, subs []*outbound.Subscription) error {
	switch strings.ToLower(output) {
	case "", "text":
		if len(subs) == 0 {
			fmt.Fprintln(w, "(no subscriptions)")
			return nil
		}
		for _, s := range subs {
			fmt.Fprintf(w, "%s\t%s\t%s\tenabled=%v\tevents=%v\n", s.ID, s.Name, s.URL, s.Enabled, s.Events)
		}
		return nil
	case "json":
		return writeJSON(w, subs)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

func formatDelivery(w io.Writer, output string, d *outbound.DeliveryRecord) error {
	switch strings.ToLower(output) {
	case "", "text":
		fmt.Fprintf(w, "id=%s sub=%s event=%s status=%s code=%d attempt=%d\n",
			d.ID, d.SubscriptionID, d.EventType, d.Status, d.StatusCode, d.Attempt)
		if d.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", d.Error)
		}
		return nil
	case "json":
		return writeJSON(w, d)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

func formatDeliveryList(w io.Writer, output string, list []*outbound.DeliveryRecord) error {
	switch strings.ToLower(output) {
	case "", "text":
		if len(list) == 0 {
			fmt.Fprintln(w, "(no deliveries)")
			return nil
		}
		for _, d := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\tcode=%d\tattempt=%d\n",
				d.ID, d.EventType, d.Status, d.StatusCode, d.Attempt)
		}
		return nil
	case "json":
		return writeJSON(w, list)
	default:
		return fmt.Errorf("unknown --output %q (want text|json)", output)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
