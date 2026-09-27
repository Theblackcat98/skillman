package main

import "strconv"

// One definition of the TUI keymap. The ? overlay, --help and the
// README key table are all generated from this list, so they cannot
// drift apart. TestDocsMatchKeymap keeps the README in step.
//
// The footer hint sets in view.go are a separate thing: they are
// width-tiered reminders, not a statement of what any key does.

// keyDef is one row of the keymap. Action is the name a config.yaml
// keybinding override binds to; it is empty for rows that are not
// rebindable.
type keyDef struct {
	Action string
	Keys   string
	Desc   string
}

const keyCol = 13 // width of the key column in help output

var keyTable = []keyDef{
	{"down", "j / down", "move selection down"},
	{"up", "k / up", "move selection up"},
	{"top", "g / home", "first skill"},
	{"bottom", "G / end", "last skill"},
	{"", "PgUp / PgDn", "page (Ctrl-B / Ctrl-F)"},
	{"pane", "Tab / Enter", "switch pane · focus preview"},
	{"help", "H / ?", "this help"},
	{"filter", "/", "filter · esc keeps, esc esc clears"},
	{"command", ":", "command: edit delete validate reload clear quit"},
	{"edit", "e", "open SKILL.md in $EDITOR"},
	{"delete", "d", "delete to trash"},
	{"undo", "u", "undo a delete, 30s window"},
	{"validate", "v", "validate all skills"},
	{"rescan", "r", "rescan the skills directory"},
	{"quit", "q", "quit from the base layer"},
	{"", "Esc", "back one layer"},
}

// keyTableLines renders the keymap, substituting any config override for
// the key in a rebindable row.
func keyTableLines(cfg Config) []string {
	out := make([]string, 0, len(keyTable))
	for _, k := range keyTable {
		keys := k.Keys
		if bound, ok := cfg.Keys[k.Action]; k.Action != "" && ok && bound != "" {
			keys = bound
		}
		out = append(out, padRight(keys, keyCol)+k.Desc)
	}
	return out
}

// envGateLines are the environment gates, in one list so the help
// overlay and the README say the same thing. The per-variable table is
// in envVarDoc; this is the one-line summary for the overlay.
var envGateLines = []string{
	"Respects NO_COLOR, TERM=dumb, NO_ANIMATIONS, REDUCED_MOTION, CI.",
}

// envVarDoc is the environment variable contract. --help lists the
// names; the README table is generated from this list.
var envVarDoc = []struct{ Name, Effect string }{
	{"SKILLMAN_SKILLS", "skills directory override"},
	{"XDG_CONFIG_HOME", "config.yaml and state.json live here"},
	{"XDG_DATA_HOME", "trash and skillman.log live here"},
	{"VISUAL", "editor to open SKILL.md with, before EDITOR"},
	{"EDITOR", "editor to open SKILL.md with"},
	{"NO_COLOR", "set: no colour, anywhere"},
	{"SKILLMAN_COLOR", "always: colour even when piped"},
	{"TERM", "dumb: no colour and no animation"},
	{"NO_ANIMATIONS", "set: no spinner, no transitions"},
	{"REDUCED_MOTION", "set: no spinner, no transitions"},
	{"CI", "set: no spinner, no transitions"},
}

func envVarLines() []string {
	out := make([]string, 0, len(envVarDoc))
	for _, e := range envVarDoc {
		out = append(out, "  "+padRight(e.Name, 18)+e.Effect)
	}
	return out
}

// exitCodeDoc is the exit code contract, in one list. The tests pin the
// behaviour; the README and --help are generated from here.
var exitCodeDoc = []struct {
	Code    int
	Meaning string
}{
	{0, "ok"},
	{1, "validation issues, or skill not found"},
	{2, "bad usage, or a skill has a broken frontmatter or no SKILL.md"},
	{3, "destructive action refused without --yes"},
	{4, "command not built in this version"},
}

func exitCodeLines() []string {
	out := make([]string, 0, len(exitCodeDoc))
	for _, e := range exitCodeDoc {
		out = append(out, "  "+padRight(strconv.Itoa(e.Code), 3)+e.Meaning)
	}
	return out
}
