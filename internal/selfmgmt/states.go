// SPDX-License-Identifier: Apache-2.0

package selfmgmt

// BootstrapState is the kscore-bootstrap lifecycle state. The state
// machine in machine.go drives a [BootstrapManager] through the six
// phases named in Epic 18 (detect → configure → validate → install
// → blueprints → verify), with [StateFailed] as the catch-all for
// any phase-level error and [StateRolledBack] as the post-failure
// terminal. Both [StateVerified] and [StateRolledBack] are terminal
// states ([BootstrapState.IsTerminal] returns true).
type BootstrapState string

const (
	StateNotStarted         BootstrapState = "not_started"
	StateDetecting          BootstrapState = "detecting"
	StateDetected           BootstrapState = "detected"
	StateConfiguring        BootstrapState = "configuring"
	StateConfigured         BootstrapState = "configured"
	StateValidating         BootstrapState = "validating"
	StateValidated          BootstrapState = "validated"
	StateInstalling         BootstrapState = "installing"
	StateInstalled          BootstrapState = "installed"
	StateApplyingBlueprints BootstrapState = "applying_blueprints"
	StateBlueprintsApplied  BootstrapState = "blueprints_applied"
	StateVerifying          BootstrapState = "verifying"
	StateVerified           BootstrapState = "verified"
	StateFailed             BootstrapState = "failed"
	StateRolledBack         BootstrapState = "rolled_back"
)

// IsTerminal reports whether s has no outgoing edges in the
// state machine — only [StateVerified] (happy-path completion) and
// [StateRolledBack] (post-failure cleanup completion). [StateFailed]
// is NOT terminal because the rollback transition is still available
// from it.
func (s BootstrapState) IsTerminal() bool {
	switch s {
	case StateVerified, StateRolledBack:
		return true
	default:
		return false
	}
}

// BootstrapEvent drives the [BootstrapManager] state machine. Each
// phase has a `start_X` / `X_done` / `X_fail` triple; [EventRollback]
// is the manual or auto-rollback transition out of [StateFailed].
type BootstrapEvent string

const (
	EventStartDetect     BootstrapEvent = "start_detect"
	EventDetectDone      BootstrapEvent = "detect_done"
	EventDetectFail      BootstrapEvent = "detect_fail"
	EventStartConfigure  BootstrapEvent = "start_configure"
	EventConfigureDone   BootstrapEvent = "configure_done"
	EventConfigureFail   BootstrapEvent = "configure_fail"
	EventStartValidate   BootstrapEvent = "start_validate"
	EventValidateDone    BootstrapEvent = "validate_done"
	EventValidateFail    BootstrapEvent = "validate_fail"
	EventStartInstall    BootstrapEvent = "start_install"
	EventInstallDone     BootstrapEvent = "install_done"
	EventInstallFail     BootstrapEvent = "install_fail"
	EventStartBlueprints BootstrapEvent = "start_blueprints"
	EventBlueprintsDone  BootstrapEvent = "blueprints_done"
	EventBlueprintsFail  BootstrapEvent = "blueprints_fail"
	EventStartVerify     BootstrapEvent = "start_verify"
	EventVerifyDone      BootstrapEvent = "verify_done"
	EventVerifyFail      BootstrapEvent = "verify_fail"
	EventRollback        BootstrapEvent = "rollback"
)
