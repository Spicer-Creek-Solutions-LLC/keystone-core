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
