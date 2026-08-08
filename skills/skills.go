// Package skills carries the agent integrations that ship with wo.
//
// The markdown is embedded rather than read from disk so that an installed
// binary can write it out wherever it is run from - a homebrew install has no
// checkout to copy it from.
package skills

import _ "embed"

// Claude is the Claude Code skill describing how to drive wo.
//
//go:embed wo/SKILL.md
var Claude string

// ClaudeName is the directory the skill installs into, under the skills
// directory. It is the name Claude Code addresses the skill by.
const ClaudeName = "wo"
