package telnet

import (
	"testing"
)

func TestNewNegotiator_Defaults(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	if n.termType != "vt100" {
		t.Errorf("expected default termType=vt100, got %s", n.termType)
	}
	if n.rows != 24 {
		t.Errorf("expected default rows=24, got %d", n.rows)
	}
	if n.cols != 80 {
		t.Errorf("expected default cols=80, got %d", n.cols)
	}
}

func TestNewNegotiator_CustomValues(t *testing.T) {
	n := NewNegotiator("xterm-256color", 40, 120)
	if n.termType != "xterm-256color" {
		t.Errorf("expected termType=xterm-256color, got %s", n.termType)
	}
	if n.rows != 40 {
		t.Errorf("expected rows=40, got %d", n.rows)
	}
	if n.cols != 120 {
		t.Errorf("expected cols=120, got %d", n.cols)
	}
}

func TestProcessData_PlainText(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	clean, responses := n.ProcessData([]byte("Hello, world!"))
	if string(clean) != "Hello, world!" {
		t.Errorf("expected clean text, got %q", string(clean))
	}
	if len(responses) != 0 {
		t.Errorf("expected no responses, got %d bytes", len(responses))
	}
}

func TestProcessData_DoubleIAC(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// IAC IAC = literal 0xFF
	data := []byte{'A', IAC, IAC, 'B'}
	clean, responses := n.ProcessData(data)
	if len(clean) != 3 || clean[0] != 'A' || clean[1] != IAC || clean[2] != 'B' {
		t.Errorf("expected A, 0xFF, B; got %v", clean)
	}
	if len(responses) != 0 {
		t.Errorf("expected no responses")
	}
}

func TestProcessData_WILLEcho(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// Server: IAC WILL ECHO
	data := []byte{IAC, WILL, OptEcho}
	clean, responses := n.ProcessData(data)
	if len(clean) != 0 {
		t.Errorf("expected no clean data, got %d bytes", len(clean))
	}
	// We should respond IAC DO ECHO
	expected := []byte{IAC, DO, OptEcho}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected %v, got %v", expected, responses)
	}
	// Option state should be Remote=true
	if !n.IsOptionRemote(OptEcho) {
		t.Error("expected Echo to be remote active")
	}
}

func TestProcessData_WILLSuppressGoAhead(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	data := []byte{IAC, WILL, OptSuppressGoAhead}
	_, responses := n.ProcessData(data)
	expected := []byte{IAC, DO, OptSuppressGoAhead}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected %v, got %v", expected, responses)
	}
	if !n.IsOptionRemote(OptSuppressGoAhead) {
		t.Error("expected SGA to be remote active")
	}
}

func TestProcessData_WILLUnknown(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// Unknown option should be refused
	data := []byte{IAC, WILL, 99}
	_, responses := n.ProcessData(data)
	expected := []byte{IAC, DONT, 99}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected DONT for unknown, got %v", responses)
	}
}

func TestProcessData_DOTermType(t *testing.T) {
	n := NewNegotiator("vt100", 0, 0)
	// Server: IAC DO TermType
	data := []byte{IAC, DO, OptTermType}
	_, responses := n.ProcessData(data)
	expected := []byte{IAC, WILL, OptTermType}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected WILL TermType, got %v", responses)
	}
	if !n.IsOptionActive(OptTermType) {
		t.Error("expected TermType to be locally active")
	}
}

func TestProcessData_DOWindowSize(t *testing.T) {
	n := NewNegotiator("", 24, 80)
	data := []byte{IAC, DO, OptWindowSize}
	_, responses := n.ProcessData(data)
	expected := []byte{IAC, WILL, OptWindowSize}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected WILL WindowSize, got %v", responses)
	}
	if !n.IsOptionActive(OptWindowSize) {
		t.Error("expected WindowSize to be locally active")
	}
}

func TestProcessData_DOSuppressGoAhead(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	data := []byte{IAC, DO, OptSuppressGoAhead}
	_, responses := n.ProcessData(data)
	expected := []byte{IAC, WILL, OptSuppressGoAhead}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected WILL SGA, got %v", responses)
	}
}

func TestProcessData_DOUnknown(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	data := []byte{IAC, DO, 99}
	_, responses := n.ProcessData(data)
	expected := []byte{IAC, WONT, 99}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected WONT for unknown, got %v", responses)
	}
}

func TestProcessData_WONT(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// First set remote active
	n.ProcessData([]byte{IAC, WILL, OptEcho})
	if !n.IsOptionRemote(OptEcho) {
		t.Fatal("precondition: expected Echo remote active")
	}
	// Now WONT
	_, responses := n.ProcessData([]byte{IAC, WONT, OptEcho})
	expected := []byte{IAC, DONT, OptEcho}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected DONT, got %v", responses)
	}
	if n.IsOptionRemote(OptEcho) {
		t.Error("expected Echo to no longer be remote active")
	}
}

func TestProcessData_DONT(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// First set local active
	n.ProcessData([]byte{IAC, DO, OptTermType})
	if !n.IsOptionActive(OptTermType) {
		t.Fatal("precondition: expected TermType active")
	}
	// Now DONT
	_, responses := n.ProcessData([]byte{IAC, DONT, OptTermType})
	expected := []byte{IAC, WONT, OptTermType}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected WONT, got %v", responses)
	}
	if n.IsOptionActive(OptTermType) {
		t.Error("expected TermType to no longer be active")
	}
}

func TestProcessData_SubNeg_TermType(t *testing.T) {
	n := NewNegotiator("xterm", 0, 0)
	// Enable TermType first
	n.ProcessData([]byte{IAC, DO, OptTermType})

	// Sub-negotiation: IAC SB TERMTYPE SEND IAC SE
	data := []byte{IAC, SB, OptTermType, SubSend, IAC, SE}
	_, responses := n.ProcessData(data)

	// Expected: IAC SB TERMTYPE IS "xterm" IAC SE
	expected := []byte{IAC, SB, OptTermType, SubIs}
	expected = append(expected, []byte("xterm")...)
	expected = append(expected, IAC, SE)

	if !bytesEqual(responses, expected) {
		t.Errorf("expected TermType subneg response %v, got %v", expected, responses)
	}
}

func TestProcessData_SubNeg_WindowSize(t *testing.T) {
	n := NewNegotiator("", 24, 80)
	// Enable NAWS
	n.ProcessData([]byte{IAC, DO, OptWindowSize})

	// Sub-negotiation: IAC SB NAWS SEND IAC SE
	data := []byte{IAC, SB, OptWindowSize, SubSend, IAC, SE}
	_, responses := n.ProcessData(data)

	// Expected: IAC SB NAWS <cols-hi> <cols-lo> <rows-hi> <rows-lo> IAC SE
	expected := []byte{IAC, SB, OptWindowSize, 0, 80, 0, 24, IAC, SE}

	if !bytesEqual(responses, expected) {
		t.Errorf("expected NAWS subneg response %v, got %v", expected, responses)
	}
}

func TestProcessData_MixedDataAndIAC(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// Mix of plain text and IAC sequences
	data := []byte{'H', 'e', 'l', 'l', 'o', IAC, WILL, OptEcho, ' ', 'w', 'o', 'r', 'l', 'd'}
	clean, responses := n.ProcessData(data)
	if string(clean) != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", string(clean))
	}
	expected := []byte{IAC, DO, OptEcho}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected DO ECHO response, got %v", responses)
	}
}

func TestProcessData_NOP(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	data := []byte{'A', IAC, NOP, 'B'}
	clean, responses := n.ProcessData(data)
	if string(clean) != "AB" {
		t.Errorf("expected 'AB', got %q", string(clean))
	}
	if len(responses) != 0 {
		t.Errorf("expected no responses for NOP")
	}
}

func TestProcessData_GA(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	data := []byte{'X', IAC, GA, 'Y'}
	clean, _ := n.ProcessData(data)
	if string(clean) != "XY" {
		t.Errorf("expected 'XY', got %q", string(clean))
	}
}

func TestProcessData_MultipleIACSequences(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// WILL ECHO + DO TERMTYPE + NOP
	data := []byte{
		IAC, WILL, OptEcho,
		IAC, DO, OptTermType,
		IAC, NOP,
	}
	_, responses := n.ProcessData(data)
	// Should get: DO ECHO + WILL TERMTYPE
	expected := []byte{IAC, DO, OptEcho, IAC, WILL, OptTermType}
	if !bytesEqual(responses, expected) {
		t.Errorf("expected %v, got %v", expected, responses)
	}
}

func TestIsOptionActive_NotSet(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	if n.IsOptionActive(OptEcho) {
		t.Error("expected Echo not active")
	}
}

func TestIsOptionRemote_NotSet(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	if n.IsOptionRemote(OptEcho) {
		t.Error("expected Echo not remote active")
	}
}

func TestProcessData_SubNeg_EscapedIAC(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	n.ProcessData([]byte{IAC, DO, OptTermType})

	// Sub-negotiation with IAC IAC inside (escaped 0xFF in data)
	data := []byte{IAC, SB, OptTermType, SubSend, IAC, IAC, IAC, SE}
	_, responses := n.ProcessData(data)

	// Should still produce valid TermType response
	if len(responses) == 0 {
		t.Error("expected subneg response")
	}
}

func TestProcessData_UnexpectedByteInSubNeg(t *testing.T) {
	n := NewNegotiator("", 0, 0)
	// IAC SB <opt> <data> IAC <unexpected-non-SE-non-IAC>
	data := []byte{IAC, SB, OptTermType, SubSend, IAC, NOP, 'A'}
	clean, _ := n.ProcessData(data)
	// After unexpected byte in subneg IAC, state returns to normal; 'A' is clean data
	if string(clean) != "A" {
		t.Errorf("expected 'A' after subneg abort, got %q", string(clean))
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
