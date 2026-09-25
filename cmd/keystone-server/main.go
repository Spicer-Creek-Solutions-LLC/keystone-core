// Command keystone-server is the server.
//
// It owns the local operator socket and, when enrollment is configured, issues
// enrollment tokens through it.
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
	"go.keystone-core.io/keystone-core/internal/enrollment"
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
	// Enrollment's inputs are read and checked before anything is recorded or
	// served: a server that cannot enroll correctly does not start (ROLE-1,
	// SKEY-1).
	var enroll *enrollment.Server
	if cfg.Enrollment() {
		if enroll, err = enrollment.Open(cfg, st); err != nil {
			fmt.Fprintf(errOut, "%s: enrollment: %v\n", role, err)
			return cli.Local
		}
	}
	// ADR-0010 § 5: a fault point in the shipped binary is acceptable only
	// because enabling it is on record before the server serves.
	for _, f := range []struct {
		key string
		on  bool
	}{
		{"operator_hold_before_decision_ms", cfg.Faults.OperatorHoldBeforeDecision > 0},
		{"operator_hold_before_response_ms", cfg.Faults.OperatorHoldBeforeResponse > 0},
	} {
		if !f.on {
			continue
		}
		if err := st.AppendAuditRecord(context.Background(), store.AuditRecord{
			Action: operator.ActionFaultPrefix + f.key,
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
	if enroll != nil {
		srv.Handle(enrollment.OperationCreate, enroll.Issuer.CreateHandler())
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
