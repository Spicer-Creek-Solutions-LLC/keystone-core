package secrets

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func transitCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transit",
		Short: "Encryption-as-a-service (Vault transit engine)",
		Long: "Subcommands for the TransitBackend surface — encrypt / " +
			"decrypt / sign / verify. v1.0 wire format is singleton-only; " +
			"batch transit + HMAC + Rewrap + GenerateDataKey are tracked " +
			"under the v1.x ROADMAP.",
	}
	cmd.AddCommand(transitEncryptCmd(g))
	cmd.AddCommand(transitDecryptCmd(g))
	cmd.AddCommand(transitSignCmd(g))
	cmd.AddCommand(transitVerifyCmd(g))
	return cmd
}

// ---- transit encrypt ---------------------------------------------

type transitEncryptOpts struct {
	plaintext     string // inline string (UTF-8)
	plaintextHex  string // hex bytes
	plaintextFile string // path to file containing raw bytes
	context       string
}

func transitEncryptCmd(g *globals) *cobra.Command {
	opts := &transitEncryptOpts{}
	cmd := &cobra.Command{
		Use:   "encrypt <key>",
		Short: "Encrypt a plaintext via the transit key",
		Long: "Encrypts the supplied plaintext. Exactly one of " +
			"--plaintext / --plaintext-hex / --plaintext-file is required.\n" +
			"The returned ciphertext is in Vault wire format " +
			"`vault:vN:base64…` — pass it back to `decrypt` unchanged.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransitEncrypt(cmd.Context(), cmd.OutOrStdout(), g, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.plaintext, "plaintext", "",
		"plaintext as a UTF-8 string")
	cmd.Flags().StringVar(&opts.plaintextHex, "plaintext-hex", "",
		"plaintext as a hex string")
	cmd.Flags().StringVar(&opts.plaintextFile, "plaintext-file", "",
		"path to a file containing the plaintext bytes")
	cmd.Flags().StringVar(&opts.context, "context", "",
		"derivation context (UTF-8) for convergent / derived keys")
	return cmd
}

func runTransitEncrypt(ctx context.Context, out io.Writer, g *globals, key string, opts *transitEncryptOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	plain, err := loadPlaintext(opts.plaintext, opts.plaintextHex, opts.plaintextFile)
	if err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	req := &v1.EncryptRequest{KeyName: key, Plaintext: plain}
	if opts.context != "" {
		req.Context = []byte(opts.context)
	}
	resp, err := client.Encrypt(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("transit encrypt: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		// Ciphertext is text wire format (vault:vN:base64…) but the
		// proto carries it as bytes — print as a string.
		_, _ = fmt.Fprintln(out, string(resp.GetCiphertext()))
		return nil
	}
}

// ---- transit decrypt ---------------------------------------------

type transitDecryptOpts struct {
	ciphertext string
	context    string
	asString   bool
}

func transitDecryptCmd(g *globals) *cobra.Command {
	opts := &transitDecryptOpts{}
	cmd := &cobra.Command{
		Use:   "decrypt <key>",
		Short: "Decrypt a transit ciphertext",
		Long: "Decrypts the supplied ciphertext (in Vault wire form " +
			"`vault:vN:base64…`). By default the plaintext is rendered " +
			"as hex; pass --as-string to print it as UTF-8.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransitDecrypt(cmd.Context(), cmd.OutOrStdout(), g, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.ciphertext, "ciphertext", "",
		"ciphertext to decrypt (`vault:vN:base64…`)")
	cmd.Flags().StringVar(&opts.context, "context", "",
		"derivation context matching the encrypt-time value")
	cmd.Flags().BoolVar(&opts.asString, "as-string", false,
		"render plaintext as UTF-8 (default: hex)")
	_ = cmd.MarkFlagRequired("ciphertext")
	return cmd
}

func runTransitDecrypt(ctx context.Context, out io.Writer, g *globals, key string, opts *transitDecryptOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	req := &v1.DecryptRequest{KeyName: key, Ciphertext: []byte(opts.ciphertext)}
	if opts.context != "" {
		req.Context = []byte(opts.context)
	}
	resp, err := client.Decrypt(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("transit decrypt: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		plain := resp.GetPlaintext()
		if opts.asString {
			_, _ = fmt.Fprintln(out, string(plain))
		} else {
			_, _ = fmt.Fprintln(out, base64.StdEncoding.EncodeToString(plain))
		}
		return nil
	}
}

// ---- transit sign ------------------------------------------------

type transitSignOpts struct {
	message     string
	messageFile string
	algorithm   string
}

func transitSignCmd(g *globals) *cobra.Command {
	opts := &transitSignOpts{}
	cmd := &cobra.Command{
		Use:   "sign <key>",
		Short: "Sign a message via the transit key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransitSign(cmd.Context(), cmd.OutOrStdout(), g, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.message, "message", "",
		"message bytes as a UTF-8 string")
	cmd.Flags().StringVar(&opts.messageFile, "message-file", "",
		"path to file containing the message bytes")
	cmd.Flags().StringVar(&opts.algorithm, "algorithm", "",
		"signature algorithm (e.g. rsa-pss-sha256, ed25519); empty = Vault default")
	return cmd
}

func runTransitSign(ctx context.Context, out io.Writer, g *globals, key string, opts *transitSignOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	msg, err := loadMessage(opts.message, opts.messageFile)
	if err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.Sign(authContext(ctx, g.APIKey), &v1.SignRequest{
		KeyName:   key,
		Message:   msg,
		Algorithm: opts.algorithm,
	})
	if err != nil {
		return fmt.Errorf("transit sign: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintln(out, string(resp.GetSignature()))
		return nil
	}
}

// ---- transit verify ----------------------------------------------

type transitVerifyOpts struct {
	message     string
	messageFile string
	signature   string
	algorithm   string
}

func transitVerifyCmd(g *globals) *cobra.Command {
	opts := &transitVerifyOpts{}
	cmd := &cobra.Command{
		Use:   "verify <key>",
		Short: "Verify a signature against a message",
		Long: "Verifies the supplied signature against the message. A " +
			"mismatched signature is reported as `valid: false` — NOT " +
			"a non-zero exit code (the verify operation itself succeeded; " +
			"the signature just didn't match).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransitVerify(cmd.Context(), cmd.OutOrStdout(), g, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.message, "message", "",
		"message bytes as a UTF-8 string")
	cmd.Flags().StringVar(&opts.messageFile, "message-file", "",
		"path to file containing the message bytes")
	cmd.Flags().StringVar(&opts.signature, "signature", "",
		"signature to verify (`vault:vN:base64…`)")
	cmd.Flags().StringVar(&opts.algorithm, "algorithm", "",
		"signature algorithm matching the sign-time value")
	_ = cmd.MarkFlagRequired("signature")
	return cmd
}

func runTransitVerify(ctx context.Context, out io.Writer, g *globals, key string, opts *transitVerifyOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	msg, err := loadMessage(opts.message, opts.messageFile)
	if err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.Verify(authContext(ctx, g.APIKey), &v1.VerifyRequest{
		KeyName:   key,
		Message:   msg,
		Signature: []byte(opts.signature),
		Algorithm: opts.algorithm,
	})
	if err != nil {
		return fmt.Errorf("transit verify: %w", err)
	}

	switch g.Output {
	case FormatJSON:
		return writeJSON(out, resp)
	default:
		_, _ = fmt.Fprintf(out, "valid: %v\n", resp.GetValid())
		return nil
	}
}

// ---- helpers -----------------------------------------------------

// loadPlaintext returns the raw plaintext bytes from one of the three
// flag sources. Exactly one must be set.
func loadPlaintext(inline, hex, file string) ([]byte, error) {
	set := 0
	if inline != "" {
		set++
	}
	if hex != "" {
		set++
	}
	if file != "" {
		set++
	}
	if set != 1 {
		return nil, fmt.Errorf("exactly one of --plaintext / --plaintext-hex / --plaintext-file is required")
	}
	switch {
	case inline != "":
		return []byte(inline), nil
	case hex != "":
		out, err := decodeHex(hex)
		if err != nil {
			return nil, fmt.Errorf("--plaintext-hex: %w", err)
		}
		return out, nil
	default:
		return os.ReadFile(file) // #nosec G304 -- operator-supplied file path
	}
}

// loadMessage returns raw message bytes from `--message` or
// `--message-file`. Exactly one must be set.
func loadMessage(inline, file string) ([]byte, error) {
	if inline != "" && file != "" {
		return nil, fmt.Errorf("only one of --message / --message-file may be set")
	}
	if inline == "" && file == "" {
		return nil, fmt.Errorf("--message or --message-file is required")
	}
	if inline != "" {
		return []byte(inline), nil
	}
	return os.ReadFile(file) // #nosec G304 -- operator-supplied file path
}

func decodeHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var hi, lo byte
		var err error
		if hi, err = hexNibble(s[i*2]); err != nil {
			return nil, err
		}
		if lo, err = hexNibble(s[i*2+1]); err != nil {
			return nil, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexNibble(b byte) (byte, error) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	}
	return 0, fmt.Errorf("invalid hex byte 0x%02x", b)
}
