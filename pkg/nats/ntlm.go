package nats

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
)

// NTLM message types
const (
	ntlmTypeNegotiate    = 1
	ntlmTypeChallenge    = 2
	ntlmTypeAuthenticate = 3
)

// NTLM negotiate flags
const (
	ntlmFlagNegotiateUnicode              = 0x00000001
	ntlmFlagNegotiateOEM                  = 0x00000002
	ntlmFlagRequestTarget                 = 0x00000004
	ntlmFlagNegotiateSign                 = 0x00000010
	ntlmFlagNegotiateSeal                 = 0x00000020
	ntlmFlagNegotiateDatagram             = 0x00000040
	ntlmFlagNegotiateLMKey                = 0x00000080
	ntlmFlagNegotiateNTLM                 = 0x00000200
	ntlmFlagNegotiateAnonymous            = 0x00000800
	ntlmFlagNegotiateDomainSupplied       = 0x00001000
	ntlmFlagNegotiateWorkstationSupplied  = 0x00002000
	ntlmFlagNegotiateAlwaysSign           = 0x00008000
	ntlmFlagNegotiateTargetTypeDomain     = 0x00010000
	ntlmFlagNegotiateTargetTypeServer     = 0x00020000
	ntlmFlagNegotiateExtendedSecurity     = 0x00080000
	ntlmFlagNegotiateIdentify             = 0x00100000
	ntlmFlagRequestNonNTSession           = 0x00400000
	ntlmFlagNegotiateTargetInfo           = 0x00800000
	ntlmFlagNegotiateVersion              = 0x02000000
	ntlmFlagNegotiate128                  = 0x20000000
	ntlmFlagNegotiateKeyExchange          = 0x40000000
	ntlmFlagNegotiate56                   = 0x80000000
)

// NTLM signature
var ntlmSignature = []byte("NTLMSSP\x00")

// NTLMAuth handles NTLM authentication
type NTLMAuth struct {
	Domain      string
	Username    string
	Password    string
	Workstation string
}

// NewNTLMAuth creates a new NTLM authenticator
func NewNTLMAuth(domain, username, password string) *NTLMAuth {
	return &NTLMAuth{
		Domain:      domain,
		Username:    username,
		Password:    password,
		Workstation: "WORKSTATION",
	}
}

// GenerateNegotiateMessage creates an NTLM Type 1 (Negotiate) message
func (n *NTLMAuth) GenerateNegotiateMessage() ([]byte, error) {
	flags := uint32(
		ntlmFlagNegotiateUnicode |
			ntlmFlagNegotiateOEM |
			ntlmFlagRequestTarget |
			ntlmFlagNegotiateNTLM |
			ntlmFlagNegotiateAlwaysSign |
			ntlmFlagNegotiateExtendedSecurity |
			ntlmFlagNegotiate128 |
			ntlmFlagNegotiate56,
	)

	// Type 1 message structure:
	// - Signature (8 bytes)
	// - MessageType (4 bytes)
	// - NegotiateFlags (4 bytes)
	// - DomainNameFields (8 bytes)
	// - WorkstationFields (8 bytes)
	// - Version (8 bytes) - optional
	// - Payload (domain, workstation)

	buf := new(bytes.Buffer)

	// Signature
	buf.Write(ntlmSignature)

	// MessageType
	binary.Write(buf, binary.LittleEndian, uint32(ntlmTypeNegotiate))

	// NegotiateFlags
	binary.Write(buf, binary.LittleEndian, flags)

	// Domain fields (Len, MaxLen, Offset) - empty for now
	binary.Write(buf, binary.LittleEndian, uint16(0)) // Len
	binary.Write(buf, binary.LittleEndian, uint16(0)) // MaxLen
	binary.Write(buf, binary.LittleEndian, uint32(0)) // Offset

	// Workstation fields (Len, MaxLen, Offset) - empty for now
	binary.Write(buf, binary.LittleEndian, uint16(0)) // Len
	binary.Write(buf, binary.LittleEndian, uint16(0)) // MaxLen
	binary.Write(buf, binary.LittleEndian, uint32(0)) // Offset

	return buf.Bytes(), nil
}

// ParseChallengeMessage parses an NTLM Type 2 (Challenge) message
func (n *NTLMAuth) ParseChallengeMessage(data []byte) (*NTLMChallenge, error) {
	if len(data) < 32 {
		return nil, errors.New("challenge message too short")
	}

	// Verify signature
	if !bytes.Equal(data[:8], ntlmSignature) {
		return nil, errors.New("invalid NTLM signature")
	}

	// Verify message type
	msgType := binary.LittleEndian.Uint32(data[8:12])
	if msgType != ntlmTypeChallenge {
		return nil, fmt.Errorf("expected challenge message type %d, got %d", ntlmTypeChallenge, msgType)
	}

	challenge := &NTLMChallenge{}

	// Target name fields (offset 12)
	targetNameLen := binary.LittleEndian.Uint16(data[12:14])
	targetNameOffset := binary.LittleEndian.Uint32(data[16:20])

	// Negotiate flags (offset 20)
	challenge.Flags = binary.LittleEndian.Uint32(data[20:24])

	// Server challenge (offset 24)
	copy(challenge.ServerChallenge[:], data[24:32])

	// Target name
	if targetNameLen > 0 && int(targetNameOffset+uint32(targetNameLen)) <= len(data) {
		challenge.TargetName = string(data[targetNameOffset : targetNameOffset+uint32(targetNameLen)])
	}

	// Target info (offset 40, if present)
	if len(data) >= 48 {
		targetInfoLen := binary.LittleEndian.Uint16(data[40:42])
		targetInfoOffset := binary.LittleEndian.Uint32(data[44:48])

		if targetInfoLen > 0 && int(targetInfoOffset+uint32(targetInfoLen)) <= len(data) {
			challenge.TargetInfo = data[targetInfoOffset : targetInfoOffset+uint32(targetInfoLen)]
		}
	}

	return challenge, nil
}

// NTLMChallenge represents a parsed Type 2 message
type NTLMChallenge struct {
	TargetName      string
	Flags           uint32
	ServerChallenge [8]byte
	TargetInfo      []byte
}

// GenerateAuthenticateMessage creates an NTLM Type 3 (Authenticate) message
func (n *NTLMAuth) GenerateAuthenticateMessage(challenge *NTLMChallenge) ([]byte, error) {
	// Use NTLMv2 for security
	ntlmHash := ntlmv2Hash(n.Password, n.Username, n.Domain)

	// Generate client challenge
	clientChallenge := make([]byte, 8)
	if _, err := rand.Read(clientChallenge); err != nil {
		return nil, fmt.Errorf("generate client challenge: %w", err)
	}

	// Generate timestamp
	timestamp := windowsTimestamp(time.Now())

	// Build NTLMv2 client blob
	blob := buildNTLMv2Blob(clientChallenge, timestamp, challenge.TargetInfo)

	// Calculate NTLMv2 response
	ntlmv2Response := calculateNTLMv2Response(ntlmHash, challenge.ServerChallenge[:], blob)

	// Calculate LMv2 response (simplified - just use NTLMv2 hash)
	lmv2Response := calculateLMv2Response(ntlmHash, challenge.ServerChallenge[:], clientChallenge)

	// Encode strings as UTF-16LE
	domainBytes := utf16LEEncode(strings.ToUpper(n.Domain))
	usernameBytes := utf16LEEncode(n.Username)
	workstationBytes := utf16LEEncode(strings.ToUpper(n.Workstation))

	// Calculate offsets
	// Fixed header size: 88 bytes (including version)
	headerSize := uint32(88)
	lmOffset := headerSize
	ntOffset := lmOffset + uint32(len(lmv2Response))
	domainOffset := ntOffset + uint32(len(ntlmv2Response))
	usernameOffset := domainOffset + uint32(len(domainBytes))
	workstationOffset := usernameOffset + uint32(len(usernameBytes))

	// Build message
	buf := new(bytes.Buffer)

	// Signature
	buf.Write(ntlmSignature)

	// MessageType
	binary.Write(buf, binary.LittleEndian, uint32(ntlmTypeAuthenticate))

	// LM response fields
	binary.Write(buf, binary.LittleEndian, uint16(len(lmv2Response)))  // Len
	binary.Write(buf, binary.LittleEndian, uint16(len(lmv2Response)))  // MaxLen
	binary.Write(buf, binary.LittleEndian, lmOffset)                   // Offset

	// NT response fields
	binary.Write(buf, binary.LittleEndian, uint16(len(ntlmv2Response))) // Len
	binary.Write(buf, binary.LittleEndian, uint16(len(ntlmv2Response))) // MaxLen
	binary.Write(buf, binary.LittleEndian, ntOffset)                    // Offset

	// Domain fields
	binary.Write(buf, binary.LittleEndian, uint16(len(domainBytes))) // Len
	binary.Write(buf, binary.LittleEndian, uint16(len(domainBytes))) // MaxLen
	binary.Write(buf, binary.LittleEndian, domainOffset)             // Offset

	// Username fields
	binary.Write(buf, binary.LittleEndian, uint16(len(usernameBytes))) // Len
	binary.Write(buf, binary.LittleEndian, uint16(len(usernameBytes))) // MaxLen
	binary.Write(buf, binary.LittleEndian, usernameOffset)             // Offset

	// Workstation fields
	binary.Write(buf, binary.LittleEndian, uint16(len(workstationBytes))) // Len
	binary.Write(buf, binary.LittleEndian, uint16(len(workstationBytes))) // MaxLen
	binary.Write(buf, binary.LittleEndian, workstationOffset)             // Offset

	// Encrypted random session key fields (empty)
	binary.Write(buf, binary.LittleEndian, uint16(0))                             // Len
	binary.Write(buf, binary.LittleEndian, uint16(0))                             // MaxLen
	binary.Write(buf, binary.LittleEndian, workstationOffset+uint32(len(workstationBytes))) // Offset

	// NegotiateFlags
	flags := uint32(
		ntlmFlagNegotiateUnicode |
			ntlmFlagRequestTarget |
			ntlmFlagNegotiateNTLM |
			ntlmFlagNegotiateAlwaysSign |
			ntlmFlagNegotiateExtendedSecurity |
			ntlmFlagNegotiate128 |
			ntlmFlagNegotiate56,
	)
	binary.Write(buf, binary.LittleEndian, flags)

	// Version (8 bytes) - Windows 10 version
	buf.Write([]byte{0x0a, 0x00, 0x63, 0x45, 0x00, 0x00, 0x00, 0x0f})

	// Payload
	buf.Write(lmv2Response)
	buf.Write(ntlmv2Response)
	buf.Write(domainBytes)
	buf.Write(usernameBytes)
	buf.Write(workstationBytes)

	return buf.Bytes(), nil
}

// ntlmv2Hash computes the NTLMv2 hash
func ntlmv2Hash(password, username, domain string) []byte {
	// NT hash = MD4(UTF16-LE(password))
	ntHash := md4Hash(utf16LEEncode(password))

	// NTLMv2 hash = HMAC-MD5(NT hash, uppercase(username) + uppercase(domain))
	userDomain := utf16LEEncode(strings.ToUpper(username) + strings.ToUpper(domain))

	h := hmac.New(md5.New, ntHash)
	h.Write(userDomain)
	return h.Sum(nil)
}

// md4Hash computes MD4 hash (required for NTLM)
func md4Hash(data []byte) []byte {
	// MD4 implementation for NTLM compatibility
	// Note: MD4 is cryptographically weak but required for NTLM protocol
	h := newMD4()
	h.Write(data)
	return h.Sum(nil)
}

// calculateNTLMv2Response computes the NTLMv2 response
func calculateNTLMv2Response(ntlmv2Hash, serverChallenge, blob []byte) []byte {
	// NTProofStr = HMAC-MD5(NTLMv2Hash, ServerChallenge + Blob)
	h := hmac.New(md5.New, ntlmv2Hash)
	h.Write(serverChallenge)
	h.Write(blob)
	ntProofStr := h.Sum(nil)

	// Response = NTProofStr + Blob
	response := make([]byte, len(ntProofStr)+len(blob))
	copy(response, ntProofStr)
	copy(response[len(ntProofStr):], blob)
	return response
}

// calculateLMv2Response computes the LMv2 response
func calculateLMv2Response(ntlmv2Hash, serverChallenge, clientChallenge []byte) []byte {
	// LMv2Response = HMAC-MD5(NTLMv2Hash, ServerChallenge + ClientChallenge) + ClientChallenge
	h := hmac.New(md5.New, ntlmv2Hash)
	h.Write(serverChallenge)
	h.Write(clientChallenge)
	proof := h.Sum(nil)

	response := make([]byte, len(proof)+len(clientChallenge))
	copy(response, proof)
	copy(response[len(proof):], clientChallenge)
	return response
}

// buildNTLMv2Blob builds the NTLMv2 client blob
func buildNTLMv2Blob(clientChallenge []byte, timestamp uint64, targetInfo []byte) []byte {
	buf := new(bytes.Buffer)

	// Blob signature
	binary.Write(buf, binary.LittleEndian, uint32(0x00000101))

	// Reserved
	binary.Write(buf, binary.LittleEndian, uint32(0))

	// Timestamp
	binary.Write(buf, binary.LittleEndian, timestamp)

	// Client challenge
	buf.Write(clientChallenge)

	// Reserved
	binary.Write(buf, binary.LittleEndian, uint32(0))

	// Target info
	if len(targetInfo) > 0 {
		buf.Write(targetInfo)
	}

	// End of target info (MsvAvEOL)
	binary.Write(buf, binary.LittleEndian, uint32(0))

	return buf.Bytes()
}

// windowsTimestamp returns the current time as Windows FILETIME
func windowsTimestamp(t time.Time) uint64 {
	// Windows FILETIME: 100-nanosecond intervals since January 1, 1601
	const epochDiff = 116444736000000000 // 100-ns intervals between 1601 and 1970
	return uint64(t.UnixNano()/100) + epochDiff
}

// utf16LEEncode encodes a string as UTF-16LE
func utf16LEEncode(s string) []byte {
	runes := utf16.Encode([]rune(s))
	b := make([]byte, len(runes)*2)
	for i, r := range runes {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return b
}

// ParseNTLMAuthHeader parses the Proxy-Authenticate header for NTLM challenge
func ParseNTLMAuthHeader(header string) ([]byte, error) {
	// Header format: "NTLM <base64-encoded-challenge>"
	const prefix = "NTLM "
	if !strings.HasPrefix(header, prefix) {
		return nil, errors.New("invalid NTLM auth header format")
	}

	encoded := strings.TrimPrefix(header, prefix)
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		// Try base64 decoding instead
		return base64Decode(encoded)
	}
	return decoded, nil
}

// base64Decode decodes base64 string
func base64Decode(s string) ([]byte, error) {
	// Standard base64 decoding
	s = strings.TrimSpace(s)
	// Pad if necessary
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}

	decoded := make([]byte, len(s))
	n := 0
	for i := 0; i < len(s); i += 4 {
		var val uint32
		for j := 0; j < 4 && i+j < len(s); j++ {
			val <<= 6
			c := s[i+j]
			switch {
			case c >= 'A' && c <= 'Z':
				val |= uint32(c - 'A')
			case c >= 'a' && c <= 'z':
				val |= uint32(c - 'a' + 26)
			case c >= '0' && c <= '9':
				val |= uint32(c - '0' + 52)
			case c == '+':
				val |= 62
			case c == '/':
				val |= 63
			case c == '=':
				// Padding
			default:
				return nil, fmt.Errorf("invalid base64 character: %c", c)
			}
		}

		if i+3 < len(s) && s[i+3] == '=' {
			if s[i+2] == '=' {
				decoded[n] = byte(val >> 16)
				n++
			} else {
				decoded[n] = byte(val >> 16)
				decoded[n+1] = byte(val >> 8)
				n += 2
			}
		} else {
			decoded[n] = byte(val >> 16)
			decoded[n+1] = byte(val >> 8)
			decoded[n+2] = byte(val)
			n += 3
		}
	}
	return decoded[:n], nil
}

// base64Encode encodes bytes to base64
func base64Encode(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := make([]byte, ((len(data)+2)/3)*4)
	j := 0
	for i := 0; i < len(data); i += 3 {
		var val uint32
		remaining := len(data) - i
		if remaining >= 3 {
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result[j] = alphabet[val>>18&0x3F]
			result[j+1] = alphabet[val>>12&0x3F]
			result[j+2] = alphabet[val>>6&0x3F]
			result[j+3] = alphabet[val&0x3F]
		} else if remaining == 2 {
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result[j] = alphabet[val>>18&0x3F]
			result[j+1] = alphabet[val>>12&0x3F]
			result[j+2] = alphabet[val>>6&0x3F]
			result[j+3] = '='
		} else {
			val = uint32(data[i]) << 16
			result[j] = alphabet[val>>18&0x3F]
			result[j+1] = alphabet[val>>12&0x3F]
			result[j+2] = '='
			result[j+3] = '='
		}
		j += 4
	}
	return string(result)
}

// MD4 implementation for NTLM (required by protocol)
// This is a minimal implementation - MD4 is cryptographically broken
// but required for NTLM protocol compatibility
// Based on RFC 1320

type md4State struct {
	s   [4]uint32
	x   [64]byte
	nx  int
	len uint64
}

func newMD4() *md4State {
	d := new(md4State)
	d.Reset()
	return d
}

func (d *md4State) Reset() {
	d.s[0] = 0x67452301
	d.s[1] = 0xefcdab89
	d.s[2] = 0x98badcfe
	d.s[3] = 0x10325476
	d.nx = 0
	d.len = 0
}

func (d *md4State) Write(p []byte) (nn int, err error) {
	nn = len(p)
	d.len += uint64(nn)
	if d.nx > 0 {
		n := copy(d.x[d.nx:], p)
		d.nx += n
		if d.nx == 64 {
			d.block(d.x[:])
			d.nx = 0
		}
		p = p[n:]
	}
	for len(p) >= 64 {
		d.block(p[:64])
		p = p[64:]
	}
	if len(p) > 0 {
		d.nx = copy(d.x[:], p)
	}
	return
}

func (d *md4State) Sum(in []byte) []byte {
	d0 := *d
	hash := d0.checkSum()
	return append(in, hash[:]...)
}

func (d *md4State) checkSum() [16]byte {
	// Save the original length before padding
	bitLen := d.len << 3

	tmp := [64]byte{0x80}
	if d.nx < 56 {
		d.Write(tmp[:56-d.nx])
	} else {
		d.Write(tmp[:64+56-d.nx])
	}

	binary.LittleEndian.PutUint64(tmp[:], bitLen)
	d.Write(tmp[:8])

	var digest [16]byte
	binary.LittleEndian.PutUint32(digest[0:], d.s[0])
	binary.LittleEndian.PutUint32(digest[4:], d.s[1])
	binary.LittleEndian.PutUint32(digest[8:], d.s[2])
	binary.LittleEndian.PutUint32(digest[12:], d.s[3])
	return digest
}

// md4F is the F function: (X AND Y) OR (NOT X AND Z)
func md4F(x, y, z uint32) uint32 {
	return (x & y) | (^x & z)
}

// md4G is the G function: (X AND Y) OR (X AND Z) OR (Y AND Z)
func md4G(x, y, z uint32) uint32 {
	return (x & y) | (x & z) | (y & z)
}

// md4H is the H function: X XOR Y XOR Z
func md4H(x, y, z uint32) uint32 {
	return x ^ y ^ z
}

// leftRotate performs a left rotation
func leftRotate(x uint32, n uint) uint32 {
	return (x << n) | (x >> (32 - n))
}

func (d *md4State) block(p []byte) {
	var x [16]uint32
	for i := 0; i < 16; i++ {
		x[i] = binary.LittleEndian.Uint32(p[i*4:])
	}

	a, b, c, dd := d.s[0], d.s[1], d.s[2], d.s[3]

	// Round 1
	a = leftRotate(a+md4F(b, c, dd)+x[0], 3)
	dd = leftRotate(dd+md4F(a, b, c)+x[1], 7)
	c = leftRotate(c+md4F(dd, a, b)+x[2], 11)
	b = leftRotate(b+md4F(c, dd, a)+x[3], 19)
	a = leftRotate(a+md4F(b, c, dd)+x[4], 3)
	dd = leftRotate(dd+md4F(a, b, c)+x[5], 7)
	c = leftRotate(c+md4F(dd, a, b)+x[6], 11)
	b = leftRotate(b+md4F(c, dd, a)+x[7], 19)
	a = leftRotate(a+md4F(b, c, dd)+x[8], 3)
	dd = leftRotate(dd+md4F(a, b, c)+x[9], 7)
	c = leftRotate(c+md4F(dd, a, b)+x[10], 11)
	b = leftRotate(b+md4F(c, dd, a)+x[11], 19)
	a = leftRotate(a+md4F(b, c, dd)+x[12], 3)
	dd = leftRotate(dd+md4F(a, b, c)+x[13], 7)
	c = leftRotate(c+md4F(dd, a, b)+x[14], 11)
	b = leftRotate(b+md4F(c, dd, a)+x[15], 19)

	// Round 2
	a = leftRotate(a+md4G(b, c, dd)+x[0]+0x5a827999, 3)
	dd = leftRotate(dd+md4G(a, b, c)+x[4]+0x5a827999, 5)
	c = leftRotate(c+md4G(dd, a, b)+x[8]+0x5a827999, 9)
	b = leftRotate(b+md4G(c, dd, a)+x[12]+0x5a827999, 13)
	a = leftRotate(a+md4G(b, c, dd)+x[1]+0x5a827999, 3)
	dd = leftRotate(dd+md4G(a, b, c)+x[5]+0x5a827999, 5)
	c = leftRotate(c+md4G(dd, a, b)+x[9]+0x5a827999, 9)
	b = leftRotate(b+md4G(c, dd, a)+x[13]+0x5a827999, 13)
	a = leftRotate(a+md4G(b, c, dd)+x[2]+0x5a827999, 3)
	dd = leftRotate(dd+md4G(a, b, c)+x[6]+0x5a827999, 5)
	c = leftRotate(c+md4G(dd, a, b)+x[10]+0x5a827999, 9)
	b = leftRotate(b+md4G(c, dd, a)+x[14]+0x5a827999, 13)
	a = leftRotate(a+md4G(b, c, dd)+x[3]+0x5a827999, 3)
	dd = leftRotate(dd+md4G(a, b, c)+x[7]+0x5a827999, 5)
	c = leftRotate(c+md4G(dd, a, b)+x[11]+0x5a827999, 9)
	b = leftRotate(b+md4G(c, dd, a)+x[15]+0x5a827999, 13)

	// Round 3
	a = leftRotate(a+md4H(b, c, dd)+x[0]+0x6ed9eba1, 3)
	dd = leftRotate(dd+md4H(a, b, c)+x[8]+0x6ed9eba1, 9)
	c = leftRotate(c+md4H(dd, a, b)+x[4]+0x6ed9eba1, 11)
	b = leftRotate(b+md4H(c, dd, a)+x[12]+0x6ed9eba1, 15)
	a = leftRotate(a+md4H(b, c, dd)+x[2]+0x6ed9eba1, 3)
	dd = leftRotate(dd+md4H(a, b, c)+x[10]+0x6ed9eba1, 9)
	c = leftRotate(c+md4H(dd, a, b)+x[6]+0x6ed9eba1, 11)
	b = leftRotate(b+md4H(c, dd, a)+x[14]+0x6ed9eba1, 15)
	a = leftRotate(a+md4H(b, c, dd)+x[1]+0x6ed9eba1, 3)
	dd = leftRotate(dd+md4H(a, b, c)+x[9]+0x6ed9eba1, 9)
	c = leftRotate(c+md4H(dd, a, b)+x[5]+0x6ed9eba1, 11)
	b = leftRotate(b+md4H(c, dd, a)+x[13]+0x6ed9eba1, 15)
	a = leftRotate(a+md4H(b, c, dd)+x[3]+0x6ed9eba1, 3)
	dd = leftRotate(dd+md4H(a, b, c)+x[11]+0x6ed9eba1, 9)
	c = leftRotate(c+md4H(dd, a, b)+x[7]+0x6ed9eba1, 11)
	b = leftRotate(b+md4H(c, dd, a)+x[15]+0x6ed9eba1, 15)

	d.s[0] += a
	d.s[1] += b
	d.s[2] += c
	d.s[3] += dd
}
