// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func caInfoCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show root + signing CA details + the active JWT kid",
		Long: "Reads the provider's GetCAInfo + ExportCA SIGNING. The signing " +
			"cert details (subject, expiry, key type) are derived client-side by " +
			"parsing the PEM that ExportCA returns — the gRPC GetCAInfo response " +
			"itself ships only the root + kid in v0.1.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCAInfo(cmd.Context(), cmd.OutOrStdout(), g)
		},
	}
	return cmd
}

func runCAInfo(ctx context.Context, out io.Writer, g *globals) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	authCtx := authContext(ctx, g.APIKey)
	info, err := client.GetCAInfo(authCtx, &v1.GetCAInfoRequest{})
	if err != nil {
		return fmt.Errorf("GetCAInfo: %w", err)
	}
	signingPEM, err := client.ExportCA(authCtx, &v1.ExportCARequest{
		What: v1.ExportCARequest_WHAT_SIGNING,
	})
	if err != nil {
		return fmt.Errorf("ExportCA SIGNING: %w", err)
	}
	signingInfo, err := parseSigningCert(signingPEM.GetPem())
	if err != nil {
		return fmt.Errorf("parse signing cert: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		// Merge the signing-side info into a single payload so
		// `--output json` consumers don't need two calls.
		return writeJSONAny(out, struct {
			TrustDomain string         `json:"trust_domain"`
			Root        *v1.CACertInfo `json:"root"`
			Signing     *v1.CACertInfo `json:"signing"`
			JWTKid      string         `json:"jwt_kid"`
		}{
			TrustDomain: info.GetTrustDomain(),
			Root:        info.GetRoot(),
			Signing:     signingInfo,
			JWTKid:      info.GetJwtKid(),
		})
	default:
		return printCAInfo(out, info, signingInfo)
	}
}

func printCAInfo(out io.Writer, info *v1.GetCAInfoResponse, signing *v1.CACertInfo) error {
	t := newTable(out)
	t.header("FIELD", "VALUE")
	t.row("TrustDomain", info.GetTrustDomain())
	t.row("JWT kid", info.GetJwtKid())
	t.row("", "")
	t.row("Root subject", info.GetRoot().GetSubject())
	t.row("Root serial", info.GetRoot().GetSerial())
	t.row("Root key type", info.GetRoot().GetKeyType())
	if nb := info.GetRoot().GetNotBefore(); nb != nil {
		ts := nb.AsTime()
		t.row("Root not-before", formatTimestamp(&ts))
	}
	if na := info.GetRoot().GetNotAfter(); na != nil {
		ts := na.AsTime()
		t.row("Root not-after", formatTimestamp(&ts))
	}
	t.row("", "")
	t.row("Signing subject", signing.GetSubject())
	t.row("Signing serial", signing.GetSerial())
	t.row("Signing key type", signing.GetKeyType())
	if nb := signing.GetNotBefore(); nb != nil {
		ts := nb.AsTime()
		t.row("Signing not-before", formatTimestamp(&ts))
	}
	if na := signing.GetNotAfter(); na != nil {
		ts := na.AsTime()
		t.row("Signing not-after", formatTimestamp(&ts))
	}
	return t.flush()
}

// parseSigningCert decodes a single-block PEM cert and projects
// its CACertInfo. Used by both `ca info` and `status`.
func parseSigningCert(pemBytes []byte) (*v1.CACertInfo, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("expected CERTIFICATE PEM block, got %v", block)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return certInfoForCLI(cert), nil
}
