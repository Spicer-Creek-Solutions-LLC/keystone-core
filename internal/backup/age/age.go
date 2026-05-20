// Package age implements the [internal/backup.Encrypter] and
// [internal/backup.Decrypter] seams using filippo.io/age — a
// streaming envelope cipher built on X25519 + ChaCha20-Poly1305.
//
// PROJECT-DETAILS §4.20 names age as the v1.0 default ("age-encrypted
// (master key from env or KMS)"). KMS / Vault key providers defer to
// v1.x under the ROADMAP entry "Backup encryption: AWS KMS + Vault
// key providers"; this package ships file-backed identity +
// recipients providers only.
//
// Usage (encrypt side, kscore-backup create):
//
//	recipients, _ := age.LoadRecipientsFile("/etc/kscore/backup.pub")
//	enc := &age.Encrypter{Recipients: recipients}
//	out, err := backup.NewEncryptingWriter(dest, enc)
//	defer out.Close()
//	manifest, err := mgr.CreateBackup(ctx, out)
//
// Usage (decrypt side, kscore-backup restore):
//
//	identities, _ := age.LoadIdentityFile("/var/lib/kscore/backup.key")
//	dec := &age.Decrypter{Identities: identities}
//	in, err := backup.NewDecryptingReader(src, dec)
//	... pipe to tar reader ...
package age

import (
	"errors"
	"fmt"
	"io"

	upstream "filippo.io/age"
)

// Encrypter holds the public recipients an artifact is encrypted to.
// At least one recipient is required; concrete loading helpers
// ([LoadRecipientsFile], [RecipientsFromIdentities]) populate the
// field for the typical CLI path.
type Encrypter struct {
	Recipients []upstream.Recipient
}

// Wrap satisfies internal/backup.Encrypter. The returned WriteCloser
// MUST be Closed by the caller — age writes a trailer at Close that
// the decoder requires.
func (e *Encrypter) Wrap(dst io.Writer) (io.WriteCloser, error) {
	if len(e.Recipients) == 0 {
		return nil, errors.New("age: Encrypter has no recipients")
	}
	wc, err := upstream.Encrypt(dst, e.Recipients...)
	if err != nil {
		return nil, fmt.Errorf("age: encrypt: %w", err)
	}
	return wc, nil
}

// Decrypter holds the private identities able to unseal an artifact.
// Multiple identities are tried in order; the first match wins.
type Decrypter struct {
	Identities []upstream.Identity
}

// Wrap satisfies internal/backup.Decrypter.
func (d *Decrypter) Wrap(src io.Reader) (io.Reader, error) {
	if len(d.Identities) == 0 {
		return nil, errors.New("age: Decrypter has no identities")
	}
	r, err := upstream.Decrypt(src, d.Identities...)
	if err != nil {
		return nil, fmt.Errorf("age: decrypt: %w", err)
	}
	return r, nil
}
