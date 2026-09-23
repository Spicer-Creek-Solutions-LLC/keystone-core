// Command keystone-server is the server.
//
// It owns the local operator socket. Journey operations and NATS transport are
// introduced by later C-stage tasks.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/operator"
	"go.keystone-core.io/keystone-core/internal/store"
	"go.keystone-core.io/keystone-core/internal/version"
)

const role = config.RoleServer

func main() {
	os.Exit(int(run(os.Args[1:], os.Stdout, os.Stderr)))
}

func run(args []string, out, errOut *os.File) cli.Code {
	fs := flag.NewFlagSet(string(role), flag.ContinueOnError)
	fs.SetOutput(errOut)
	showVersion := fs.Bool("version", false, "print the version and exit")
	showConfig := fs.Bool("config-path", false, "print the configuration path that would be read, and exit")
	if err := fs.Parse(args); err != nil {
		return cli.Local
	}

	switch {
	case *showVersion:
		fmt.Fprintln(out, version.String())
		return cli.OK
	case *showConfig:
		p, err := config.Locate(role, nil)
		if err != nil {
			fmt.Fprintf(errOut, "%s: %v\n", role, err)
			return cli.Local
		}
		fmt.Fprintln(out, p)
		return cli.OK
	}

	path, err := config.Locate(role, nil)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	cfg, err := config.LoadServer(path)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	st, err := store.Open(cfg.Store.Path)
	if err != nil {
		fmt.Fprintf(errOut, "%s: open store: %v\n", role, err)
		return cli.Local
	}
	defer st.Close()
	if cfg.Faults.OperatorHoldBeforeDecision > 0 {
		if err := st.AppendAuditRecord(context.Background(), store.AuditRecord{
			Action: operator.ActionFaultPrefix + "operator_hold_before_decision_ms",
			Result: "enabled", At: time.Now(),
		}); err != nil {
			fmt.Fprintf(errOut, "%s: record enabled fault: %v\n", role, err)
			return cli.Local
		}
	}
	srv, err := operator.New(cfg, st)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	if err := srv.Listen(); err != nil {
		fmt.Fprintf(errOut, "%s: listen: %v\n", role, err)
		return cli.Local
	}
	defer srv.Close()
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stopped)
	go func() {
		<-stopped
		srv.Close()
	}()
	if err := srv.Serve(); err != nil {
		fmt.Fprintf(errOut, "%s: serve: %v\n", role, err)
		return cli.Local
	}
	return cli.OK
}
