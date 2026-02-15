package diode

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestWire_HeaderRoundtrip(t *testing.T) {
	h := &HeaderPacket{
		SessionID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		TotalChunks:  42,
		TotalSize:    12345,
		Checksum:     [32]byte{0xaa, 0xbb},
		FECEnabled:   true,
		FECGroupSize: 5,
		Filename:     "test-file.tar.gz",
		Timestamp:    time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data := marshalHeader(h)
	pt, err := parsePacketType(data)
	if err != nil {
		t.Fatalf("parsePacketType: %v", err)
	}
	if pt != PacketHeader {
		t.Fatalf("type = %d, want %d", pt, PacketHeader)
	}

	got, err := unmarshalHeader(data)
	if err != nil {
		t.Fatalf("unmarshalHeader: %v", err)
	}
	if got.SessionID != h.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if got.TotalChunks != h.TotalChunks {
		t.Errorf("TotalChunks = %d, want %d", got.TotalChunks, h.TotalChunks)
	}
	if got.TotalSize != h.TotalSize {
		t.Errorf("TotalSize = %d, want %d", got.TotalSize, h.TotalSize)
	}
	if got.Checksum != h.Checksum {
		t.Errorf("Checksum mismatch")
	}
	if got.FECEnabled != h.FECEnabled {
		t.Errorf("FECEnabled = %v, want %v", got.FECEnabled, h.FECEnabled)
	}
	if got.FECGroupSize != h.FECGroupSize {
		t.Errorf("FECGroupSize = %d, want %d", got.FECGroupSize, h.FECGroupSize)
	}
	if got.Filename != h.Filename {
		t.Errorf("Filename = %q, want %q", got.Filename, h.Filename)
	}
	if !got.Timestamp.Equal(h.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, h.Timestamp)
	}
}

func TestWire_DataRoundtrip(t *testing.T) {
	d := &DataPacket{
		SessionID: [16]byte{1},
		Sequence:  7,
		FECGroup:  2,
		Payload:   []byte("hello world"),
	}

	data := marshalData(d)
	pt, err := parsePacketType(data)
	if err != nil {
		t.Fatal(err)
	}
	if pt != PacketData {
		t.Fatalf("type = %d, want %d", pt, PacketData)
	}

	got, err := unmarshalData(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != d.SessionID {
		t.Error("SessionID mismatch")
	}
	if got.Sequence != d.Sequence {
		t.Errorf("Sequence = %d, want %d", got.Sequence, d.Sequence)
	}
	if got.FECGroup != d.FECGroup {
		t.Errorf("FECGroup = %d, want %d", got.FECGroup, d.FECGroup)
	}
	if !bytes.Equal(got.Payload, d.Payload) {
		t.Errorf("Payload mismatch")
	}
}

func TestWire_ParityRoundtrip(t *testing.T) {
	p := &ParityPacket{
		SessionID: [16]byte{2},
		FECGroup:  3,
		Parity:    []byte{0xff, 0xee, 0xdd},
	}

	data := marshalParity(p)
	pt, err := parsePacketType(data)
	if err != nil {
		t.Fatal(err)
	}
	if pt != PacketParity {
		t.Fatalf("type = %d, want %d", pt, PacketParity)
	}

	got, err := unmarshalParity(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != p.SessionID {
		t.Error("SessionID mismatch")
	}
	if got.FECGroup != p.FECGroup {
		t.Errorf("FECGroup = %d, want %d", got.FECGroup, p.FECGroup)
	}
	if !bytes.Equal(got.Parity, p.Parity) {
		t.Errorf("Parity mismatch")
	}
}

func TestWire_EndRoundtrip(t *testing.T) {
	e := &EndPacket{
		SessionID:   [16]byte{3},
		TotalChunks: 100,
		Checksum:    [32]byte{0x11, 0x22},
	}

	data := marshalEnd(e)
	pt, err := parsePacketType(data)
	if err != nil {
		t.Fatal(err)
	}
	if pt != PacketEnd {
		t.Fatalf("type = %d, want %d", pt, PacketEnd)
	}

	got, err := unmarshalEnd(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != e.SessionID {
		t.Error("SessionID mismatch")
	}
	if got.TotalChunks != e.TotalChunks {
		t.Errorf("TotalChunks = %d, want %d", got.TotalChunks, e.TotalChunks)
	}
	if got.Checksum != e.Checksum {
		t.Errorf("Checksum mismatch")
	}
}

func TestParsePacketType_Empty(t *testing.T) {
	_, err := parsePacketType(nil)
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestParsePacketType_Unknown(t *testing.T) {
	_, err := parsePacketType([]byte{0xff})
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestFEC_EncodeDecode_AllPresent(t *testing.T) {
	enc := NewFECEncoder(3)
	packets := [][]byte{
		{0x01, 0x02, 0x03},
		{0x04, 0x05, 0x06},
		{0x07, 0x08, 0x09},
	}

	var parity []byte
	for _, p := range packets {
		if par := enc.AddPacket(p); par != nil {
			parity = par
		}
	}
	if parity == nil {
		t.Fatal("expected parity after full group")
	}

	// Verify parity = XOR of all packets
	expected := make([]byte, 3)
	for _, p := range packets {
		for i := range p {
			expected[i] ^= p[i]
		}
	}
	if !bytes.Equal(parity, expected) {
		t.Errorf("parity = %x, want %x", parity, expected)
	}

	// Decode with all present — no recovery needed
	dec := NewFECDecoder(3)
	for i, p := range packets {
		dec.AddDataPacket(0, uint32(i), p)
	}
	dec.AddParityPacket(0, parity)

	recovered, err := dec.Recover(0, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 0 {
		t.Errorf("expected no recovered packets, got %d", len(recovered))
	}
}

func TestFEC_RecoverOneMissing(t *testing.T) {
	enc := NewFECEncoder(3)
	packets := [][]byte{
		{0x01, 0x02, 0x03},
		{0x04, 0x05, 0x06},
		{0x07, 0x08, 0x09},
	}

	var parity []byte
	for _, p := range packets {
		if par := enc.AddPacket(p); par != nil {
			parity = par
		}
	}

	// Drop packet 1 (index 1)
	dec := NewFECDecoder(3)
	dec.AddDataPacket(0, 0, packets[0])
	// skip packet 1
	dec.AddDataPacket(0, 2, packets[2])
	dec.AddParityPacket(0, parity)

	recovered, err := dec.Recover(0, []uint32{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 {
		t.Fatalf("expected 1 recovered, got %d", len(recovered))
	}
	if !bytes.Equal(recovered[1], packets[1]) {
		t.Errorf("recovered = %x, want %x", recovered[1], packets[1])
	}
}

func TestFEC_TwoMissingFails(t *testing.T) {
	enc := NewFECEncoder(3)
	packets := [][]byte{
		{0x01, 0x02, 0x03},
		{0x04, 0x05, 0x06},
		{0x07, 0x08, 0x09},
	}

	var parity []byte
	for _, p := range packets {
		if par := enc.AddPacket(p); par != nil {
			parity = par
		}
	}

	dec := NewFECDecoder(3)
	dec.AddDataPacket(0, 0, packets[0])
	dec.AddParityPacket(0, parity)

	_, err := dec.Recover(0, []uint32{0, 1, 2})
	if err == nil {
		t.Error("expected error for 2 missing packets")
	}
}

func TestFEC_Flush(t *testing.T) {
	enc := NewFECEncoder(5)
	// Only add 3 of 5 then flush
	enc.AddPacket([]byte{0x01})
	enc.AddPacket([]byte{0x02})
	parity := enc.Flush()
	if parity == nil {
		t.Fatal("expected parity from Flush")
	}
	if parity[0] != 0x03 { // 0x01 ^ 0x02
		t.Errorf("parity = %x, want 0x03", parity[0])
	}
}

func TestFEC_FlushEmpty(t *testing.T) {
	enc := NewFECEncoder(5)
	if parity := enc.Flush(); parity != nil {
		t.Error("expected nil from empty Flush")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{"valid", Config{Address: ":9000"}, false},
		{"missing address", Config{}, true},
		{"negative packet size", Config{Address: ":9000", PacketSize: -1}, true},
		{"negative fec group", Config{Address: ":9000", FECGroupSize: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	c := &Config{Address: ":9000"}
	c.Defaults()
	if c.PacketSize != DefaultPacketSize {
		t.Errorf("PacketSize = %d, want %d", c.PacketSize, DefaultPacketSize)
	}
	if c.FECGroupSize != DefaultFECGroupSize {
		t.Errorf("FECGroupSize = %d, want %d", c.FECGroupSize, DefaultFECGroupSize)
	}
	if c.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", c.Timeout)
	}
}

func TestSenderReceiver_Loopback(t *testing.T) {
	// Bind receiver to ephemeral port
	recv, err := NewReceiver(Config{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	defer recv.Close()

	recvAddr := recv.conn.LocalAddr().String()

	sender, err := NewSender(Config{Address: recvAddr})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer sender.Close()

	testData := []byte("Hello, data diode!")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Receive in background
	type result struct {
		filename string
		data     []byte
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		fn, data, err := recv.Receive(ctx)
		ch <- result{fn, data, err}
	}()

	time.Sleep(50 * time.Millisecond) // let receiver start

	if err := sender.Send(ctx, testData, "test.bin"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Receive: %v", r.err)
		}
		if r.filename != "test.bin" {
			t.Errorf("filename = %q, want %q", r.filename, "test.bin")
		}
		if !bytes.Equal(r.data, testData) {
			t.Errorf("data = %q, want %q", r.data, testData)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for receive")
	}

	if sender.State() != StateComplete {
		t.Errorf("sender state = %s, want %s", sender.State(), StateComplete)
	}
}

func TestSenderReceiver_FEC_Loopback(t *testing.T) {
	recv, err := NewReceiver(Config{Address: "127.0.0.1:0", FECEnabled: true, FECGroupSize: 3})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	defer recv.Close()

	recvAddr := recv.conn.LocalAddr().String()

	sender, err := NewSender(Config{Address: recvAddr, FECEnabled: true, FECGroupSize: 3, PacketSize: 50})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer sender.Close()

	// Send enough data to span multiple FEC groups
	testData := bytes.Repeat([]byte("ABCDEFGHIJ"), 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		_, data, err := recv.Receive(ctx)
		ch <- result{data, err}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := sender.Send(ctx, testData, "fec-test.bin"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Receive: %v", r.err)
		}
		if !bytes.Equal(r.data, testData) {
			t.Errorf("data length %d, want %d", len(r.data), len(testData))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for receive")
	}
}

func TestSender_ContextCancelled(t *testing.T) {
	recv, err := NewReceiver(Config{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	defer recv.Close()

	sender, err := NewSender(Config{
		Address:   recv.conn.LocalAddr().String(),
		RateLimit: 10, // very slow
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer sender.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = sender.Send(ctx, bytes.Repeat([]byte("x"), 10000), "large.bin")
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestStateMachine_Transitions(t *testing.T) {
	m := buildTransferMachine("test")
	if m.State() != StateQueued {
		t.Fatalf("initial state = %s, want %s", m.State(), StateQueued)
	}
	if err := m.Fire(EventBeginSend); err != nil {
		t.Fatal(err)
	}
	if m.State() != StateSending {
		t.Fatalf("state = %s, want %s", m.State(), StateSending)
	}
	if err := m.Fire(EventSendComplete); err != nil {
		t.Fatal(err)
	}
	if m.State() != StateComplete {
		t.Fatalf("state = %s, want %s", m.State(), StateComplete)
	}
}

func TestStateMachine_FailPath(t *testing.T) {
	m := buildTransferMachine("test")
	_ = m.Fire(EventBeginSend)
	if err := m.Fire(EventSendFail); err != nil {
		t.Fatal(err)
	}
	if m.State() != StateFailed {
		t.Fatalf("state = %s, want %s", m.State(), StateFailed)
	}
}

func TestStateMachine_InvalidTransition(t *testing.T) {
	m := buildTransferMachine("test")
	err := m.Fire(EventSendComplete) // can't complete from queued
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}
