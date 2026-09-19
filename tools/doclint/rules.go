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
		ID:    "runner-label-absence",
		Added: "G25",
		Why:   "queued `runs-on: docker` jobs were read as proof that no runner advertises that label; what serves a label is read from the runner list, and an unclaimed job reports only that nothing available claimed it",

		// The spans forbid `.` and `;` so a match stays inside one clause. With
		// `[\s\S]` they joined across sentence boundaries and flagged prose whose
		// whole point IS the correction -- "watching them queue shows that
		// nothing *claimed* them; a runner that advertises `docker` while
		// offline ... produces the identical observation". An absence claim about
		// a label is one clause; two clauses sharing three words are not one.
		//
		// Pattern and Stale are the same, as they are for the two rules below:
		// the construct itself is the defect, so zero hits is the correct state
		// and MinHits is 0. A subject/stale split was tried first and could not
		// be made to work -- the sentence review actually flagged says neither
		// "runner" nor "label", only `docker`, so every subject narrow enough
		// to be meaningful missed it and every one wide enough to catch it was
		// "any paragraph mentioning Docker".
		//
		// What this forbids is claiming an absence, not stating the fact. The
		// runner list says which runners carry which labels, and text that
		// reports what it SHOWS passes unchanged; text shaped as "nothing else
		// serves it" does not, because that shape is what got inferred from job
		// scheduling twice in one review cycle.
		//
		// The absence and the verb must land near a label, a runner, or a label
		// name. Without that anchor the construct is ordinary English -- an
		// unanchored draft flagged "Nothing was deleted" and "none is an agent,
		// and each carries its reason" across twelve unrelated documents.
		Pattern: `(?:nothing|no other|nobody|none|no runner)[^.;]{0,80}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))[^.;]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)|(?:nothing|no other|nobody|none|no runner)[^.;]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)[^.;]{0,60}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))`,
		Stale:   `(?:nothing|no other|nobody|none|no runner)[^.;]{0,80}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))[^.;]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)|(?:nothing|no other|nobody|none|no runner)[^.;]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)[^.;]{0,60}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))`,

		MinHits: 0, // zero is the correct state, as for the two rules below

		// Permanent: diagnosing a stalled job is a recurring activity and this
		// is the natural inference to draw each time -- a job that never runs
		// feels like proof that nothing could have run it.
		RetireAfter: "",
	},
	{
		ID:    "dossier-file-per-task",
		Added: "G26",
		Why:   "two source documents said dossiers are one file per task while a behaviour workstream's two tasks share one; the unit is the workstream",

		// Subject: any statement of the counting rule, in either wording. The
		// corrected text says "file per workstream", so sweeping only the old
		// phrasing would stop finding anything the moment it was fixed and the
		// rule could not tell a correction from an absence.
		//
		// The emphasis markers are not decoration. The index writes the unit as
		// `per **workstream**`, and a pattern without them matched the plan and
		// silently missed the index -- which made the site count look right for
		// the wrong reason.
		//
		// WHAT THIS CANNOT DETECT: the rule being deleted outright. MinHits is
		// satisfied by the passages that QUOTE the superseded wording, so both
		// live statements can vanish and this still passes. It guards the wrong
		// unit coming back, not the right one going away. A check that both
		// documents state the rule would catch that, and does not exist.
		Pattern: `file per \*{0,2}(?:task|workstream)`,

		// Stale is the superseded unit. A hit that still says "per task" is the
		// defect; "per workstream" matches Pattern and not this, and passes.
		Stale: `file per \*{0,2}task`,

		MinHits: 2,

		// Retires after C05: by then several behaviour workstreams have split
		// and the convention has been exercised rather than merely written.
		RetireAfter: "C05",
	},
	{
		ID:    "runner-cannot-run-containers",
		Added: "G28",
		Why:   "four documents said a runner cannot run containers; the probe measured the container a JOB runs inside, and its own row `a job runs inside a container` passed -- the host starts containers, which is how the job existed",

		// Written at G27, withdrawn there, landed here. It could not be
		// demonstrated at G27: every superseded wording is bold, and the
		// emphasis classifier disposed of bold as though it were quotation, so
		// all three verbatim plantings passed. G28 fixed that first.
		//
		// Pattern and Stale are the same: the claim is the defect. What a job is
		// handed and what a machine can do are different statements, and the
		// second was never measured here. Say it about the job -- "no job on
		// this self-hosted runner can reach Docker" -- which is what the probe
		// establishes and is sufficient for every conclusion drawn from it.
		//
		// Four alternatives because the claim was rewritten three times and each
		// rewrite kept the error in a new grammar: "cannot run", "No X can run",
		// and finally "no job on a runner available to this repository", which
		// fixes the subject and keeps the quantifier. Spans forbid `.` and `;`
		// for the reason given on runner-label-absence above.
		Pattern: `(?:runners?|pool|machine|host)[^.;]{0,70}(?:can ?not|cannot|could not|unable to)[^.;]{0,50}containers?|(?:can ?not|cannot|could not|unable to)[^.;]{0,50}(?:run|start)[^.;]{0,25}containers?|\bno\b[^.;]{0,40}(?:runners?|pool|machine|host)[^.;]{0,60}\bcan\b[^.;]{0,30}(?:run|start)[^.;]{0,25}containers?|no (?:job|jobs)[^.;]{0,40}(?:a runner available|any runner|runners? available|every runner)[^.;]{0,60}(?:reach|run|start)`,
		Stale:   `(?:runners?|pool|machine|host)[^.;]{0,70}(?:can ?not|cannot|could not|unable to)[^.;]{0,50}containers?|(?:can ?not|cannot|could not|unable to)[^.;]{0,50}(?:run|start)[^.;]{0,25}containers?|\bno\b[^.;]{0,40}(?:runners?|pool|machine|host)[^.;]{0,60}\bcan\b[^.;]{0,30}(?:run|start)[^.;]{0,25}containers?|no (?:job|jobs)[^.;]{0,40}(?:a runner available|any runner|runners? available|every runner)[^.;]{0,60}(?:reach|run|start)`,

		MinHits: 0, // zero is the correct state, as for the rules below

		// Permanent: the probe's rows read like facts about a machine, and the
		// shortest summary of them is the wrong one. G25 made exactly that
		// substitution while correcting the sentence's other error.
		RetireAfter: "",
	},
	{
		ID:    "r6-unmet-suite-deferred",
		Added: "G33",
		Why:   "R6 was unmet and the container suite deferred; P4 (run 881) met R6, the suite is in `check` and in CI, and `deferred-gates-check` is gone",

		// Pattern is the SUBJECT, Stale the superseded conclusion. Mentioning
		// R6 or the suite is not a defect -- several documents must, and
		// CI-RUNNER.md records both states with dates. Asserting the old
		// conclusion in the present tense is.
		//
		// Three grammars, because the claim was written three ways before G33:
		// as a property of R6 ("not met", "unmet"), as a property of the gate
		// ("deliberately absent", "deferred"), and as a property of the job
		// ("no Docker client"). A fix that changed only the first would leave
		// the other two reading correct. Spans forbid `.` and `;` for the
		// reason runner-label-absence gives.
		//
		// The deferral alternatives require a VERB -- "is deferred", "remains
		// deferred", "deliberately absent" -- rather than the bare word. A
		// first draft matched `deferred` alone and hit `deferred-gates-check`
		// itself, so naming the target G33 deleted, in order to say it was
		// deleted, read as the claim it was written to forbid. RE2 has no
		// lookahead, so the verb is the discriminator.
		Pattern: `\bR6\b|container[- ]suite|container suite`,
		Stale: `\bR6\b[^.;]{0,70}(?:is|remains|currently)?[^.;]{0,20}(?:not met|unmet|currently fails)` +
			`|(?:not met|unmet)[^.;]{0,40}\bR6\b` +
			`|container[- ]suite[^.;]{0,90}(?:is deferred|deliberately absent|deliberately not in|remains deferred)` +
			`|(?:is|remains|stays) deferred[^.;]{0,70}container[- ]suite` +
			`|deliberately absent[^.;]{0,70}container[- ]suite` +
			`|(?:jobs?|runner|CI)[^.;]{0,70}(?:have|has|had) no Docker client`,

		// The subject is everywhere -- CI-RUNNER.md alone carries many -- so a
		// vanished subject means the rule stopped reading what it claims to.
		MinHits: 8,

		Exempt: map[string]string{
			"docs/dossiers/P11-acceptance-evidence.md": "P11's own acceptance record, dated and marked superseded in place; it is evidence of what was true then, not a claim about the tree",
		},

		// Retires after C05: by then the suite runs a real journey across the
		// topology, and a document claiming it is deferred would contradict
		// something far louder than this sweep.
		RetireAfter: "C05",
	},
	{
		ID:    "encoder-format-is-c01s",
		Added: "G36",
		Why:   "ADR-0005 said C01 owns `the canonical encoder` while section 1 defined no framing, so the length prefix belonged to nobody and C01-A stopped; G36 gave the format to the ADR and the implementation to C01",

		// Pattern is the SUBJECT -- the encoder, the encoding, the framing, the
		// length prefix -- and Stale the superseded conclusion: that any of it
		// is C01's to choose. Naming C01 as the encoder's IMPLEMENTER is correct
		// and must not hit, which is why the stale side requires a verb of
		// choosing rather than mere adjacency.
		//
		// C01-A is the very next task and is the most likely place to re-derive
		// the old reading, since its dossier asks it to hand-write framing
		// vectors and the temptation is to settle an undefined byte locally.
		//
		// WHAT THIS CANNOT DETECT: an ownership list naming C01 in a third
		// grammar -- neither a verb of choosing nor `encoder,` before a
		// `(**C01**)` parenthetical. The two forms covered are the ones that
		// have actually been written; a third needs adding when it appears.
		Pattern: `canonical encoder|length[- ]prefix|framing|wire format|encoding`,
		Stale: `(?:encoder|encoding|framing|length[- ]prefix|wire format)[^.;]{0,80}(?:is|are|remains?)[^.;]{0,30}\bC01\b` +
			`|\bC01\b[^.;]{0,60}(?:chooses?|choose|decides?|defines?|picks?|selects?)[^.;]{0,40}(?:encoder|encoding|framing|length[- ]prefix|wire format|byte order|prefix width)` +
			`|(?:encoder|encoding|framing|length[- ]prefix)[^.;]{0,60}(?:to be (?:chosen|decided|defined))[^.;]{0,40}(?:by |at |in )?\bC01\b` +
			// The shape that actually caused the blocker, and the first draft of
			// this rule MISSED it: a bare list ending in a parenthetical owner --
			// "the canonical encoder, the signature ... vectors (**C01**)". There
			// is no verb to match, so the discriminator is the comma or "and"
			// straight after `encoder`; the corrected text reads `encoder's
			// **implementation**`, which this must not hit and which RE2 cannot
			// express as a negative lookahead.
			`|canonical encoder(?:,| and)[^.;]{0,160}\(\*\*C01\*\*\)`,

		// The subject is unavoidable in ADR-0005, C01.md and TESTING.md, so a
		// vanished subject means the sweep stopped reading what it claims to.
		MinHits: 6,

		// Retires after C01: by then the encoder exists and its bytes are frozen
		// as goldens, so a document claiming C01 may choose them contradicts
		// something louder than this sweep.
		//
		// `C01`, not `C01-I`, and the difference is not cosmetic. The lifetime
		// list is built by the Makefile's `sed -n 's/^ *- \[x\] \([PCRG][0-9][0-9][ab]\?\) .*/\1/p'`,
		// which cannot capture a workstream half. `RetireAfter: "C01-I"` would
		// never be found in that list, so the rule would never be forced out and
		// would become permanent with a lifetime that reads finite. Nothing in
		// the tool checks that RetireAfter names a task the list can contain.
		RetireAfter: "C01",
	},
	{
		ID:    "primitives-are-c01s",
		Added: "G37",
		Why:   "ADR-0005 named three key roles and no algorithm, so the signature and encryption primitives belonged to nobody and C01-I stopped; G37 gave the primitives to the ADR and their implementation to C01",

		// The sibling of encoder-format-is-c01s, and it exists because that rule
		// was not enough: G36 sharpened `the canonical encoder` and left `the
		// signature and encryption operations` in the same sentence untouched,
		// so the identical defect sat one clause to the right until C01-I hit
		// it. Pattern is the SUBJECT, Stale the superseded conclusion.
		//
		// WHAT THIS CANNOT DETECT, and the second half was learned by trying:
		//
		//  1. An algorithm chosen silently in code with no document claiming the
		//     right to choose it. That is archlint's and review's ground.
		//  2. The ownership LIST itself. encoder-format-is-c01s can match
		//     `canonical encoder,` before a `(**C01**)` parenthetical because the
		//     corrected text reads `encoder's`. There is no equivalent here: the
		//     phrase `the signature and encryption operations ... (**C01**)`
		//     survives the correction verbatim, since C01 still owns those
		//     operations as implementations. A first draft matched it and fired
		//     on the fixed sentence. The verbs below are all that discriminate.
		// The subject list must cover every noun the Stale side names, or a
		// stale claim is invisible because the SUBJECT never matched. A first
		// draft listed only `primitive|algorithm` and silently missed planted
		// claims about a `cipher`, a `curve` and a `signature scheme` -- the
		// rule read as though it swept them and did not.
		Pattern: `signature (?:and|or) encryption|primitives?|algorithms?|cipher|curve|signature scheme|Ed25519|ML-DSA|ML-KEM|X25519|AES-`,
		Stale: `(?:algorithms?|primitives?|cipher|signature scheme|curve)[^.;]{0,70}(?:is|are|remains?)[^.;]{0,25}\bC01\b` +
			`|\bC01\b[^.;]{0,60}(?:chooses?|choose|decides?|picks?|selects?)[^.;]{0,45}(?:algorithms?|primitives?|cipher|curve|signature scheme)` +
			`|(?:algorithms?|primitives?)[^.;]{0,60}(?:to be (?:chosen|decided|selected))[^.;]{0,40}(?:by |at |in )?\bC01\b`,

		// The subject is unavoidable in ADR-0005, ADR-0003 and THREAT-MODEL.md.
		MinHits: 6,

		// Retires after C01: by then the primitives are implemented and frozen
		// in goldens, and a document claiming C01 may choose them contradicts
		// something louder than this sweep.
		RetireAfter: "C01",
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
