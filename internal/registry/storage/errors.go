package storage

import "errors"

// Sentinel errors for storage operations.
var (
	ErrModuleNotFound  = errors.New("module not found")
	ErrVersionNotFound = errors.New("version not found")
	ErrVersionExists   = errors.New("version already exists")
	ErrHashMismatch    = errors.New("hash mismatch")
)
