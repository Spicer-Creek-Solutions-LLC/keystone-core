---
description: Present the next TODO.md item with a plan for approval
---

Follow the TODO approval workflow defined in CLAUDE.md. This is critical — never implement without explicit approval.

Steps:

1. **Read TODO.md**: Read the current `TODO.md` file at the project root.
2. **Find the next actionable item**: Identify the first incomplete/unchecked item. If `$ARGUMENTS` specifies a particular item or section, use that instead.
3. **Analyze the item**: Read all related code and documentation files to understand what needs to change.
4. **Present a plan**: Show the user:
   - What the TODO item requires
   - Which files will be created or modified
   - A brief description of each change
   - Any questions or ambiguities that need clarification
5. **STOP and wait**: Do NOT implement anything. Wait for the user to explicitly approve the plan with "yes" or similar confirmation.

Remember: The CLAUDE.md file requires explicit user approval before implementing any TODO item. This applies even after session resumption or context summarization. Never batch-fix TODOs without per-item approval.

$ARGUMENTS
