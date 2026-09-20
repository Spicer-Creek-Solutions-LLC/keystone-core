//go:build contract

package authorizationcontract

import "testing"

func TestGEN1TwoAccountsAndNoSystemPrincipals(t *testing.T) { pending(t, "GEN-1") }
func TestGEN5AccountLimitsAreFinite(t *testing.T)           { pending(t, "GEN-5") }
func TestGEN7OperatorSeedIsOutsideDeploymentProcesses(t *testing.T) {
	pending(t, "GEN-7")
}
