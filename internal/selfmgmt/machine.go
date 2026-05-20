package selfmgmt

import "go.keystone-core.io/keystone-core/pkg/statemachine"

// newMachine builds the bootstrap FSM starting at initial, with cp
// wired in as the checkpointer (set the in-memory default upstream if
// caller wants one). [BootstrapManager.Run] adopts any persisted
// snapshot via [statemachine.Machine.RestoreFrom] before the loop, so
// a crashed bootstrap resumes at its last checkpointed state without
// re-replaying earlier phases.
//
// Topology — six phase-pairs plus failure/rollback (Epic 18 design §):
//
//	NotStarted        --start_detect-->     Detecting          --detect_done-->     Detected
//	Detecting                                                  --detect_fail-->     Failed
//	Detected          --start_configure-->  Configuring        --configure_done--> Configured
//	Configuring                                                --configure_fail--> Failed
//	Configured        --start_validate-->   Validating         --validate_done-->  Validated
//	Validating                                                 --validate_fail-->  Failed
//	Validated         --start_install-->    Installing         --install_done-->   Installed
//	Installing                                                 --install_fail-->   Failed
//	Installed         --start_blueprints--> ApplyingBlueprints --blueprints_done--> BlueprintsApplied
//	ApplyingBlueprints                                          --blueprints_fail--> Failed
//	BlueprintsApplied --start_verify-->     Verifying          --verify_done-->    Verified
//	Verifying                                                  --verify_fail-->    Failed
//	Failed            --rollback-->         RolledBack
//
// [StateVerified] and [StateRolledBack] are terminal. [StateFailed]
// is NOT terminal — the rollback edge keeps it usable until either
// the manager auto-rolls-back or the operator explicitly drives the
// transition.
func newMachine(initial BootstrapState, cp statemachine.Checkpointer[BootstrapState, BootstrapEvent]) (*statemachine.Machine[BootstrapState, BootstrapEvent], error) {
	return statemachine.NewBuilder[BootstrapState, BootstrapEvent]().
		Initial(initial).
		State(StateVerified, StateRolledBack).
		Transition(StateNotStarted, EventStartDetect, StateDetecting).
		Transition(StateDetecting, EventDetectDone, StateDetected).
		Transition(StateDetecting, EventDetectFail, StateFailed).
		Transition(StateDetected, EventStartConfigure, StateConfiguring).
		Transition(StateConfiguring, EventConfigureDone, StateConfigured).
		Transition(StateConfiguring, EventConfigureFail, StateFailed).
		Transition(StateConfigured, EventStartValidate, StateValidating).
		Transition(StateValidating, EventValidateDone, StateValidated).
		Transition(StateValidating, EventValidateFail, StateFailed).
		Transition(StateValidated, EventStartInstall, StateInstalling).
		Transition(StateInstalling, EventInstallDone, StateInstalled).
		Transition(StateInstalling, EventInstallFail, StateFailed).
		Transition(StateInstalled, EventStartBlueprints, StateApplyingBlueprints).
		Transition(StateApplyingBlueprints, EventBlueprintsDone, StateBlueprintsApplied).
		Transition(StateApplyingBlueprints, EventBlueprintsFail, StateFailed).
		Transition(StateBlueprintsApplied, EventStartVerify, StateVerifying).
		Transition(StateVerifying, EventVerifyDone, StateVerified).
		Transition(StateVerifying, EventVerifyFail, StateFailed).
		Transition(StateFailed, EventRollback, StateRolledBack).
		Checkpointer(cp).
		Build()
}
