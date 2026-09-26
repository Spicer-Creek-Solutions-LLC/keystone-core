// Command keystone-agent is the agent.
//
// C05 adds `keystone-agent enroll --token-file <path|->`, which turns a token
// bundle into a permanent identity (ADR-0003, RFC 0005). With no command, an
// enrolled agent connects with that identity and serves until stopped; one
// without an identity exits 1 and does nothing else (ADR-0003 § 9).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/enrollment"
	"go.keystone-core.io/keystone-core/internal/version"
)

const role = config.RoleAgent

// maxBundle bounds what is read as a bundle. A real one is a few kilobytes.
const maxBundle = 64 << 10

func main() {
	os.Exit(int(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)))
}

func run(args []string, in io.Reader, out, errOut io.Writer) cli.Code {
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
	switch {
	case len(rest) >= 1 && rest[0] == "enroll":
		return enroll(rest[1:], in, errOut)
	case len(rest) == 0:
		return serve(errOut)
	}
	fmt.Fprintf(errOut, "usage: %s [enroll --token-file <path|->]\n", role)
	return cli.Local
}

// serve runs the enrolled agent. It reconnects with its permanent identity and
// never re-enrolls: an agent with no identity fails rather than seek one.
func serve(errOut io.Writer) cli.Code {
	path, err := config.Locate(role, nil)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	cfg, err := config.LoadAgent(path)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	agent, err := enrollment.LoadEnrolled(cfg)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := agent.Run(ctx); err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	return cli.OK
}

// enroll reads the bundle from a file or standard input -- never from argv
// (ADR-0003 § 2) -- and exits 0 only once S6 has confirmed the identity.
func enroll(args []string, in io.Reader, errOut io.Writer) cli.Code {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	fs.SetOutput(errOut)
	tokenFile := fs.String("token-file", "", "the token bundle's path, or - for standard input")
	if err := fs.Parse(args); err != nil || *tokenFile == "" || fs.NArg() != 0 {
		fmt.Fprintf(errOut, "usage: %s enroll --token-file <path|->\n", role)
		return cli.Local
	}
	path, err := config.Locate(role, nil)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	cfg, err := config.LoadAgent(path)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	bundle, err := readBundle(*tokenFile, in)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	a, err := enrollment.NewAgent(cfg, bundle)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	err = a.Enroll(context.Background())
	switch {
	case errors.Is(err, enrollment.ErrRefused):
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Unauthorized
	case err != nil:
		fmt.Fprintf(errOut, "%s: %v\n", role, err)
		return cli.Local
	}
	// The bundle is useless once S6 has confirmed, and it is a credential until
	// it expires: it does not outlive a successful enrollment (FILE-2).
	if *tokenFile != "-" {
		if err := os.Remove(*tokenFile); err != nil {
			fmt.Fprintf(errOut, "%s: enrolled, but the bundle could not be removed: %v\n", role, err)
			return cli.Local
		}
	}
	return cli.OK
}

// readBundle refuses a bundle file anyone but its owner can read: a bundle is a
// credential, and a warning that is ignored leaves it readable on every host
// (ADR-0003 § 2).
func readBundle(path string, in io.Reader) ([]byte, error) {
	if path == "-" {
		return readLimited(in)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm()&0o044 != 0 {
		return nil, fmt.Errorf("%s is readable by group or other (mode %04o); a token bundle must be readable only by its owner", path, info.Mode().Perm())
	}
	return readLimited(f)
}

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBundle+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBundle {
		return nil, errors.New("the token bundle is larger than any bundle")
	}
	return b, nil
}
