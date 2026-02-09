package telnet

import "encoding/binary"

// Telnet protocol bytes per RFC 854/855.
const (
	IAC  byte = 255 // Interpret As Command
	DONT byte = 254
	DO   byte = 253
	WONT byte = 252
	WILL byte = 251
	SB   byte = 250 // Sub-negotiation Begin
	SE   byte = 240 // Sub-negotiation End
	NOP  byte = 241
	GA   byte = 249 // Go Ahead
)

// Telnet options.
const (
	OptEcho            byte = 1
	OptSuppressGoAhead byte = 3
	OptTermType        byte = 24
	OptWindowSize      byte = 31 // NAWS (Negotiate About Window Size)
	OptLineMode        byte = 34
)

// Sub-negotiation constants.
const (
	SubIs   byte = 0 // IS (used in responses)
	SubSend byte = 1 // SEND (used in requests)
)

// negotiation state machine states.
const (
	stateNormal  = iota
	stateIACSeen // Saw IAC, waiting for command
	stateOption  // Saw command (WILL/WONT/DO/DONT), waiting for option
	stateSubNeg  // Inside sub-negotiation (after SB)
)

// OptionState tracks the negotiated state of a Telnet option.
type OptionState struct {
	Local  bool // We are WILLing
	Remote bool // They are DOing
}

// Negotiator handles Telnet IAC command negotiation.
type Negotiator struct {
	options  map[byte]*OptionState
	termType string
	rows     uint16
	cols     uint16
}

// NewNegotiator creates a new Telnet option negotiator.
func NewNegotiator(termType string, rows, cols uint16) *Negotiator {
	if termType == "" {
		termType = "vt100"
	}
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}
	return &Negotiator{
		options:  make(map[byte]*OptionState),
		termType: termType,
		rows:     rows,
		cols:     cols,
	}
}

// ProcessData strips IAC sequences from raw data, returning clean text
// and any response bytes that must be sent back to the server.
func (n *Negotiator) ProcessData(raw []byte) (clean, responses []byte) {
	state := stateNormal
	var cmd byte
	var subOpt byte
	var subData []byte
	var subIAC bool // Saw IAC inside sub-negotiation

	for _, b := range raw {
		switch state {
		case stateNormal:
			if b == IAC {
				state = stateIACSeen
			} else {
				clean = append(clean, b)
			}

		case stateIACSeen:
			switch b {
			case IAC:
				// Escaped IAC = literal 0xFF
				clean = append(clean, IAC)
				state = stateNormal
			case WILL, WONT, DO, DONT:
				cmd = b
				state = stateOption
			case SB:
				subData = nil
				subOpt = 0
				subIAC = false
				state = stateSubNeg
			case NOP, GA:
				state = stateNormal
			default:
				state = stateNormal
			}

		case stateOption:
			resp := n.handleCommand(cmd, b)
			responses = append(responses, resp...)
			state = stateNormal

		case stateSubNeg:
			switch {
			case subIAC:
				subIAC = false
				switch b {
				case SE:
					resp := n.handleSubNegotiation(subOpt, subData)
					responses = append(responses, resp...)
					state = stateNormal
				case IAC:
					subData = append(subData, IAC)
				default:
					// Unexpected byte after IAC in subneg
					state = stateNormal
				}
			case b == IAC:
				subIAC = true
			default:
				if subOpt == 0 && len(subData) == 0 {
					subOpt = b
				} else {
					subData = append(subData, b)
				}
			}
		}
	}

	return clean, responses
}

// handleCommand processes a single IAC WILL/WONT/DO/DONT command.
func (n *Negotiator) handleCommand(cmd, opt byte) []byte {
	state := n.getOption(opt)

	switch cmd {
	case WILL:
		// Server offers to enable option.
		if opt == OptEcho || opt == OptSuppressGoAhead {
			state.Remote = true
			return []byte{IAC, DO, opt}
		}
		return []byte{IAC, DONT, opt}

	case DO:
		// Server requests we enable option.
		if opt == OptTermType || opt == OptWindowSize || opt == OptSuppressGoAhead {
			state.Local = true
			return []byte{IAC, WILL, opt}
		}
		return []byte{IAC, WONT, opt}

	case WONT:
		state.Remote = false
		return []byte{IAC, DONT, opt}

	case DONT:
		state.Local = false
		return []byte{IAC, WONT, opt}
	}

	return nil
}

// handleSubNegotiation processes a sub-negotiation sequence.
func (n *Negotiator) handleSubNegotiation(opt byte, data []byte) []byte {
	switch opt {
	case OptTermType:
		if len(data) > 0 && data[0] == SubSend {
			return n.buildSubNeg(OptTermType, append([]byte{SubIs}, []byte(n.termType)...))
		}

	case OptWindowSize:
		if len(data) > 0 && data[0] == SubSend {
			buf := make([]byte, 4)
			binary.BigEndian.PutUint16(buf[0:2], n.cols)
			binary.BigEndian.PutUint16(buf[2:4], n.rows)
			return n.buildSubNeg(OptWindowSize, buf)
		}
	}

	return nil
}

// buildSubNeg builds a sub-negotiation response: IAC SB opt data IAC SE.
func (n *Negotiator) buildSubNeg(opt byte, data []byte) []byte {
	resp := make([]byte, 0, 4+len(data))
	resp = append(resp, IAC, SB, opt)
	resp = append(resp, data...)
	resp = append(resp, IAC, SE)
	return resp
}

// getOption returns the OptionState for an option, creating it if needed.
func (n *Negotiator) getOption(opt byte) *OptionState {
	if s, ok := n.options[opt]; ok {
		return s
	}
	s := &OptionState{}
	n.options[opt] = s
	return s
}

// IsOptionActive returns true if the given option is active locally.
func (n *Negotiator) IsOptionActive(opt byte) bool {
	if s, ok := n.options[opt]; ok {
		return s.Local
	}
	return false
}

// IsOptionRemote returns true if the given option is active on the remote side.
func (n *Negotiator) IsOptionRemote(opt byte) bool {
	if s, ok := n.options[opt]; ok {
		return s.Remote
	}
	return false
}
