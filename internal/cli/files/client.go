// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/files/transport"
	natspkg "go.keystone-core.io/keystone-core/internal/nats"
)

// closeFn is the cleanup callback every subcommand defers after
// it has built a transport.Client. It closes the underlying NATS
// connection.
type closeFn func()

// connect builds the NATS connection + transport.Client every
// subcommand uses. The returned [closeFn] tears the NATS
// connection down; callers MUST defer it.
func (g *globals) connect() (*transport.Client, closeFn, error) {
	subjects, err := natspkg.NewSubjectBuilder(g.cluster)
	if err != nil {
		return nil, nil, fmt.Errorf("nats subject builder: %w", err)
	}
	conn, err := nats.Connect(g.natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("nats connect %s: %w", g.natsURL, err)
	}
	p, err := g.principal()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	opts := []transport.ClientOption{}
	if p != nil {
		opts = append(opts, transport.WithPrincipal(p))
	}
	c, err := transport.NewClient(conn, subjects, opts...)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("transport client: %w", err)
	}
	c.Timeout = g.timeout
	return c, func() { conn.Close() }, nil
}
