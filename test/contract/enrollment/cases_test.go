//go:build contract

package enrollmentcontract

import "testing"

func TestARG1BundleNeverAppearsInArgv(t *testing.T)                     { pending(t, "ARG-1") }
func TestFILE1RefusesReadableBundle(t *testing.T)                       { pending(t, "FILE-1") }
func TestSTDIN1EnrollsFromStandardInput(t *testing.T)                   { pending(t, "STDIN-1") }
func TestFILE2SuccessfulEnrollmentRemovesBundle(t *testing.T)           { pending(t, "FILE-2") }
func TestDENY1TokenFailuresAreIndistinguishableAndAudited(t *testing.T) { pending(t, "DENY-1") }
func TestAUTH1RequestIdentityAndSignatureMustMatch(t *testing.T)        { pending(t, "AUTH-1") }
func TestIDEM1DifferentKeysAreNotARetry(t *testing.T)                   { pending(t, "IDEM-1") }
func TestIDEM2SameKeysReturnSameIdentity(t *testing.T)                  { pending(t, "IDEM-2") }
func TestID1AgentIdentifierIsServerAssigned(t *testing.T)               { pending(t, "ID-1") }
func TestKEY1AgentPrivateKeysNeverLeaveAgent(t *testing.T)              { pending(t, "KEY-1") }
func TestKEY2PersistedKeysSurvivePreRequestCrash(t *testing.T)          { pending(t, "KEY-2") }
func TestCRED1IdentityArtifactIsAtomicAndPrivate(t *testing.T)          { pending(t, "CRED-1") }
func TestLEDGER1LedgerExistsBeforeActivation(t *testing.T)              { pending(t, "LEDGER-1") }
func TestORD1PermanentConnectionPrecedesSpentWrite(t *testing.T)        { pending(t, "ORD-1") }
func TestSPENT1SpentTokenOnlyAnswersConfirmation(t *testing.T)          { pending(t, "SPENT-1") }
func TestEXP1BrokerRefusesBootstrapAfterExpiry(t *testing.T)            { pending(t, "EXP-1") }
func TestOBS1EndOfBootstrapAccessIsObserved(t *testing.T)               { pending(t, "OBS-1") }
func TestS61SuccessRequiresConfirmation(t *testing.T)                   { pending(t, "S6-1") }
func TestCRASH1RecoverS0ToS1(t *testing.T)                              { pending(t, "CRASH-1") }
func TestCRASH2RecoverS1ToS2(t *testing.T)                              { pending(t, "CRASH-2") }
func TestCRASH3RecoverS2ToS3(t *testing.T)                              { pending(t, "CRASH-3") }
func TestCRASH4RecoverS3ToS4(t *testing.T)                              { pending(t, "CRASH-4") }
func TestCRASH5RecoverAgentS4ToS6(t *testing.T)                         { pending(t, "CRASH-5") }
func TestCRASH6RecoverServerS4ToS6(t *testing.T)                        { pending(t, "CRASH-6") }
func TestRECON1EnrolledAgentReconnectsWithoutEnrollment(t *testing.T)   { pending(t, "RECON-1") }
func TestRECON2MissingIdentityFailsLocally(t *testing.T)                { pending(t, "RECON-2") }
func TestNON1SecondEnrollmentDoesNotChangeFirst(t *testing.T)           { pending(t, "NON-1") }
func TestTLS1EveryProductionConnectionVerifiesBroker(t *testing.T)      { pending(t, "TLS-1") }
func TestISO1JourneyUsesOnlyBrokerBetweenServerAndAgents(t *testing.T)  { pending(t, "ISO-1") }
