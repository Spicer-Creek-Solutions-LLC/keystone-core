// Command keystone-server is the server.
//
// P11a lands scaffolding and no behaviour: it reports its version and the
// configuration it would read. No journey verb, no NATS connection, no operator
// socket — ADR-0010's D-P11-4 and P11's AC-9.
package main

import (
	"flag"
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
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

	fmt.Fprintf(errOut, "%s %s: no command. P11a lands scaffolding only; the journeys are C-stage work.\n",
		role, version.String())
	return cli.Local
}
