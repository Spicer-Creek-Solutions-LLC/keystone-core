// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
)

// generationOneFinalSHA is the archive target recorded in
// docs/transition/manifest.json. Links in the retirement comment are pinned to
// it so they keep resolving after the baseline lands and the referenced files
// leave the tip of main.
const generationOneFinalSHA = "93eb147f7fcc559d31f2cce77d81e791f01673f8"

const (
	repoWebBase = "https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core"
	archiveRef  = "archive/2026-09-pre-v0.6-reboot"
)

// supersededComment is what gets posted on each retired issue.
//
// The wording matters more than it looks. These issues are being closed
// because the project changed direction, not because the work was done or was
// bad — and someone reading this in a year, having found it from a search
// engine, needs that distinction and a route to the reasoning without knowing
// any of this context. So the comment states plainly that it is superseded
// rather than completed, says the original content is retained, and links to
// the decision, the archive, and the catalog entry that preserves the
// capability.
func supersededComment(e AllowEntry) string {
	var b strings.Builder
	b.WriteString("**Superseded, not completed.**\n\n")
	b.WriteString("This issue is being closed as part of the Generation 2 reboot. ")
	b.WriteString("It is not closed because the work was finished, and not because it was judged unwanted — ")
	b.WriteString("the project restarted from a narrower base, and this issue described Generation 1 scope.\n\n")

	b.WriteString("Nothing here is deleted. The title, body, labels, comments and milestone are retained exactly as they were, ")
	b.WriteString("and the capability this issue described is preserved in the archive capability catalog.\n\n")

	fmt.Fprintf(&b, "- Decision: [RFC 0001](%s/src/commit/%s/docs/rfcs/0001-generation-2-reboot.md)\n",
		repoWebBase, generationOneFinalSHA)
	fmt.Fprintf(&b, "- Transition plan: [REBOOT-EXECUTION-PLAN.md](%s/src/commit/%s/docs/project/REBOOT-EXECUTION-PLAN.md)\n",
		repoWebBase, generationOneFinalSHA)
	fmt.Fprintf(&b, "- Capability catalog: [FUTURE-CAPABILITIES.md](%s/src/commit/%s/docs/project/FUTURE-CAPABILITIES.md)\n",
		repoWebBase, generationOneFinalSHA)
	fmt.Fprintf(&b, "- Archived source: `%s` at `%s`\n\n", archiveRef, generationOneFinalSHA[:9])

	b.WriteString("If this capability still matters, it is catalogued as a Future candidate rather than dropped. ")
	b.WriteString("Promotion needs operator evidence and an accepted design, not a reopened issue — ")
	b.WriteString("see the promotion gate in the execution plan.\n\n")
	b.WriteString("_Posted by `tools/transition apply` (reboot task R07)._")
	return b.String()
}
