// SPDX-License-Identifier: Apache-2.0

// kscore-registry is the standalone v1.0 filesystem-backed module
// registry server (Epic 14 task 9). It serves the Go module-proxy
// read endpoints (`/<mod>/@v/{list,<ver>.info,<ver>.mod,<ver>.zip}`)
// plus `POST /publish` (multipart manifest + module ZIP) over a
// filesystem storage backend.
//
// v1.0 publish is unauthenticated: trust is the TLS-trusted
// registry transport + Cosign verification at load time. Publish
// authentication is a deferred v1.x ROADMAP item.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/registry/storage"
	"go.keystone-core.io/keystone-core/pkg/module/registry"
	"go.keystone-core.io/keystone-core/pkg/version"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

// newCommand returns the root cobra command (exposed for tests, the
// kscore-migrate precedent).
func newCommand() *cobra.Command {
	info := version.Get()
	root := &cobra.Command{
		Use:           "kscore-registry",
		Short:         "Keystone Core module registry server",
		Version:       info.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate(versionTemplate(info))
	root.AddCommand(newServeCommand())
	root.AddCommand(newVersionCommand())
	return root
}

func versionTemplate(info version.Info) string {
	return fmt.Sprintf("kscore-registry %s\ncommit: %s\nbuilt:  %s\n",
		info.Version, info.GitCommit, info.BuildDate)
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version metadata",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprint(cmd.OutOrStdout(), versionTemplate(version.Get()))
		},
	}
}

func newServeCommand() *cobra.Command {
	var (
		addr      string
		dir       string
		maxUpload int64
	)
	cmd := &cobra.Command{
		Use:           "serve",
		Short:         "Run the registry HTTP server",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			log := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))
			return serve(ctx, log, addr, dir, maxUpload)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8181", "listen address")
	cmd.Flags().StringVar(&dir, "dir", "./registry-data", "filesystem storage root")
	cmd.Flags().Int64Var(&maxUpload, "max-upload", registry.DefaultMaxUpload,
		"max publish request body size in bytes")
	return cmd
}

// serve builds the registry over a filesystem backend and runs the
// HTTP server until ctx is cancelled (SIGTERM/SIGINT), then drains
// within shutdownTimeout — the cmd/kscore-server lifecycle pattern.
func serve(ctx context.Context, log *slog.Logger, addr, dir string, maxUpload int64) error {
	st, err := storage.NewFilesystem(dir)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	mux := http.NewServeMux()
	registry.NewHandlerWithLimit(registry.New(st), maxUpload).Register(mux)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("kscore-registry serving", "addr", addr, "dir", dir)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			errCh <- e
			return
		}
		errCh <- nil
	}()

	select {
	case e := <-errCh:
		return e
	case <-ctx.Done():
		log.Info("kscore-registry shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if e := srv.Shutdown(sctx); e != nil {
			return fmt.Errorf("shutdown: %w", e)
		}
		return nil
	}
}
