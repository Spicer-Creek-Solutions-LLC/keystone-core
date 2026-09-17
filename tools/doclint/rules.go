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
		Pattern: `(?:nothing|no other|nobody|none|no runner)[\s\S]{0,80}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))[\s\S]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)|(?:nothing|no other|nobody|none|no runner)[\s\S]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)[\s\S]{0,60}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))`,
		Stale:   `(?:nothing|no other|nobody|none|no runner)[\s\S]{0,80}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))[\s\S]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)|(?:nothing|no other|nobody|none|no runner)[\s\S]{0,60}(?:labels?\b|runners?\b|docker|ubuntu-latest)[\s\S]{0,60}(?:serves?|served|advertis\w*|answers? to|carr(?:y|ies|ied))`,

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
		Added: "G27",
		Why:   "four documents said a runner cannot run containers; the probe measured the container a JOB runs inside, and its own row `a job runs inside a container` passed -- the host starts containers, which is how the job existed",

		// Pattern and Stale are the same: the claim is the defect. What a job
		// is handed and what a machine can do are different statements, and the
		// second was never measured here.
		//
		// The third alternative is negation by "NO runner ... CAN run", which
		// the first draft missed -- and that is the exact form G25 introduced,
		// so the rule did not catch the instance it was written for. Second
		// time in one session: a sweep aimed at a defect, tested only against
		// paraphrases of it.
		//
		// Say it about the job -- "no job on a runner available here can reach
		// Docker" -- which is what the probe establishes and is sufficient for
		// every conclusion the documents draw from it.
		Pattern: `(?:runners?|pool|machine|host)[\s\S]{0,70}(?:can ?not|cannot|could not|unable to)[\s\S]{0,50}containers?|(?:can ?not|cannot|could not|unable to)[\s\S]{0,50}(?:run|start)[\s\S]{0,25}containers?|\bno\b[\s\S]{0,40}(?:runners?|pool|machine|host)[\s\S]{0,60}\bcan\b[\s\S]{0,30}(?:run|start)[\s\S]{0,25}containers?`,
		Stale:   `(?:runners?|pool|machine|host)[\s\S]{0,70}(?:can ?not|cannot|could not|unable to)[\s\S]{0,50}containers?|(?:can ?not|cannot|could not|unable to)[\s\S]{0,50}(?:run|start)[\s\S]{0,25}containers?|\bno\b[\s\S]{0,40}(?:runners?|pool|machine|host)[\s\S]{0,60}\bcan\b[\s\S]{0,30}(?:run|start)[\s\S]{0,25}containers?`,

		MinHits: 0, // zero is the correct state, as for the rules below

		// Permanent: the probe's rows read like facts about a machine, and the
		// shortest way to summarise them is the wrong one. G25 made exactly
		// that substitution while correcting the sentence's other error.
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
