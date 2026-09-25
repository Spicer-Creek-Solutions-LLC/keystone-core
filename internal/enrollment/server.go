package enrollment

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/natsauth"
	"go.keystone-core.io/keystone-core/internal/store"
)

// Server is the server's enrollment side, assembled from its configuration.
// Everything it reads is named by one configuration key; it reads no other file
// (D-C05-4, ROLE-1).
type Server struct {
	Issuer     *Issuer
	Keys       ServiceKeys
	Enrollment natsauth.ServiceCredential
	Presence   natsauth.ServiceCredential
	CAPEM      []byte
	URL        string
	ServerName string
}

// ErrMismatchedDeployment is returned when the configured credentials and
// signing seed do not belong to one account, which would mint identities the
// broker does not accept.
var ErrMismatchedDeployment = errors.New("enrollment: configured credentials and signing seed are not one deployment")

// Open reads and checks every enrollment input before the server serves: each
// credential against its slot, the signing seed against the account those
// credentials name, the broker trust anchor, and both service keys. Any failure
// refuses startup; nothing here is retried or defaulted.
func Open(cfg config.Server, st *store.Store) (*Server, error) {
	n := cfg.NATS
	enrollmentFile, err := os.ReadFile(n.EnrollmentCredentials)
	if err != nil {
		return nil, err
	}
	enrollmentCred, err := natsauth.ReadServiceCredential(natsauth.EnrollmentService, enrollmentFile)
	if err != nil {
		return nil, fmt.Errorf("nats.enrollment_credentials: %w", err)
	}
	presenceFile, err := os.ReadFile(n.PresenceCredentials)
	if err != nil {
		return nil, err
	}
	presenceCred, err := natsauth.ReadServiceCredential(natsauth.PresenceConsumer, presenceFile)
	if err != nil {
		return nil, fmt.Errorf("nats.presence_credentials: %w", err)
	}
	seed, err := os.ReadFile(n.AccountSigningSeed)
	if err != nil {
		return nil, err
	}
	minter, err := natsauth.NewMinter(enrollmentCred.Account, bytes.TrimSpace(seed))
	if err != nil {
		return nil, fmt.Errorf("nats.account_signing_seed: %w", err)
	}
	if presenceCred.Account != enrollmentCred.Account ||
		enrollmentCred.Issuer != minter.SigningKey() || presenceCred.Issuer != minter.SigningKey() {
		return nil, ErrMismatchedDeployment
	}
	ca, err := os.ReadFile(n.CAFile)
	if err != nil {
		return nil, err
	}
	if _, err := natsauth.ClientTLSConfig(ca, n.ServerName); err != nil {
		return nil, fmt.Errorf("nats.ca_file: %w", err)
	}
	keys, err := LoadServiceKeys(cfg.Service.SigningPrivateKeyFile, cfg.Service.ResultEncryptionPublicKeyFile)
	if err != nil {
		return nil, err
	}
	return &Server{
		Issuer:     NewIssuer(st, minter, keys),
		Keys:       keys,
		Enrollment: enrollmentCred,
		Presence:   presenceCred,
		CAPEM:      ca,
		URL:        n.URL,
		ServerName: n.ServerName,
	}, nil
}
