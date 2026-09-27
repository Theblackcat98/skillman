package main

// Man page generation. docs/skillman.1 is generated from the same tables
// as --help, the ? overlay and the README, and TestManPageIsUpToDate
// fails when the checked-in file stops matching.
//
// A hand-written man page is a fifth copy of the command list, and it
// would be the copy nobody remembers to update (review D9, D10). This is
// the other option: the file is checked in so it can be read without
// building, and a test fails if it drifts.

import (
	"fmt"
	"strings"
)

// manPagePath is where the generated page is written.
const manPagePath = "docs/skillman.1"

// manPage renders the roff source.
func manPage() string {
	var b strings.Builder
	w := func(format string, a ...any) {
		fmt.Fprintf(&b, format+"\n", a...)
	}

	w(".\\\" Generated from the command tables in main.go and keymap.go.")
	w(".\\\" Run: go test -run TestManPage -update")
	w(".TH SKILLMAN 1 %q %q", date, "skillman "+version)
	w(".SH NAME")
	w("skillman \\- manage opencode skills")
	w(".SH SYNOPSIS")
	w(".B skillman")
	w("[\\fIOPTIONS\\fR]")
	w(".br")
	w(".B skillman list")
	w("[\\fB\\-\\-json\\fR|\\fB\\-\\-names\\fR|\\fB\\-\\-long\\fR]")
	w(".br")
	w(".B skillman view")
	w("\\fINAME\\fR")
	w(".br")
	w(".B skillman validate")
	w("[\\fB\\-\\-json\\fR]")
	w(".br")
	w(".B skillman delete")
	w("\\fINAME\\fR \\fB\\-\\-yes\\fR")
	w(".br")
	w(".B skillman trash")
	w("\\fIlist|restore|purge\\fR [\\fB\\-\\-json\\fR|\\fB\\-\\-all\\fR|\\fB\\-\\-older\\-than\\fR \\fIAGE\\fR]")
	w(".br")
	w(".B skillman completion")
	w("\\fIbash|zsh\\fR")
	w(".br")
	w(".B skillman install")
	w("\\fIURL\\fR [\\fB\\-\\-ref\\fR \\fIREF\\fR] [\\fB\\-\\-dry\\-run\\fR]")

	w(".SH DESCRIPTION")
	w("Browse, preview, validate, edit and trash-delete skills in the")
	w("opencode skills directory. With no command and a terminal it starts")
	w("the TUI; with no command and no terminal it prints the list, so a")
	w("pipe or a script gets text instead of an escape sequence.")
	w(".PP")
	w("A skill is a directory holding a SKILL.md whose YAML frontmatter")
	w("carries at least a name and a description. A scan reads the")
	w("frontmatter only; the markdown body is read when a skill is")
	w("previewed or printed.")

	w(".SH COMMANDS")
	for _, c := range commands {
		syn := c.name
		if c.arg != "" {
			syn += " " + c.arg
		}
		w(".TP")
		w(".B " + syn)
		w(c.usage + describeAliases(c.aliases))
	}

	w(".SH OPTIONS")
	w(".TP")
	w(".B \\-\\-plain")
	w("No colours and no markdown styling. Also set by NO_COLOR and TERM=dumb.")
	w(".TP")
	w(".B \\-\\-no\\-animations")
	w("Disable the spinner. Also set by NO_ANIMATIONS, REDUCED_MOTION and CI.")
	w(".TP")
	w(".B \\-\\-skills\\-dir DIR")
	w("Override the skills directory. Also set by SKILLMAN_SKILLS and config.yaml.")
	w(".TP")
	w(".B \\-\\-long")
	w("list: add category, licence, size and mtime.")
	w(".TP")
	w(".B \\-\\-older\\-than AGE")
	w("trash purge: only entries older than AGE, which is 30d or 720h.")
	w(".TP")
	w(".B \\-\\-all")
	w(roffEscape("trash purge: every entry. Needs --yes, or the command exits 3."))
	w(".TP")
	w(".B \\-\\-yes, \\-y")
	w("Confirm a destructive action instead of asking.")
	w(".TP")
	w(".B \\-\\-help, \\-h")
	w("Print the help text and exit.")
	w(".TP")
	w(".B \\-\\-version")
	w("Print the version and exit.")

	w(".SH TUI KEYS")
	for _, k := range keyTable {
		w(".TP")
		w(".B " + roffEscape(k.Keys))
		w(roffEscape(k.Desc) + rebindNote(k.Action))
	}

	w(".PP")
	w("The install checklist: j/k move, space ticks one, a ticks all, g/G")
	w("jump, enter installs, esc cancels and removes the clone.")

	w(".SH EXIT STATUS")
	for _, e := range exitCodeDoc {
		w(".TP")
		w(fmt.Sprintf(".B %d", e.Code))
		w(roffEscape(e.Meaning))
	}

	w(".SH ENVIRONMENT")
	for _, e := range envVarDoc {
		w(".TP")
		w(".B " + e.Name)
		w(roffEscape(e.Effect))
	}

	w(".SH FILES")
	w(".TP")
	w(".B " + configPath())
	w("Configuration: skills_dir, accent, and key bindings.")
	w(".TP")
	w(".B " + statePath())
	w("Session state: last selection, preview scroll, pane focus.")
	w(".TP")
	w(".B " + trashDir())
	w("Deleted skills, restorable with skillman trash restore.")

	w(".SH SEE ALSO")
	w("The project README is the full documentation and the single source")
	w("of truth for the tables above.")
	return b.String()
}

// date is the man page date. It is a constant rather than the build time
// so regenerating the file does not produce a diff on every run.
const date = "2026-09-27"

func describeAliases(aliases []string) string {
	if len(aliases) == 0 {
		return ""
	}
	return " (alias: " + strings.Join(aliases, ", ") + ")"
}

func rebindNote(action string) string {
	if action == "" {
		return ""
	}
	return " Rebindable as " + action + "."
}

// roffEscape makes a string safe as roff body text: a leading dot or
// apostrophe would start a request, a backslash would escape the next
// character, and "--" would render as an en-dash. Dashes are escaped
// unconditionally — \- always renders as a plain hyphen, so doing it
// everywhere removes the whole class of bug rather than the two
// instances that happened to show up.
func roffEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	s = strings.ReplaceAll(s, "-", `\-`)
	var b strings.Builder
	for i, r := range s {
		if (r == '.' || r == '\'') && i == 0 {
			b.WriteString(`\` + string(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
