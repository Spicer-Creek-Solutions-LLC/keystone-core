// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/masterkey"
)

// EncryptCADirectory migrates the plaintext CA key files under dir to
// encrypted envelopes sealed with key, in place. Cert files are left
// untouched (public material). It returns the names of the CA pairs it
// migrated ("root", "signing").
//
// It is all-or-nothing on detection: if any present key file is already
// an encrypted envelope it returns an error before writing anything, and
// it errors when dir holds no CA key material — so a stray or repeated
// `ca encrypt` is a clear error, never silent corruption.
func EncryptCADirectory(dir string, key masterkey.Key) ([]string, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: directory is required", ErrInvalidCAStorage)
	}
	if key.IsZero() {
		return nil, fmt.Errorf("%w: master key is required", ErrInvalidCAStorage)
	}

	plain, err := NewFileCAStorage(dir)
	if err != nil {
		return nil, err
	}
	enc, err := NewEncryptedFileCAStorage(dir, key)
	if err != nil {
		return nil, err
	}

	type pair struct {
		name    string
		keyFile string
		load    func() (*x509.Certificate, crypto.Signer, error)
		save    func(*x509.Certificate, crypto.Signer) error
	}
	pairs := []pair{
		{"root", rootKeyFile, plain.LoadRootCA, enc.SaveRootCA},
		{"signing", signingKeyFile, plain.LoadSigningCA, enc.SaveSigningCA},
	}

	// Detection pre-pass: collect the present, plaintext pairs and bail
	// before any write if one is already encrypted.
	var todo []pair
	for _, p := range pairs {
		present, encrypted, err := classifyCAKeyFile(filepath.Join(dir, p.keyFile))
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		if encrypted {
			return nil, fmt.Errorf("%w: %s is already encrypted", ErrInvalidCAStorage, p.keyFile)
		}
		todo = append(todo, p)
	}
	if len(todo) == 0 {
		return nil, fmt.Errorf("%w: no plaintext CA key files under %s", ErrCAStorageNotFound, dir)
	}

	migrated := make([]string, 0, len(todo))
	for _, p := range todo {
		cert, signer, err := p.load()
		if err != nil {
			return migrated, err
		}
		if err := p.save(cert, signer); err != nil {
			return migrated, err
		}
		migrated = append(migrated, p.name)
	}
	return migrated, nil
}

// classifyCAKeyFile reports whether a key file exists and, if so, whether
// it is already an encrypted CA envelope (begins with the magic prefix).
func classifyCAKeyFile(path string) (present, encrypted bool, err error) {
	f, err := os.Open(path) // #nosec G304 -- operator-supplied CA storage dir
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("%w: open %s: %v", ErrInvalidCAStorage, filepath.Base(path), err)
	}
	defer func() { _ = f.Close() }()

	var magic [caEnvMagicLen]byte
	n, _ := io.ReadFull(f, magic[:])
	return true, n == caEnvMagicLen && magic == caKeyMagic, nil
}
