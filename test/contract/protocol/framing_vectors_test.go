//go:build contract

package protocol_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type framingVectorFile struct {
	ProtocolVersion uint32          `json:"protocol_version"`
	Vectors         []framingVector `json:"vectors"`
}

type framingVector struct {
	Name        string   `json:"name"`
	FieldValues []string `json:"field_values_hex"`
	Framed      string   `json:"framed_hex"`
}

func TestFramingVectorsAreSelfConsistent(t *testing.T) {
	b, err := os.ReadFile("framing-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var file framingVectorFile
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	if file.ProtocolVersion != 1 || len(file.Vectors) == 0 {
		t.Fatalf("unexpected vector file metadata: %+v", file)
	}
	for _, vector := range file.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			if len(vector.FieldValues) != 9 {
				t.Fatalf("got %d fields, want 9", len(vector.FieldValues))
			}
			var want bytes.Buffer
			for i, raw := range vector.FieldValues {
				value, err := hex.DecodeString(raw)
				if err != nil {
					t.Fatalf("field %d: %v", i+1, err)
				}
				if uint64(len(value)) > uint64(^uint32(0)) {
					t.Fatalf("field %d exceeds uint32 length", i+1)
				}
				var prefix [4]byte
				binary.BigEndian.PutUint32(prefix[:], uint32(len(value)))
				want.Write(prefix[:])
				want.Write(value)
			}
			got, err := hex.DecodeString(vector.Framed)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want.Bytes()) {
				t.Fatalf("framed bytes = %x, want %x", got, want.Bytes())
			}
		})
	}
}

func TestFramingEnvelopeBoundaries(t *testing.T) {
	const envelopeLimit = 1 << 20
	const prefixBytes = 9 * 4
	const fixedValueBytes = 1 + 1 + 0 + 0 + 1 + 1 + 1 + 32
	maxField := (1 << 20) - prefixBytes - fixedValueBytes
	if maxField <= 0 {
		t.Fatal("invalid envelope boundary arithmetic")
	}
	base := [][]byte{
		{0x01}, {0x01}, nil, nil, {0x01}, {0x01}, {0x01}, nil,
		make([]byte, 32),
	}
	for _, tc := range []struct {
		name      string
		fieldSize int
		want      int
	}{
		{name: "field-at-envelope-limit", fieldSize: maxField, want: 1 << 20},
		{name: "field-one-byte-over-envelope-limit", fieldSize: maxField + 1, want: (1 << 20) + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := append([][]byte(nil), base...)
			fields[7] = make([]byte, tc.fieldSize)
			got := len(frameFields(fields))
			if got != tc.want {
				t.Fatalf("framed envelope length = %d, want %d", got, tc.want)
			}
		})
	}
}

func frameFields(fields [][]byte) []byte {
	var framed bytes.Buffer
	for _, field := range fields {
		var prefix [4]byte
		binary.BigEndian.PutUint32(prefix[:], uint32(len(field)))
		framed.Write(prefix[:])
		framed.Write(field)
	}
	return framed.Bytes()
}
