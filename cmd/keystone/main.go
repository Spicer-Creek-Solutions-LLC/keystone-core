// Command keystone is the operator CLI.
//
// It speaks to the local server over ADR-0009's operator socket. C05 adds the
// first journey verb, `keystone enroll create`.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/operator"
	"go.keystone-core.io/keystone-core/internal/version"
)

const role = config.RoleOperator

func main() {
	os.Exit(int(run(os.Args[1:], os.Stdout, os.Stderr)))
}

func run(args []string, out, errOut io.Writer) cli.Code {
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

	rest := fs.Args()
	if len(rest) >= 2 && rest[0] == "enroll" && rest[1] == "create" {
		return enrollCreate(rest[2:], out, errOut)
	}
	fmt.Fprintf(errOut, "usage: %s enroll create --agent-name <name> [--ttl-seconds <n>]\n", role)
	return cli.Local
}

// enrollCreate issues one token and prints its bundle once, to standard output
// and nowhere else. The bundle carries the bootstrap credential, which is the
// token's secret (RFC 0005); the operator decides where it goes next.
func enrollCreate(args []string, out, errOut io.Writer) cli.Code {
	fs := flag.NewFlagSet("enroll create", flag.ContinueOnError)
	fs.SetOutput(errOut)
	name := fs.String("agent-name", "", "the operator's label for the agent; not its identifier")
	ttl := fs.Int64("ttl-seconds", 0, fmt.Sprintf("token lifetime in seconds, %d to %d; default %d",
		int64(enrollment.MinTokenTTL/time.Second), int64(enrollment.MaxTokenTTL/time.Second), int64(enrollment.DefaultTokenTTL/time.Second)))
	if err := fs.Parse(args); err != nil {
		return cli.Local
	}
	if fs.NArg() != 0 || !enrollment.ValidAgentName(*name) {
		fmt.Fprintf(errOut, "%s enroll create: --agent-name is required and must be printable text of at most %d bytes\n", role, enrollment.MaxAgentName)
		return cli.Local
	}
	if *ttl != 0 {
		if _, err := enrollment.TokenTTL(time.Duration(*ttl) * time.Second); err != nil {
			fmt.Fprintf(errOut, "%s enroll create: %v\n", role, err)
			return cli.Local
		}
	}
	var resp enrollment.CreateResponse
	err := operator.Call(operator.SocketPath, enrollment.CreateRequest{
		Op: enrollment.OperationCreate, AgentName: *name, TTLSeconds: *ttl,
	}, &resp)
	switch {
	case errors.Is(err, operator.ErrUnauthorized):
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Unauthorized
	case err != nil:
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	b, err := json.Marshal(resp.Bundle)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	if _, err := fmt.Fprintf(out, "%s\n", b); err != nil {
		fmt.Fprintf(errOut, "%s: writing the bundle: %v\n", role, err)
		return cli.Local
	}
	return cli.OK
}
