package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// CLI exit-code contract: 0 ok, 1 issues/not found, 2 missing
// SKILL.md or bad usage, 3 destructive without --yes.

func TestListExitCodes(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	if got := code(func() int { return runList(false, true, false) }); got != 0 {
		t.Errorf("empty list exit = %d, want 0", got)
	}
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if got := code(func() int { return runList(false, true, false) }); got != 0 {
		t.Errorf("list exit = %d, want 0", got)
	}
	if got := code(func() int { return runList(true, true, false) }); got != 0 {
		t.Errorf("list --json exit = %d, want 0", got)
	}
}

func TestViewExitCodes(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if got := code(func() int { return runView("demo", true) }); got != 0 {
		t.Errorf("view found exit = %d, want 0", got)
	}
	if got := code(func() int { return runView("nope", true) }); got != 1 {
		t.Errorf("view missing exit = %d, want 1", got)
	}
}

func TestValidateExitCodes(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)

	writeSkill(t, skills, "good", validFM("good", "fine"))
	if got := code(func() int { return runValidate(false) }); got != 0 {
		t.Errorf("all-valid exit = %d, want 0", got)
	}

	writeSkill(t, skills, "mismatch", validFM("other", "fine"))
	if got := code(func() int { return runValidate(false) }); got != 1 {
		t.Errorf("warn exit = %d, want 1", got)
	}

	if err := os.MkdirAll(filepath.Join(skills, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := code(func() int { return runValidate(false) }); got != 2 {
		t.Errorf("missing SKILL.md exit = %d, want 2", got)
	}
	if got := code(func() int { return runValidate(true) }); got != 2 {
		t.Errorf("missing SKILL.md --json exit = %d, want 2", got)
	}
}

func TestDeleteCLIRequiresYes(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))

	if got := code(func() int { return runDeleteCLI("demo", false) }); got != 3 {
		t.Errorf("delete without --yes exit = %d, want 3", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("skill removed without --yes: %v", err)
	}

	if got := code(func() int { return runDeleteCLI("demo", true) }); got != 0 {
		t.Errorf("delete --yes exit = %d, want 0", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("skill dir still present after delete --yes")
	}
	if got := code(func() int { return runDeleteCLI("demo", true) }); got != 1 {
		t.Errorf("delete missing exit = %d, want 1", got)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Setenv("SKILLMAN_SKILLS", t.TempDir())
	if got := code(func() int { return run([]string{"bogus"}) }); got != 2 {
		t.Errorf("unknown command exit = %d, want 2", got)
	}
	if got := code(func() int { return run([]string{"--nope"}) }); got != 2 {
		t.Errorf("unknown flag exit = %d, want 2", got)
	}
}

// Regression: audit B8 — bare --json must print JSON, never start a TUI.
func TestBareJSONPrintsList(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if got := code(func() int { return run([]string{"--json"}) }); got != 0 {
		t.Errorf("bare --json exit = %d, want 0", got)
	}
}

// --- Phase 11: trash CLI, completion, flag handling ---

func TestTrashCLIListJSON(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "demo", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	entries, _ := listTrash()
	for _, e := range entries {
		_ = os.Chtimes(e.Path, old, old)
	}
	if got := code(func() int { return runTrash(nil, true, false, "") }); got != 2 {
		t.Errorf("trash with no subcommand = %d, want 2", got)
	}
	if got := code(func() int { return runTrash([]string{"nope"}, false, false, "") }); got != 2 {
		t.Errorf("unknown trash subcommand = %d, want 2", got)
	}
	if got := code(func() int { return runTrash([]string{"list"}, true, false, "") }); got != 0 {
		t.Errorf("trash list --json = %d, want 0", got)
	}
}

func TestTrashCLIRestoreAndPurge(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "demo", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if got := code(func() int { return runTrash([]string{"restore"}, false, false, "") }); got != 2 {
		t.Errorf("restore with no name = %d, want 2", got)
	}
	if got := code(func() int { return runTrash([]string{"restore", "demo"}, false, false, "") }); got != 0 {
		t.Errorf("restore = %d, want 0", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("skill not restored: %v", err)
	}
	if got := code(func() int { return runTrash([]string{"restore", "demo"}, false, false, "") }); got != 1 {
		t.Errorf("restore of a missing entry = %d, want 1", got)
	}

	// Purge must be told what to do.
	if _, err := deleteSkillToTrash(Skill{Name: "demo", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if got := code(func() int { return runTrash([]string{"purge"}, false, false, "") }); got != 2 {
		t.Errorf("purge with no selector = %d, want 2", got)
	}
	if got := code(func() int { return runTrash([]string{"purge", "--older-than", "soon"}, false, false, "soon") }); got != 2 {
		t.Errorf("purge with a bad age = %d, want 2", got)
	}
	if got := code(func() int { return runTrash([]string{"purge"}, false, true, "") }); got != 0 {
		t.Errorf("purge --all = %d, want 0", got)
	}
	entries, err := listTrash()
	if err != nil || len(entries) != 0 {
		t.Errorf("trash not empty after purge --all: %v %v", entries, err)
	}
}

func TestCompletionShells(t *testing.T) {
	if got := code(func() int { return runCompletion("fish") }); got != 2 {
		t.Errorf("unknown shell = %d, want 2", got)
	}
	if got := code(func() int { return runCompletion("") }); got != 2 {
		t.Errorf("empty shell = %d, want 2", got)
	}
	for _, sh := range []string{"bash", "zsh"} {
		if got := code(func() int { return runCompletion(sh) }); got != 0 {
			t.Errorf("completion %s = %d, want 0", sh, got)
		}
	}
	// The bash script must be syntactically valid and must mention the
	// commands it completes.
	if !strings.Contains(bashCompletion, "complete -F _skillman skillman") {
		t.Error("bash completion does not register the function")
	}
	for _, want := range []string{"trash", "restore", "purge", "completion", "--skills-dir"} {
		if !strings.Contains(bashCompletion, want) {
			t.Errorf("bash completion missing %q", want)
		}
	}
	if !strings.HasPrefix(zshCompletion, "#compdef skillman") {
		t.Error("zsh completion is missing its compdef header")
	}
}

// A skill named "help" must be reachable: the bare word is only help
// when it is the whole command line.
func TestHelpDoesNotShadowASkill(t *testing.T) {
	// isolatedEnv, not just SKILLMAN_SKILLS: this test deletes, and
	// delete moves the skill into the real trash unless XDG_DATA_HOME
	// points somewhere temporary.
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "help", validFM("help", "a skill literally named help"))
	if got := code(func() int { return run([]string{"view", "help", "--plain"}) }); got != 0 {
		t.Errorf("view help = %d, want 0", got)
	}
	if got := code(func() int { return run([]string{"delete", "help", "--yes"}) }); got != 0 {
		t.Errorf("delete help = %d, want 0", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the skill named help was not deleted")
	}
	if got := code(func() int { return run([]string{"help"}) }); got != 0 {
		t.Errorf("bare help = %d, want 0", got)
	}
	if got := code(func() int { return run([]string{"delete"}) }); got != 2 {
		t.Errorf("delete with no name = %d, want 2", got)
	}
	if got := code(func() int { return run([]string{"delete", "help", "--yes"}) }); got != 1 {
		t.Errorf("delete of an already-deleted skill = %d, want 1", got)
	}
}

func TestFlagsRejected(t *testing.T) {
	t.Setenv("SKILLMAN_SKILLS", t.TempDir())
	for _, args := range [][]string{
		{"--skills-dir="},      // empty override silently ignored the default
		{"--skills-dir", ""},   // same, split form
		{"--skills-dir"},       // missing value
		{"--older-than"},       // missing value
		{"--nope"},             // unknown flag
		{"view"},               // missing name
		{"delete"},             // missing name
		{"trash"},              // missing subcommand
		{"trash", "restore"},   // missing name
		{"completion"},         // missing shell
		{"completion", "fish"}, // unsupported shell
	} {
		if got := code(func() int { return run(args) }); got != 2 {
			t.Errorf("run(%v) = %d, want 2", args, got)
		}
	}
	// install is "not built", which is its own exit code.
	if got := code(func() int { return run([]string{"install", "https://example.com/x"}) }); got != 4 {
		t.Errorf("install = %d, want 4", got)
	}
}

func TestListNames(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	writeSkill(t, skills, "beta", validFM("beta", "b"))
	writeSkill(t, skills, "alpha", validFM("alpha", "a"))
	if got := code(func() int { return run([]string{"list", "--names"}) }); got != 0 {
		t.Errorf("list --names = %d, want 0", got)
	}
	if got := code(func() int { return run([]string{"--names"}) }); got != 0 {
		t.Errorf("bare --names = %d, want 0", got)
	}
}

// Regression: the exit code used to depend on alphabetical order,
// because it returned from inside the first invalid skill.
func TestValidateExitCodeIsOrderIndependent(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	writeSkill(t, skills, "aaa-warn", validFM("other", "warn only"))
	if err := os.MkdirAll(filepath.Join(skills, "zzz-err"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := code(func() int { return runValidate(false) }); got != 2 {
		t.Errorf("warn before error = %d, want 2 (worst wins)", got)
	}
	// Same skills, reversed names: the answer must not move.
	skills2 := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills2)
	if err := os.MkdirAll(filepath.Join(skills2, "aaa-err"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, skills2, "zzz-warn", validFM("other", "warn only"))
	if got := code(func() int { return runValidate(false) }); got != 2 {
		t.Errorf("error before warn = %d, want 2", got)
	}
}

// code runs a CLI command and returns its exit code with its output
// discarded, so `go test` shows results rather than pages of help text
// and JSON (review G7).
func code(f func() int) int {
	var c int
	quiet(func() { c = f() })
	return c
}
