// Package targeting compiles user-facing target expressions into an
// expr-lang VM program that can be evaluated against a flattened agent
// metadata map (Epic 07 task 1).
//
// Shorthand syntax:
//
//	field:value             // built-in field equality / glob
//	field.subfield:value    // nested map access (e.g., labels.env:prod)
//	non-builtin:value       // sugar for labels.<field>:value
//	"value with spaces"     // quoted value (also supports single-quotes)
//	AND, OR, NOT, ( )       // boolean composition (case-insensitive,
//	                        // && / || / ! also accepted)
//
// Built-in fields: id, hostname, os, arch, status, ip. Anything else is
// treated as a label key.
//
// Tasks 2 and 3 add the metadata flattener and the Matcher that runs
// compiled programs against an agent record. CIDR handling for `ip:`
// also lands in task 2 alongside the flattener.
package targeting
