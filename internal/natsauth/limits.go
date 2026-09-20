package natsauth

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Limits renders ADR-0002 § 9. That section calls its values "starting points a
// deployment may tighten" and requires that **none is absent or infinite**, so
// every field here is explicit and GEN-5 asserts the property rather than the
// numbers.
//
// Account limits are applied to the account JWT here. Stream and consumer
// limits are VALUES ONLY: ADR-0002 § 7's streams are created by C06, and
// D-C03-4 puts applying them there. Rendering them here is what stops C06
// inventing a second set.
type Limits struct {
	Account  AccountLimits
	Command  StreamLimits
	Result   StreamLimits
	Consumer ConsumerLimits
}

// AccountLimits is the Keystone account's envelope. FleetSize is what the
// connection bound is derived from rather than a second number to keep in step.
type AccountLimits struct {
	FleetSize            int64
	MaxConnections       int64
	MaxSubscriptions     int64
	MaxPayloadBytes      int64
	MaxJetStreamDiskByte int64
}

// StreamLimits are ADR-0002 § 9's per-stream rows.
//
// MaxMessagesPerSubject is set on BOTH streams even though § 9 names it for the
// result stream only. One subject per agent makes a per-subject limit a
// per-agent limit natively, and that is as true of a command backlog as of a
// result backlog. The other reason is GEN-5: a field left at zero because the
// zero is benign is a field the check has to make an exception for, and an
// exception is where an absent limit hides.
type StreamLimits struct {
	MaxAgeSeconds         int64
	MaxMessages           int64
	MaxBytes              int64
	MaxMessagesPerSubject int64
}

// ConsumerLimits are ARCH-NATS-009's serialization and ARCH-NATS-010's finite
// redelivery, as values C06 applies.
type ConsumerLimits struct {
	MaxAckPending  int64
	MaxDeliver     int64
	BackOffSeconds []int64
	AckWaitSeconds int64
}

// DefaultLimits is one deployment's starting point. A caller may tighten any of
// them; none may be absent, and GEN-5 is the case that says so.
func DefaultLimits(fleetSize int64) Limits {
	if fleetSize < 1 {
		fleetSize = 1
	}
	return Limits{
		Account: AccountLimits{
			FleetSize: fleetSize,
			// One connection per agent, one per service role, and headroom for
			// a reconnect overlapping the connection it replaces.
			MaxConnections: fleetSize + int64(len(Services())) + 8,
			// An agent needs its command subtree, its inbox and a little room;
			// a service role needs its own subject set and its inbox.
			MaxSubscriptions: 32,
			// ARCH-EXEC-001 bounds captured output, so a larger payload is a
			// defect rather than a workload.
			MaxPayloadBytes:      1 << 20,
			MaxJetStreamDiskByte: 1 << 30,
		},
		Command: StreamLimits{
			MaxAgeSeconds:         6 * 3600,
			MaxMessages:           fleetSize * 1024,
			MaxBytes:              256 << 20,
			MaxMessagesPerSubject: 64,
		},
		Result: StreamLimits{
			MaxAgeSeconds: 24 * 3600,
			MaxMessages:   fleetSize * 4096,
			MaxBytes:      512 << 20,
			// This is what bounds one agent's contribution to shared storage.
			MaxMessagesPerSubject: 256,
		},
		Consumer: ConsumerLimits{
			MaxAckPending:  1,
			MaxDeliver:     5,
			BackOffSeconds: []int64{1, 5, 15, 60},
			AckWaitSeconds: 30,
		},
	}
}

// ErrLimits is returned for a limit set that is absent or infinite anywhere.
var ErrLimits = errors.New("every limit must be present and finite")

// Validate rejects a limit set ADR-0002 § 9 forbids: "the requirement is that
// none is absent or infinite". Zero is absent, and jwt.NoLimit is -1, so both
// are non-positive and one rule covers them.
//
// It walks by reflection rather than listing fields, for the reason GEN-5 does:
// a field added later is covered without this function being edited, and a
// listed field set is a second copy of the struct that goes stale. Any kind the
// walk does not understand is an error rather than a skip -- a limit the check
// cannot judge is a limit it is not checking.
//
// WHY THIS EXISTS AT ALL: Generate used to copy Config.Limits straight through,
// so a caller could hand it a zero value or jwt.NoLimit and receive a signed
// account with no bound on anything. GEN-5 could not see it, because the
// contract generates with the defaults and the reflective case therefore
// demonstrated the default instance rather than the configurable input.
//
// It does NOT enforce that a supplied set only tightens the defaults. ADR-0002
// § 9 calls the values "starting points a deployment may tighten" and states one
// requirement -- none absent, none infinite. Refusing a looser-but-finite value
// would be C03 deciding a limit, which § 3's "What C03 decides" puts outside it.
func (l Limits) Validate() error {
	var problems []string
	var walk func(path string, v reflect.Value)
	walk = func(path string, v reflect.Value) {
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(path+"."+v.Type().Field(i).Name, v.Field(i))
			}
		case reflect.Slice:
			if v.Len() == 0 {
				problems = append(problems, path+" is empty")
				return
			}
			for i := 0; i < v.Len(); i++ {
				walk(fmt.Sprintf("%s[%d]", path, i), v.Index(i))
			}
		case reflect.Int64, reflect.Int:
			if v.Int() <= 0 {
				problems = append(problems, fmt.Sprintf("%s is %d", path, v.Int()))
			}
		default:
			problems = append(problems, path+" has kind "+v.Kind().String()+", which this check cannot judge")
		}
	}
	walk("Limits", reflect.ValueOf(l))
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrLimits, strings.Join(problems, "; "))
	}
	return nil
}
