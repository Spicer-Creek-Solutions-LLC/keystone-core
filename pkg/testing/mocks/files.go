package mocks

import (
	"bytes"
	"context"
	"io"
	"time"
)

// FileSource is a simple mock for state.FileSource.
type FileSource struct {
	Data     []byte
	Checksum string
	Version  string
	Err      error
	Delay    time.Duration
}

func (m *FileSource) Get(ctx context.Context) (io.ReadCloser, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Delay > 0 {
		timer := time.NewTimer(m.Delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return io.NopCloser(bytes.NewReader(m.Data)), nil
}

func (m *FileSource) GetChecksum() string {
	return m.Checksum
}

func (m *FileSource) GetVersion() string {
	return m.Version
}
