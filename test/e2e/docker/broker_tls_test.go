package docker

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/natsauth"
)

// ADR-0002 § 12: TLS on every connection, terminating at the broker (G49).
//
// Each refusal below runs beside the one connection that must succeed, on the
// same broker. A broker that refused everyone would satisfy every refusal, and
// a client that could not authenticate would make a TLS refusal indistinguishable
// from an authorization one -- so the credential used throughout is one the
// broker is shown to accept.
func TestTheTopologysBrokerAcceptsOnlyVerifiedTLSFirstClients(t *testing.T) {
	requireDocker(t)

	deployment, err := natsauth.Generate(natsauth.Config{
		FleetSize: 4, BrokerNames: []string{brokerName}, RuntimeDir: brokerRuntimeDir,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	dir := t.TempDir()
	if err := deployment.Write(dir); err != nil {
		t.Fatalf("write deployment: %v", err)
	}
	seedConfigVolume(t, dir)
	url := composeUp(t)
	address := strings.TrimPrefix(url, "nats://")

	// A service principal's own credential: permitted to connect, by the
	// generated JWTs, whatever TLS does.
	presence := deployment.Services[natsauth.PresenceConsumer]
	creds, err := jwt.FormatUserConfig(presence.JWT, presence.Seed)
	if err != nil {
		t.Fatal(err)
	}
	credsPath := filepath.Join(dir, "presence.creds")
	if err := os.WriteFile(credsPath, creds, 0o600); err != nil {
		t.Fatal(err)
	}
	connect := func(opts ...nats.Option) error {
		conn, err := nats.Connect(url, append([]nats.Option{
			nats.UserCredentials(credsPath),
			nats.Timeout(10 * time.Second),
			nats.MaxReconnects(0),
			nats.NoReconnect(),
		}, opts...)...)
		if conn != nil {
			conn.Close()
		}
		return err
	}
	verified := func(name string, mutate func(*tls.Config)) *tls.Config {
		cfg, err := natsauth.ClientTLSConfig(deployment.TLS.CACertPEM, name)
		if err != nil {
			t.Fatal(err)
		}
		if mutate != nil {
			mutate(cfg)
		}
		return cfg
	}

	t.Run("a verified TLS-first client connects", func(t *testing.T) {
		if err := connect(brokerTLS(t, deployment)...); err != nil {
			t.Fatalf("the broker refused a client that verified it and spoke TLS first: %v", err)
		}
	})

	t.Run("a plaintext client is refused", func(t *testing.T) {
		if err := connect(); err == nil {
			t.Fatal("the broker accepted a plaintext client")
		}
	})

	t.Run("a TLS client that waits for a plaintext greeting is refused", func(t *testing.T) {
		if err := connect(nats.Secure(verified(brokerName, nil))); err == nil {
			t.Fatal("the broker accepted a client that did not speak TLS first")
		}
	})

	t.Run("a client expecting another name refuses the broker", func(t *testing.T) {
		err := connect(nats.Secure(verified("not-the-broker", nil)), nats.TLSHandshakeFirst())
		if err == nil || !strings.Contains(err.Error(), "certificate") {
			t.Fatalf("a client verifying another name connected, or failed for another reason: %v", err)
		}
	})

	t.Run("a client trusting another CA refuses the broker", func(t *testing.T) {
		other, err := natsauth.Generate(natsauth.Config{
			FleetSize: 1, BrokerNames: []string{brokerName}, RuntimeDir: brokerRuntimeDir,
		})
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := natsauth.ClientTLSConfig(other.TLS.CACertPEM, brokerName)
		if err != nil {
			t.Fatal(err)
		}
		err = connect(nats.Secure(cfg), nats.TLSHandshakeFirst())
		if err == nil || !strings.Contains(err.Error(), "unknown authority") {
			t.Fatalf("a client trusting another CA connected, or failed for another reason: %v", err)
		}
	})

	t.Run("a client limited to TLS 1.2 is refused", func(t *testing.T) {
		cfg := verified(brokerName, func(c *tls.Config) { c.MinVersion, c.MaxVersion = tls.VersionTLS12, tls.VersionTLS12 })
		if err := connect(nats.Secure(cfg), nats.TLSHandshakeFirst()); err == nil {
			t.Fatal("the broker negotiated TLS 1.2")
		}
	})

	// TLS-first means the broker speaks only after the client's ClientHello. A
	// broker that sent its INFO greeting first would hand any connector its
	// metadata in plaintext.
	t.Run("the broker sends nothing before the handshake", func(t *testing.T) {
		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 256)
		n, err := conn.Read(buf)
		if n > 0 {
			t.Fatalf("the broker sent %d plaintext bytes before any handshake: %q", n, buf[:n])
		}
		// Nothing sent, then either our deadline or the broker's own handshake
		// timeout closing the idle connection -- whichever comes first. Both
		// end the read with zero bytes; any other error is not this property.
		var ne net.Error
		if err == nil || !(errors.Is(err, io.EOF) || (asNetError(err, &ne) && ne.Timeout())) {
			t.Fatalf("expected the read to end with nothing sent, by timeout or close; got %v", err)
		}
	})

	// The only listener is the client port. A monitoring endpoint would be a
	// second, plaintext, connection surface on the same broker.
	t.Run("the broker listens on nothing but the client port", func(t *testing.T) {
		id := strings.TrimSpace(output(t, "docker", "compose", "-f", composeFile, "ps", "--quiet", "broker"))
		tables := output(t, "docker", "exec", id, "cat", "/proc/net/tcp", "/proc/net/tcp6")
		ports := listeningPorts(tables)
		if len(ports) == 0 {
			t.Fatalf("read no listening sockets, so this observation saw nothing:\n%s", tables)
		}
		for _, p := range ports {
			if p != 4222 {
				t.Errorf("the broker listens on port %d as well as the client port", p)
			}
		}
	})
}

func asNetError(err error, target *net.Error) bool {
	ne, ok := err.(net.Error)
	if ok {
		*target = ne
	}
	return ok
}

// dockerResolver is 127.0.0.11 as /proc/net/tcp writes it: little-endian hex.
// On a user-defined network Docker runs its embedded DNS resolver at that
// address, on a random port, inside every container's network namespace. It is
// the daemon's, not the broker's, and it is the one listener excluded; loopback
// in general is not, because a broker could bind a real endpoint there.
const dockerResolver = "0B00007F"

// listeningPorts reads /proc/net/tcp{,6} and returns every port in state LISTEN
// (0A), each once, except Docker's embedded resolver.
func listeningPorts(tables string) []int {
	seen := map[int]bool{}
	var ports []int
	for _, line := range strings.Split(tables, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[3] != "0A" {
			continue
		}
		local := f[1]
		i := strings.LastIndex(local, ":")
		if i < 0 || local[:i] == dockerResolver {
			continue
		}
		p, err := strconv.ParseInt(local[i+1:], 16, 32)
		if err != nil || seen[int(p)] {
			continue
		}
		seen[int(p)] = true
		ports = append(ports, int(p))
	}
	return ports
}
