package module

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

func signCmd() *cobra.Command {
	var keyFile, out string
	cmd := &cobra.Command{
		Use:   "sign <module.zip>",
		Short: "Produce a detached Cosign-compatible signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			zipPath := args[0]
			if keyFile == "" {
				return fmt.Errorf("module: --key is required")
			}
			zipBytes, err := os.ReadFile(zipPath) //nolint:gosec // operator-supplied artifact
			if err != nil {
				return err
			}
			keyPEM, err := os.ReadFile(keyFile) //nolint:gosec // operator-supplied key path
			if err != nil {
				return err
			}
			signer, err := parsePrivateKey(keyPEM)
			if err != nil {
				return err
			}
			sig, err := verify.Sign(zipBytes, signer)
			if err != nil {
				return err
			}
			sb, err := verify.MarshalSignature(sig)
			if err != nil {
				return err
			}
			if out == "" {
				out = zipPath + ".sig"
			}
			// out is an operator-supplied --output / <zip>.sig path
			// in an author CLI — writing where asked is the intent.
			// #nosec G304 G703 -- operator-supplied path; module-author CLI.
			if err := os.WriteFile(out, sb, 0o600); err != nil { //nolint:gosec
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "signed: %s (key %s)\n", out, sig.KeyID)
			return nil
		},
	}
	cmd.Flags().StringVar(&keyFile, "key", "", "PEM private key (PKCS8 / SEC1 / PKCS1)")
	cmd.Flags().StringVarP(&out, "output", "o", "", "signature output path (default <zip>.sig)")
	return cmd
}

func verifyCmd() *cobra.Command {
	var keyFile, sigFile string
	cmd := &cobra.Command{
		Use:   "verify <module.zip>",
		Short: "Verify a module ZIP against a trusted public key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			zipPath := args[0]
			if keyFile == "" {
				return fmt.Errorf("module: --key (trusted public key PEM) is required")
			}
			zipBytes, err := os.ReadFile(zipPath) //nolint:gosec // operator-supplied artifact
			if err != nil {
				return err
			}
			keyPEM, err := os.ReadFile(keyFile) //nolint:gosec // operator-supplied key path
			if err != nil {
				return err
			}
			tp := verify.NewTrustPolicy()
			if err := tp.AddKeyPEM(keyPEM); err != nil {
				return err
			}
			if sigFile == "" {
				sigFile = zipPath + ".sig"
			}
			sb, err := os.ReadFile(sigFile) //nolint:gosec // operator-supplied sig path
			if err != nil {
				return err
			}
			sig, err := verify.UnmarshalSignature(sb)
			if err != nil {
				return err
			}
			if err := verify.NewVerifier(tp).Verify(zipBytes, sig); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "verified: %s (key %s)\n", zipPath, sig.KeyID)
			return nil
		},
	}
	cmd.Flags().StringVar(&keyFile, "key", "", "trusted public key PEM")
	cmd.Flags().StringVar(&sigFile, "sig", "", "signature file (default <zip>.sig)")
	return cmd
}
