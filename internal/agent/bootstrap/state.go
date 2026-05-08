package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Phase identifies a step in the bootstrap state machine. PhaseDone
// is the terminal state — a re-run that loads a State with
// Phase==PhaseDone returns immediately.
type Phase string

const (
	PhaseDetect    Phase = "detect"
	PhaseConfigure Phase = "configure"
	PhaseValidate  Phase = "validate"
	PhaseInstall   Phase = "install"
	PhaseVerify    Phase = "verify"
	PhaseDone      Phase = "done"
)

// stateSchemaVersion is bumped when a backwards-incompatible field
// change lands in v1.x. Engine refuses to load a State with a
// higher version than it knows.
const stateSchemaVersion = 1

// State is the persisted bootstrap progress + accumulated artifacts.
// Re-runs read State, jump past the last-completed phase, and
// continue. JSON wire format pinned via tags so v1.x migrations
// are explicit.
type State struct {
	Version     int               `json:"version"`
	Phase       Phase             `json:"phase"`
	StartedAt   time.Time         `json:"started_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Detection   *DetectionResult  `json:"detection,omitempty"`
	Config      *Configuration    `json:"config,omitempty"`
	Validation  *ValidationResult `json:"validation,omitempty"`
	Install     *InstallResult    `json:"install,omitempty"`
	Verify      *VerifyResult     `json:"verify,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
}

// NewState returns a fresh State at PhaseDetect. now is injected
// for test determinism.
func NewState(now time.Time) *State {
	return &State{
		Version:   stateSchemaVersion,
		Phase:     PhaseDetect,
		StartedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	}
}

// LoadState reads the state file at path. Returns (nil, nil) when
// the file is absent — bootstrap starts fresh. A corrupt or
// unreadable file is a hard error.
func LoadState(path string) (*State, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled (StatePath); not a user input
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read state %q: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("bootstrap: parse state %q: %w", path, err)
	}
	if s.Version > stateSchemaVersion {
		return nil, fmt.Errorf("bootstrap: state %q has version %d (this binary supports up to %d) — upgrade kscore-agent or remove the state file",
			path, s.Version, stateSchemaVersion)
	}
	return &s, nil
}

// Save writes s to path atomically (temp file in the same directory
// + os.Rename). Defends against a mid-write crash leaving a corrupt
// state file. Caller updates s.UpdatedAt before calling.
func (s *State) Save(path string) error {
	if path == "" {
		return errors.New("bootstrap: state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("bootstrap: state dir: %w", err)
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("bootstrap: marshal state: %w", err)
	}
	tmp := path + ".tmp." + strconv.Itoa(os.Getpid()) + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("bootstrap: write temp %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("bootstrap: rename temp into place: %w", err)
	}
	return nil
}

// nextPhase returns the phase that follows current. Defined here
// as the single source of truth for Engine's resume logic.
func nextPhase(current Phase) Phase {
	switch current {
	case PhaseDetect:
		return PhaseConfigure
	case PhaseConfigure:
		return PhaseValidate
	case PhaseValidate:
		return PhaseInstall
	case PhaseInstall:
		return PhaseVerify
	case PhaseVerify:
		return PhaseDone
	default:
		return PhaseDone
	}
}
