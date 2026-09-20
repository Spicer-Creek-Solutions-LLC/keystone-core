// Command keystone-ledger-crash is the C02 crash-boundary harness.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/ledger"
)

func main() {
	path := flag.String("ledger", "", "ledger path")
	step := flag.String("stop-after", "none", "halt boundary: none, receipt, or start")
	job := flag.String("job", "job-c02", "job identifier")
	flag.Parse()
	if *path == "" || (*step != "none" && *step != "receipt" && *step != "start") {
		fmt.Fprintln(os.Stderr, "usage: keystone-ledger-crash --ledger PATH --stop-after none|receipt|start --job ID")
		os.Exit(1)
	}
	l, err := ledger.Open(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx := context.Background()
	if *step == "receipt" || *step == "start" {
		if err := l.RecordReceipt(ctx, ledger.Receipt{JobID: *job, Metadata: []byte("authenticated-envelope"), State: "Received", ReceivedAt: time.Now().UTC()}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *step == "start" {
		if err := l.RecordStart(ctx, ledger.Start{JobID: *job, StartedAt: time.Now().UTC()}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	// Deliberately do not close the database or run deferred cleanup. The
	// harness models a process crash at the selected commit boundary.
	os.Exit(0)
}
