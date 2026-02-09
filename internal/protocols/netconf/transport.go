package netconf

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// EOM delimiter for NETCONF 1.0 message framing.
var eomDelimiter = []byte("]]>]]>")

// Transport handles NETCONF message framing over an SSH subsystem.
type Transport struct {
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	framing FramingMode
	mu      sync.Mutex
}

// NewTransport opens the "netconf" SSH subsystem and returns a Transport.
func NewTransport(client *ssh.Client) (*Transport, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new ssh session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := session.RequestSubsystem("netconf"); err != nil {
		session.Close()
		return nil, fmt.Errorf("request netconf subsystem: %w", err)
	}

	return &Transport{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		framing: FramingEOM,
	}, nil
}

// SetFraming switches the message framing mode.
func (t *Transport) SetFraming(mode FramingMode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.framing = mode
}

// Framing returns the current framing mode.
func (t *Transport) Framing() FramingMode {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.framing
}

// Send writes a framed NETCONF message.
func (t *Transport) Send(msg []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var data []byte
	switch t.framing {
	case FramingChunked:
		data = encodeChunked(msg)
	default:
		data = encodeEOM(msg)
	}

	_, err := t.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("transport send: %w", err)
	}
	return nil
}

// Receive reads and deframes a complete NETCONF message.
func (t *Transport) Receive() ([]byte, error) {
	t.mu.Lock()
	framing := t.framing
	t.mu.Unlock()

	switch framing {
	case FramingChunked:
		return readChunked(t.stdout)
	default:
		return readEOM(t.stdout)
	}
}

// Close closes the underlying SSH session.
func (t *Transport) Close() error {
	return t.session.Close()
}

// EOM framing: message terminated by ]]>]]>

func encodeEOM(msg []byte) []byte {
	buf := make([]byte, 0, len(msg)+len(eomDelimiter)+1)
	buf = append(buf, msg...)
	buf = append(buf, eomDelimiter...)
	buf = append(buf, '\n')
	return buf
}

func readEOM(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)

	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if idx := bytes.Index(buf.Bytes(), eomDelimiter); idx >= 0 {
				return buf.Bytes()[:idx], nil
			}
		}
		if err != nil {
			if err == io.EOF && buf.Len() > 0 {
				return nil, fmt.Errorf("unexpected EOF before EOM delimiter")
			}
			return nil, fmt.Errorf("read eom: %w", err)
		}
	}
}

// Chunked framing (RFC 6242):
// \n#<size>\n<data>\n##\n

func encodeChunked(msg []byte) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "\n#%d\n", len(msg))
	buf.Write(msg)
	buf.WriteString("\n##\n")
	return buf.Bytes()
}

func readChunked(r io.Reader) ([]byte, error) {
	br := newLineReader(r)
	var result bytes.Buffer

	for {
		line, err := br.ReadLine()
		if err != nil {
			return nil, fmt.Errorf("read chunked header: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == "##" {
			return result.Bytes(), nil
		}

		if !strings.HasPrefix(line, "#") {
			return nil, fmt.Errorf("invalid chunk header: %q", line)
		}

		sizeStr := line[1:]
		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid chunk size %q: %w", sizeStr, err)
		}
		if size <= 0 || size > 4294967295 {
			return nil, fmt.Errorf("chunk size out of range: %d", size)
		}

		chunk := make([]byte, size)
		if _, err := io.ReadFull(br, chunk); err != nil {
			return nil, fmt.Errorf("read chunk data: %w", err)
		}
		result.Write(chunk)
	}
}

// lineReader reads lines from a stream, buffering partial reads.
type lineReader struct {
	reader io.Reader
	buf    []byte
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{reader: r}
}

func (lr *lineReader) Read(p []byte) (int, error) {
	if len(lr.buf) > 0 {
		n := copy(p, lr.buf)
		lr.buf = lr.buf[n:]
		return n, nil
	}
	return lr.reader.Read(p)
}

func (lr *lineReader) ReadLine() (string, error) {
	for {
		if idx := bytes.IndexByte(lr.buf, '\n'); idx >= 0 {
			line := string(lr.buf[:idx])
			lr.buf = lr.buf[idx+1:]
			return line, nil
		}
		tmp := make([]byte, 512)
		n, err := lr.reader.Read(tmp)
		if n > 0 {
			lr.buf = append(lr.buf, tmp[:n]...)
		}
		if err != nil {
			if len(lr.buf) > 0 {
				line := string(lr.buf)
				lr.buf = nil
				return line, nil
			}
			return "", err
		}
	}
}
