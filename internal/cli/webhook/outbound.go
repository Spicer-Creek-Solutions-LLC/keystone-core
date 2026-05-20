package webhook

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

// newOutboundCmd is the `kscore-webhook outbound` parent. Each
// subcommand opens a [outbound.SQLiteStore] at --store; the `test`
// subcommand additionally constructs an [outbound.HTTPDispatcher] to
// post a synthetic payload directly (no Manager / retry / circuit
// breaker — operator wants a "did my URL respond" verdict).
func newOutboundCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outbound",
		Short: "Manage outbound webhook subscriptions",
	}
	cmd.AddCommand(
		newListCmd(d),
		newCreateCmd(d),
		newShowCmd(d),
		newDeleteCmd(d),
		newHistoryCmd(d),
		newTestCmd(d),
	)
	return cmd
}

func openStore(path string) (*outbound.SQLiteStore, func(), error) {
	s, err := outbound.NewSQLiteStore(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open store at %q: %w", path, err)
	}
	return s, func() { _ = s.Close() }, nil
}

// --- shared flag --------------------------------------------------------------

type storeFlag struct {
	store  string
	output string
}

func (f *storeFlag) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.store, "store", "./.kscore-webhook.db",
		"path to the outbound subscriptions SQLite store (use ':memory:' for ephemeral)")
	cmd.Flags().StringVar(&f.output, "output", "text", "output format: text|json")
}

// --- list --------------------------------------------------------------------

func newListCmd(d Deps) *cobra.Command {
	var f storeFlag
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List outbound subscriptions",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, closeFn, err := openStore(f.store)
			if err != nil {
				return err
			}
			defer closeFn()
			subs, err := s.ListSubscriptions(context.Background())
			if err != nil {
				return err
			}
			out := make([]*outbound.Subscription, 0, len(subs))
			for _, sub := range subs {
				out = append(out, mask(sub))
			}
			return formatSubscriptionList(cmd.OutOrStdout(), f.output, out)
		},
	}
	f.bind(cmd)
	return cmd
}

// --- create -----------------------------------------------------------------

func newCreateCmd(d Deps) *cobra.Command {
	var f storeFlag
	var (
		name, url, secret, eventsCSV, headersCSV string
		maxRetries, timeoutSec                   int
		enabled                                  bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new outbound subscription",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || url == "" {
				return fmt.Errorf("--name and --url are required")
			}
			s, closeFn, err := openStore(f.store)
			if err != nil {
				return err
			}
			defer closeFn()
			headers, err := parseHeaderCSV(headersCSV)
			if err != nil {
				return err
			}
			now := d.Now().UTC()
			sub := &outbound.Subscription{
				ID:         d.IDGen(),
				Name:       name,
				URL:        url,
				Secret:     secret,
				Events:     splitCSV(eventsCSV),
				Enabled:    enabled,
				Headers:    headers,
				MaxRetries: maxRetries,
				TimeoutSec: timeoutSec,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := s.CreateSubscription(context.Background(), sub); err != nil {
				return err
			}
			// Creation echoes the cleartext secret once (§4.14 contract).
			return formatSubscription(cmd.OutOrStdout(), f.output, sub)
		},
	}
	f.bind(cmd)
	cmd.Flags().StringVar(&name, "name", "", "subscription name")
	cmd.Flags().StringVar(&url, "url", "", "receiver URL")
	cmd.Flags().StringVar(&secret, "secret", "", "HMAC signing secret (echoed cleartext in this response only)")
	cmd.Flags().StringVar(&eventsCSV, "events", "", "comma-separated event-type glob list")
	cmd.Flags().StringVar(&headersCSV, "headers", "", "comma-separated k=v custom headers")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "max retries per delivery")
	cmd.Flags().IntVar(&timeoutSec, "timeout-sec", 10, "per-delivery timeout in seconds")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "subscription is enabled")
	return cmd
}

// --- show -------------------------------------------------------------------

func newShowCmd(d Deps) *cobra.Command {
	var f storeFlag
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one subscription (secret masked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, closeFn, err := openStore(f.store)
			if err != nil {
				return err
			}
			defer closeFn()
			sub, ok, err := s.GetSubscription(context.Background(), args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("subscription %q not found", args[0])
			}
			return formatSubscription(cmd.OutOrStdout(), f.output, mask(sub))
		},
	}
	f.bind(cmd)
	return cmd
}

// --- delete -----------------------------------------------------------------

func newDeleteCmd(d Deps) *cobra.Command {
	var f storeFlag
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, closeFn, err := openStore(f.store)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := s.DeleteSubscription(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
	f.bind(cmd)
	return cmd
}

// --- history ----------------------------------------------------------------

func newHistoryCmd(d Deps) *cobra.Command {
	var f storeFlag
	var limit int
	cmd := &cobra.Command{
		Use:   "history <id>",
		Short: "List recent delivery records for a subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, closeFn, err := openStore(f.store)
			if err != nil {
				return err
			}
			defer closeFn()
			list, err := s.ListDeliveries(context.Background(), args[0], limit)
			if err != nil {
				return err
			}
			return formatDeliveryList(cmd.OutOrStdout(), f.output, list)
		},
	}
	f.bind(cmd)
	cmd.Flags().IntVar(&limit, "limit", 0, "cap result count (0 = unlimited)")
	return cmd
}

// --- test -------------------------------------------------------------------

func newTestCmd(d Deps) *cobra.Command {
	var f storeFlag
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "POST a synthetic webhook.test payload to the subscription URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, closeFn, err := openStore(f.store)
			if err != nil {
				return err
			}
			defer closeFn()
			sub, ok, err := s.GetSubscription(context.Background(), args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("subscription %q not found", args[0])
			}
			disp := d.Dispatcher
			if disp == nil {
				disp = &outbound.HTTPDispatcher{HTTPClient: &http.Client{Timeout: 30 * time.Second}}
			}
			rec := &outbound.DeliveryRecord{
				ID:             d.IDGen(),
				SubscriptionID: sub.ID,
				EventType:      "webhook.test",
				EventID:        "test-" + sub.ID,
				Status:         outbound.DeliveryPending,
				Attempt:        1,
				DeliveredAt:    d.Now().UTC(),
			}
			payload := fmt.Sprintf(`{"event":"webhook.test","subscription":%q,"emitted_at":%q}`,
				sub.ID, rec.DeliveredAt.Format(time.RFC3339Nano))
			if err := s.SaveDelivery(context.Background(), rec); err != nil {
				return err
			}
			code, derr := disp.Deliver(context.Background(), sub, []byte(payload), rec.ID)
			rec.StatusCode = code
			rec.DeliveredAt = d.Now().UTC()
			if derr != nil {
				rec.Status = outbound.DeliveryFailed
				rec.Error = derr.Error()
			} else {
				rec.Status = outbound.DeliverySuccess
			}
			_ = s.SaveDelivery(context.Background(), rec)
			return formatDelivery(cmd.OutOrStdout(), f.output, rec)
		},
	}
	f.bind(cmd)
	return cmd
}

// --- helpers ----------------------------------------------------------------

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseHeaderCSV(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		if eq <= 0 || eq == len(pair)-1 {
			return nil, fmt.Errorf("invalid --headers entry %q (want k=v)", pair)
		}
		out[strings.TrimSpace(pair[:eq])] = strings.TrimSpace(pair[eq+1:])
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// uuid import kept for cli main wiring (production IDGen).
var _ = uuid.NewString
