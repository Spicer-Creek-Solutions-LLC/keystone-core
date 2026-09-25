//go:build contract

package enrollmentcontract

import "testing"

func TestKEY2PersistedKeysSurvivePreRequestCrash(t *testing.T)        { pending(t, "KEY-2") }
func TestLEDGER1LedgerExistsBeforeActivation(t *testing.T)            { pending(t, "LEDGER-1") }
func TestORD1PermanentConnectionPrecedesSpentWrite(t *testing.T)      { pending(t, "ORD-1") }
func TestS61SuccessRequiresConfirmation(t *testing.T)                 { pending(t, "S6-1") }
func TestCRASH1RecoverS0ToS1(t *testing.T)                            { pending(t, "CRASH-1") }
func TestCRASH2RecoverS1ToS2(t *testing.T)                            { pending(t, "CRASH-2") }
func TestCRASH3RecoverS2ToS3(t *testing.T)                            { pending(t, "CRASH-3") }
func TestCRASH4RecoverS3ToS4(t *testing.T)                            { pending(t, "CRASH-4") }
func TestCRASH5RecoverAgentS4ToS6(t *testing.T)                       { pending(t, "CRASH-5") }
func TestCRASH6RecoverServerS4ToS6(t *testing.T)                      { pending(t, "CRASH-6") }
func TestRECON1EnrolledAgentReconnectsWithoutEnrollment(t *testing.T) { pending(t, "RECON-1") }
func TestRECON2MissingIdentityFailsLocally(t *testing.T)              { pending(t, "RECON-2") }
