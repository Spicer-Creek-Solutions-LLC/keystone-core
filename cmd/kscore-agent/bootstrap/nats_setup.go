package bootstrap

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

func setupNATS(cfg *BootstrapConfig, output io.Writer, verbose bool) error {
	if err := validateNATSURLs(cfg.NATSURLs); err != nil {
		return err
	}
	if cfg.NATSCredsFile != "" {
		if _, err := os.Stat(cfg.NATSCredsFile); err != nil {
			return fmt.Errorf("nats creds file not found: %w", err)
		}
	}

	// Create NATS store directories for embedded modes
	mode := strings.ToLower(cfg.NATSMode)
	if mode == "" || mode == "embedded" || mode == "cluster" || mode == "leaf" {
		if err := ensureNATSDirectories(output, verbose); err != nil {
			return err
		}
	}

	if verbose {
		fmt.Fprintln(output, "nats configuration validated")
	}
	return nil
}

func ensureNATSDirectories(output io.Writer, verbose bool) error {
	dirs := []string{natsStoreDir, jetstreamStoreDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create nats directory %s: %w", dir, err)
		}
		if verbose {
			fmt.Fprintf(output, "created nats directory %s\n", dir)
		}
	}
	return nil
}

func validateNATSURLs(urls []string) error {
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid nats url %q: %w", raw, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid nats url %q: missing scheme or host", raw)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "nats", "tls", "ws", "wss":
		default:
			return fmt.Errorf("invalid nats url %q: unsupported scheme", raw)
		}
	}
	return nil
}
