package netconf

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeEOM(t *testing.T) {
	msg := []byte("<rpc/>")
	encoded := encodeEOM(msg)
	assert.True(t, bytes.HasSuffix(encoded, append(eomDelimiter, '\n')))
	assert.True(t, bytes.HasPrefix(encoded, msg))
}

func TestReadEOM_Simple(t *testing.T) {
	input := "<rpc-reply>ok</rpc-reply>]]>]]>"
	r := strings.NewReader(input)

	data, err := readEOM(r)
	require.NoError(t, err)
	assert.Equal(t, "<rpc-reply>ok</rpc-reply>", string(data))
}

func TestReadEOM_MultiRead(t *testing.T) {
	// Simulate a slow reader that delivers data in small chunks.
	msg := "<rpc-reply><data>test</data></rpc-reply>"
	full := msg + string(eomDelimiter)
	r := &slowReader{data: []byte(full), chunkSize: 5}

	data, err := readEOM(r)
	require.NoError(t, err)
	assert.Equal(t, msg, string(data))
}

func TestReadEOM_UnexpectedEOF(t *testing.T) {
	r := strings.NewReader("<incomplete")
	_, err := readEOM(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EOF")
}

func TestEncodeChunked(t *testing.T) {
	msg := []byte("<rpc/>")
	encoded := encodeChunked(msg)
	s := string(encoded)
	assert.Contains(t, s, "\n#6\n")
	assert.Contains(t, s, "<rpc/>")
	assert.True(t, strings.HasSuffix(s, "\n##\n"))
}

func TestReadChunked_Simple(t *testing.T) {
	input := "\n#6\n<rpc/>\n##\n"
	r := strings.NewReader(input)

	data, err := readChunked(r)
	require.NoError(t, err)
	assert.Equal(t, "<rpc/>", string(data))
}

func TestReadChunked_MultiChunk(t *testing.T) {
	input := "\n#3\nabc\n#3\ndef\n##\n"
	r := strings.NewReader(input)

	data, err := readChunked(r)
	require.NoError(t, err)
	assert.Equal(t, "abcdef", string(data))
}

func TestReadChunked_InvalidHeader(t *testing.T) {
	input := "no-hash\n##\n"
	r := strings.NewReader(input)

	_, err := readChunked(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid chunk header")
}

func TestReadChunked_InvalidSize(t *testing.T) {
	input := "\n#abc\ndata\n##\n"
	r := strings.NewReader(input)

	_, err := readChunked(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid chunk size")
}

func TestReadChunked_NegativeSize(t *testing.T) {
	input := "\n#-1\n\n##\n"
	r := strings.NewReader(input)

	_, err := readChunked(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestEncodeDecodeEOM_RoundTrip(t *testing.T) {
	original := []byte(`<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><get/></rpc>`)
	encoded := encodeEOM(original)

	r := bytes.NewReader(encoded)
	decoded, err := readEOM(r)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestEncodeDecodeChunked_RoundTrip(t *testing.T) {
	original := []byte(`<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><commit/></rpc>`)
	encoded := encodeChunked(original)

	r := bytes.NewReader(encoded)
	decoded, err := readChunked(r)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestLineReader_SingleLine(t *testing.T) {
	lr := newLineReader(strings.NewReader("hello\n"))
	line, err := lr.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, "hello", line)
}

func TestLineReader_MultiLine(t *testing.T) {
	lr := newLineReader(strings.NewReader("line1\nline2\nline3\n"))

	line, err := lr.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, "line1", line)

	line, err = lr.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, "line2", line)

	line, err = lr.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, "line3", line)
}

func TestLineReader_NoTrailingNewline(t *testing.T) {
	lr := newLineReader(strings.NewReader("no-newline"))
	line, err := lr.ReadLine()
	require.NoError(t, err)
	assert.Equal(t, "no-newline", line)
}

// slowReader delivers data in fixed-size chunks.
type slowReader struct {
	data      []byte
	pos       int
	chunkSize int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	end := r.pos + r.chunkSize
	if end > len(r.data) {
		end = len(r.data)
	}
	n := copy(p, r.data[r.pos:end])
	r.pos += n
	return n, nil
}
