package bootstrap

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/user"
	"strconv"
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
	// NATS creates a "jetstream" subdirectory inside natsStoreDir regardless of
	// the jetstream.storedir config setting. We must create this subdirectory
	// with kscore ownership so the server (running as kscore user) can write to it.
	natsJetstreamSubdir := natsStoreDir + "/jetstream"
	dirs := []string{natsStoreDir, jetstreamStoreDir, natsJetstreamSubdir}

	// Lookup kscore user for ownership
	kscoreUser, err := user.Lookup("kscore")
	if err != nil {
		// If kscore user doesn't exist, create directories without chown
		// This allows bootstrap to work in environments without the user
		if verbose {
			fmt.Fprintf(output, "kscore user not found, creating directories without chown: %v\n", err)
		}
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

	uid, err := strconv.Atoi(kscoreUser.Uid)
	if err != nil {
		return fmt.Errorf("parse kscore uid: %w", err)
	}
	gid, err := strconv.Atoi(kscoreUser.Gid)
	if err != nil {
		return fmt.Errorf("parse kscore gid: %w", err)
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create nats directory %s: %w", dir, err)
		}
		if err := os.Chown(dir, uid, gid); err != nil {
			return fmt.Errorf("chown nats directory %s: %w", dir, err)
		}
		if verbose {
			fmt.Fprintf(output, "created nats directory %s (owner: kscore)\n", dir)
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
