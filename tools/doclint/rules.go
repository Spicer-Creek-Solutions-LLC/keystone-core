package main

// A Rule asserts that a conclusion this project reached has not been
// contradicted since.
//
// Every rule carries a lifetime. A sweep guards a correction, and each task
// adds one -- without retirement the set grows monotonically until its failures
// stop being read. RetireAfter names the task after which the corrected text has
// been stable long enough that the wrong belief is no longer plausible.
type Rule struct {
	ID    string
	Added string // the task that found the defect this guards

	// RetireAfter is the task after which this rule should be removed. Empty
	// means the rule is permanent -- allowed, but it must say why in Why.
	RetireAfter string

	// Why the rule exists, in one sentence: the belief that was wrong.
	Why string

	// Pattern matches the SUBJECT, not the old phrasing. DL-8's third tier: a
	// conclusion has no canonical wording, so searching for what it used to say
	// is enumerating instances one level up.
	Pattern string

	// Stale matches a hit that still carries the superseded conclusion. A hit
	// matching Pattern and not Stale is fine; the rule fails on hits matching
	// both, unless a Disposition exempts them.
	Stale string

	// Exempt names files whose hits are records rather than claims, each with a
	// reason. Never a pattern: an exemption that matches by shape exempts the
	// next thing by accident.
	Exempt map[string]string

	// MinHits guards against vacuity. A rule whose subject has vanished from
	// the tree is a rule that cannot fail, which is DL-1 wearing a sweep's
	// clothes.
	MinHits int
}

// rules is the whole set. Adding one is cheap; removing one when it retires is
// the part that needs forcing, which is what the lifetime check does.
var rules = []Rule{
	{
		ID:      "evidence-location",
		Added:   "G21",
		Why:     "docs/dossiers/README.md said acceptance evidence sits alongside the dossier; it lands with the work, and no pair has ever shipped together",
		Pattern: `acceptance[- ]evidence|<TASK>-acceptance-evidence`,
		Stale:   `alongside the dossier`,
		MinHits: 3,
		// Permanent: the wrong belief is the intuitive one, and every new
		// dossier is an opportunity to restate it.
		RetireAfter: "",
	},
	{
		ID:      "what-p10-is",
		Added:   "G22",
		Why:     "six documents called P10 a deployment or operations task, and three residual risks were gated on that belief",
		Pattern: `\bP10\b`,
		Stale:   `P10 — deployment and operations|deployment mode P10 would own|operational procedure is P10's|first\s+task that could demonstrate`,
		Exempt: map[string]string{
			"docs/dossiers/P08-acceptance-evidence.md": "a record of what was demonstrable at P08, superseded by its own terms",
			"docs/dossiers/P03-acceptance-evidence.md": "same",
		},
		MinHits: 10,
		// Retires with p10-risk-gates, at C14.
		//
		// This first said P11, on the reasoning that nobody describes P10 once
		// Stage P closes. The lifetime check fired the moment P11 was ticked --
		// in the same commit that added the rule -- which is the mechanism
		// working, and it forced the question the guess had skipped: a sweep
		// retires when the corrected text has been STABLE across enough tasks
		// that the belief is no longer available, and one task later is not
		// that. The demonstrate half of this rule shares a subject with
		// p10-risk-gates, so they retire together at the task that actually
		// attempts the reconstruction.
		RetireAfter: "C14",
	},
	{
		ID:      "agent-ledger-route",
		Added:   "P08",
		Why:     "RSK-4's control stopped routing through the server, and THR-46 went on saying it did",
		Pattern: `agent[- ]ledger|RSK-4`,
		Stale:   `reach\w*\s+(?:it|them|those ledgers?|the ledgers?)\s+only through the server`,
		MinHits: 3,
		// Retires after C14, the task that attempts the reconstruction: by then
		// the claim is tested rather than asserted.
		RetireAfter: "C14",
	},
	{
		ID:      "rsk8-forward-reference",
		Added:   "P09",
		Why:     "RSK-8 spoke of P09 as a decision not yet made, after ADR-0009 made it",
		Pattern: `\bRSK-8\b`,
		Stale:   `P09 will define|authorization model will share`,
		Exempt: map[string]string{
			"docs/dossiers/P01-acceptance-evidence.md": "a record of P01, superseded by its own terms",
		},
		MinHits:     3,
		RetireAfter: "C04",
	},
	{
		ID:      "p10-risk-gates",
		Added:   "P10",
		Why:     "ADR-0008 sent three risks to P10 expecting a demonstration P10 cannot make; the gates moved to C13 and C14",
		Pattern: `RSK-(?:1|4|9|12|13|14)\b`,
		Stale:   `\bor P10\b`,
		Exempt: map[string]string{
			"docs/adr/0003-enrollment-and-identity.md": "an earlier ADR's own disposition; THREAT-MODEL.md section 9 is authoritative",
			"docs/adr/0008-persistence-and-audit.md":   "same",
		},
		MinHits:     6,
		RetireAfter: "C14",
	},
	{
		ID:      "forbidden-ci-triggers",
		Added:   "G23",
		Why:     "a self-hosted runner on a public repository must never execute code from an arbitrary contributor's event",
		Pattern: `^\s*(?:pull_request_target|issue_comment)\s*:`,
		Stale:   `^\s*(?:pull_request_target|issue_comment)\s*:`,
		MinHits: 0, // zero hits is the correct state, so vacuity is the goal here
		// Permanent: the triggers stay convenient and stay wrong.
		RetireAfter: "",
	},
	{
		ID:          "no-pipe-to-shell",
		Added:       "G23",
		Why:         "the runner host's Docker socket is root on it, so an unverified root-level download is host compromise",
		Pattern:     `curl[^\n|]*\|\s*(?:sudo\s+)?(?:ba)?sh\b|wget[^\n|]*\|\s*(?:sudo\s+)?(?:ba)?sh\b`,
		Stale:       `curl[^\n|]*\|\s*(?:sudo\s+)?(?:ba)?sh\b|wget[^\n|]*\|\s*(?:sudo\s+)?(?:ba)?sh\b`,
		MinHits:     0,
		RetireAfter: "",
	},
}
