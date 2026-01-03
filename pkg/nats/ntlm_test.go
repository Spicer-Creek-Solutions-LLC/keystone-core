package nats

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestNewNTLMAuth(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")
	if ntlm.Domain != "DOMAIN" {
		t.Errorf("expected domain DOMAIN, got %s", ntlm.Domain)
	}
	if ntlm.Username != "user" {
		t.Errorf("expected username user, got %s", ntlm.Username)
	}
	if ntlm.Password != "password" {
		t.Errorf("expected password password, got %s", ntlm.Password)
	}
	if ntlm.Workstation != "WORKSTATION" {
		t.Errorf("expected workstation WORKSTATION, got %s", ntlm.Workstation)
	}
}

func TestGenerateNegotiateMessage(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")
	msg, err := ntlm.GenerateNegotiateMessage()
	if err != nil {
		t.Fatalf("GenerateNegotiateMessage failed: %v", err)
	}

	// Verify signature
	if !bytes.Equal(msg[:8], ntlmSignature) {
		t.Errorf("invalid signature: %v", msg[:8])
	}

	// Verify message type (offset 8)
	msgType := binary.LittleEndian.Uint32(msg[8:12])
	if msgType != ntlmTypeNegotiate {
		t.Errorf("expected message type %d, got %d", ntlmTypeNegotiate, msgType)
	}

	// Verify flags are set
	flags := binary.LittleEndian.Uint32(msg[12:16])
	if flags&ntlmFlagNegotiateUnicode == 0 {
		t.Error("NegotiateUnicode flag not set")
	}
	if flags&ntlmFlagNegotiateNTLM == 0 {
		t.Error("NegotiateNTLM flag not set")
	}
	if flags&ntlmFlagNegotiateExtendedSecurity == 0 {
		t.Error("NegotiateExtendedSecurity flag not set")
	}
}

func TestParseChallengeMessage(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")

	// Create a mock challenge message
	challenge := buildMockChallenge()

	parsed, err := ntlm.ParseChallengeMessage(challenge)
	if err != nil {
		t.Fatalf("ParseChallengeMessage failed: %v", err)
	}

	// Verify server challenge was extracted
	expectedChallenge := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if parsed.ServerChallenge != expectedChallenge {
		t.Errorf("server challenge mismatch: got %v, expected %v", parsed.ServerChallenge, expectedChallenge)
	}
}

func TestParseChallengeMessageTooShort(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")

	// Too short message
	_, err := ntlm.ParseChallengeMessage([]byte("short"))
	if err == nil {
		t.Error("expected error for short message")
	}
}

func TestParseChallengeMessageInvalidSignature(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")

	// Invalid signature
	invalid := make([]byte, 32)
	copy(invalid[:8], []byte("INVALID\x00"))
	_, err := ntlm.ParseChallengeMessage(invalid)
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}

func TestParseChallengeMessageWrongType(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")

	// Wrong message type (Type 1 instead of Type 2)
	msg := make([]byte, 32)
	copy(msg[:8], ntlmSignature)
	binary.LittleEndian.PutUint32(msg[8:12], ntlmTypeNegotiate) // Wrong type
	_, err := ntlm.ParseChallengeMessage(msg)
	if err == nil {
		t.Error("expected error for wrong message type")
	}
}

func TestGenerateAuthenticateMessage(t *testing.T) {
	ntlm := NewNTLMAuth("DOMAIN", "user", "password")

	// Create a mock challenge
	challenge := &NTLMChallenge{
		TargetName:      "TARGET",
		Flags:           ntlmFlagNegotiateUnicode | ntlmFlagNegotiateNTLM | ntlmFlagNegotiateExtendedSecurity,
		ServerChallenge: [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		TargetInfo:      nil,
	}

	msg, err := ntlm.GenerateAuthenticateMessage(challenge)
	if err != nil {
		t.Fatalf("GenerateAuthenticateMessage failed: %v", err)
	}

	// Verify signature
	if !bytes.Equal(msg[:8], ntlmSignature) {
		t.Errorf("invalid signature: %v", msg[:8])
	}

	// Verify message type (offset 8)
	msgType := binary.LittleEndian.Uint32(msg[8:12])
	if msgType != ntlmTypeAuthenticate {
		t.Errorf("expected message type %d, got %d", ntlmTypeAuthenticate, msgType)
	}
}

func TestNtlmv2Hash(t *testing.T) {
	// Test NTLMv2 hash computation
	hash := ntlmv2Hash("Password", "User", "Domain")
	if len(hash) != 16 {
		t.Errorf("expected 16 byte hash, got %d bytes", len(hash))
	}
}

func TestUtf16LEEncode(t *testing.T) {
	tests := []struct {
		input    string
		expected []byte
	}{
		{"A", []byte{0x41, 0x00}},
		{"AB", []byte{0x41, 0x00, 0x42, 0x00}},
		{"Hello", []byte{0x48, 0x00, 0x65, 0x00, 0x6c, 0x00, 0x6c, 0x00, 0x6f, 0x00}},
	}

	for _, tt := range tests {
		result := utf16LEEncode(tt.input)
		if !bytes.Equal(result, tt.expected) {
			t.Errorf("utf16LEEncode(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestBase64EncodeAndDecode(t *testing.T) {
	tests := []struct {
		input []byte
	}{
		{[]byte("Hello")},
		{[]byte("Hello World!")},
		{[]byte{0x00, 0x01, 0x02, 0x03}},
		{ntlmSignature},
	}

	for _, tt := range tests {
		encoded := base64Encode(tt.input)
		decoded, err := base64Decode(encoded)
		if err != nil {
			t.Errorf("base64Decode failed for input %v: %v", tt.input, err)
			continue
		}
		if !bytes.Equal(decoded, tt.input) {
			t.Errorf("round-trip failed: input=%v, encoded=%s, decoded=%v", tt.input, encoded, decoded)
		}
	}
}

func TestParseNTLMCredentials(t *testing.T) {
	tests := []struct {
		input          string
		expectedDomain string
		expectedUser   string
	}{
		{"DOMAIN\\user", "DOMAIN", "user"},
		{"user@domain.com", "domain.com", "user"},
		{"user", "", "user"},
		{"", "", ""},
	}

	for _, tt := range tests {
		domain, user := parseNTLMCredentials(tt.input)
		if domain != tt.expectedDomain {
			t.Errorf("parseNTLMCredentials(%q): domain = %q, expected %q", tt.input, domain, tt.expectedDomain)
		}
		if user != tt.expectedUser {
			t.Errorf("parseNTLMCredentials(%q): user = %q, expected %q", tt.input, user, tt.expectedUser)
		}
	}
}

func TestParseNTLMChallengeHeader(t *testing.T) {
	// Create a valid base64-encoded NTLM message
	msg := make([]byte, 32)
	copy(msg[:8], ntlmSignature)
	binary.LittleEndian.PutUint32(msg[8:12], ntlmTypeChallenge)

	encoded := base64Encode(msg)
	header := "NTLM " + encoded

	decoded, err := parseNTLMChallengeHeader(header)
	if err != nil {
		t.Fatalf("parseNTLMChallengeHeader failed: %v", err)
	}
	if !bytes.Equal(decoded, msg) {
		t.Errorf("decoded message mismatch")
	}
}

func TestParseNTLMChallengeHeaderInvalid(t *testing.T) {
	// Invalid prefix
	_, err := parseNTLMChallengeHeader("Basic xyz")
	if err == nil {
		t.Error("expected error for invalid prefix")
	}
}

func TestMD4Hash(t *testing.T) {
	// Test MD4 hash with known value
	// MD4("") = 31d6cfe0d16ae931b73c59d7e0c089c0
	result := md4Hash([]byte(""))
	expected := []byte{0x31, 0xd6, 0xcf, 0xe0, 0xd1, 0x6a, 0xe9, 0x31, 0xb7, 0x3c, 0x59, 0xd7, 0xe0, 0xc0, 0x89, 0xc0}
	if !bytes.Equal(result, expected) {
		t.Errorf("MD4('') = %x, expected %x", result, expected)
	}
}

func TestMD4HashMessage(t *testing.T) {
	// MD4("message digest") = d9130a8164549fe818874806e1c7014b
	result := md4Hash([]byte("message digest"))
	expected := []byte{0xd9, 0x13, 0x0a, 0x81, 0x64, 0x54, 0x9f, 0xe8, 0x18, 0x87, 0x48, 0x06, 0xe1, 0xc7, 0x01, 0x4b}
	if !bytes.Equal(result, expected) {
		t.Errorf("MD4('message digest') = %x, expected %x", result, expected)
	}
}

func TestWindowsTimestamp(t *testing.T) {
	// Just verify it returns a non-zero value
	ts := windowsTimestamp(time.Now())
	if ts == 0 {
		t.Error("windowsTimestamp returned 0")
	}
}

func TestBuildNTLMv2Blob(t *testing.T) {
	clientChallenge := make([]byte, 8)
	timestamp := windowsTimestamp(time.Now())
	targetInfo := []byte{0x01, 0x02, 0x03, 0x04}

	blob := buildNTLMv2Blob(clientChallenge, timestamp, targetInfo)

	// Verify blob starts with signature (0x00000101)
	sig := binary.LittleEndian.Uint32(blob[:4])
	if sig != 0x00000101 {
		t.Errorf("blob signature = 0x%08x, expected 0x00000101", sig)
	}

	// Verify reserved field is 0
	reserved := binary.LittleEndian.Uint32(blob[4:8])
	if reserved != 0 {
		t.Errorf("blob reserved = %d, expected 0", reserved)
	}
}

// Helper function to build a mock NTLM Type 2 (Challenge) message
func buildMockChallenge() []byte {
	buf := new(bytes.Buffer)

	// Signature
	buf.Write(ntlmSignature)

	// MessageType = 2
	binary.Write(buf, binary.LittleEndian, uint32(ntlmTypeChallenge))

	// Target name fields (offset 12)
	binary.Write(buf, binary.LittleEndian, uint16(0))  // Len
	binary.Write(buf, binary.LittleEndian, uint16(0))  // MaxLen
	binary.Write(buf, binary.LittleEndian, uint32(56)) // Offset

	// Negotiate flags (offset 20)
	flags := uint32(ntlmFlagNegotiateUnicode | ntlmFlagNegotiateNTLM | ntlmFlagNegotiateExtendedSecurity)
	binary.Write(buf, binary.LittleEndian, flags)

	// Server challenge (offset 24)
	buf.Write([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})

	// Reserved (8 bytes)
	buf.Write(make([]byte, 8))

	// Target info fields (offset 40)
	binary.Write(buf, binary.LittleEndian, uint16(0))  // Len
	binary.Write(buf, binary.LittleEndian, uint16(0))  // MaxLen
	binary.Write(buf, binary.LittleEndian, uint32(56)) // Offset

	// Version (8 bytes) - optional
	buf.Write(make([]byte, 8))

	return buf.Bytes()
}
