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
	if got := code(func() int { return runList(false, true, false, false) }); got != 0 {
		t.Errorf("empty list exit = %d, want 0", got)
	}
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if got := code(func() int { return runList(false, true, false, false) }); got != 0 {
		t.Errorf("list exit = %d, want 0", got)
	}
	if got := code(func() int { return runList(true, true, false, false) }); got != 0 {
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

// --- phase 3: the flag contract -------------------------------------------

// A flag that is silently ignored is worse than a flag that is rejected:
// the script asks for JSON, gets rendered text, and fails later somewhere
// unrelated (review A9, E15).
func TestUnsupportedFlagsAreRejected(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	cases := [][]string{
		{"view", "demo", "--json"},
		{"view", "demo", "--names"},
		{"view", "demo", "--long"},
		{"delete", "demo", "--json"},
		{"delete", "demo", "--long"},
		{"validate", "--names"},
		{"validate", "--long"},
		{"trash", "list", "--long"},
		{"completion", "bash", "--json"},
		{"completion", "bash", "--yes"},
		{"install", "https://example.com/x", "--json"},
		{"list", "--names", "--json"},
	}
	for _, args := range cases {
		if got := code(func() int { return run(args) }); got != 2 {
			t.Errorf("run(%v) = %d, want 2 (flag silently ignored?)", args, got)
		}
	}
}

// `view a b c` used to drop b and c without a word.
func TestViewRejectsExtraArguments(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	for _, args := range [][]string{
		{"view", "demo", "extra"},
		{"view", "a", "b", "c"},
		{"delete", "demo", "extra"},
	} {
		if got := code(func() int { return run(args) }); got != 2 {
			t.Errorf("run(%v) = %d, want 2", args, got)
		}
	}
}

// A skill whose name looks like a flag has to be reachable, or the flag
// parser is a name parser with extra steps.
func TestDoubleDashReachesFlagShapedNames(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "--names", validFM("flagname", "a skill called --names"))
	writeSkill(t, skills, "-h", validFM("dashh", "a skill called -h"))

	if got := code(func() int { return run([]string{"view", "--plain", "--", "--names"}) }); got != 0 {
		t.Errorf("view -- --names = %d, want 0", got)
	}
	if got := code(func() int { return run([]string{"view", "--plain", "--", "-h"}) }); got != 0 {
		t.Errorf("view -- -h = %d, want 0", got)
	}
	// And --help still works as a flag.
	if got := code(func() int { return run([]string{"--help"}) }); got != 0 {
		t.Errorf("--help = %d, want 0", got)
	}
}

// Size and ModTime used to be computed by a full tree walk and then
// discarded. --long is where they are worth the walk (review D1).
func TestListLongSurfacesSizeAndModTime(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "demo",
		"---\nname: demo\ndescription: d\nlicense: MIT\ncompatibility: any\nmetadata:\n  category: dev\n---\n\nbody\n")

	short := captureStdout(t, func() { runList(false, true, false, false) })
	if strings.Contains(short, "size=") {
		t.Errorf("plain list shows --long fields:\n%s", short)
	}
	long := captureStdout(t, func() { runList(false, true, false, true) })
	for _, want := range []string{"size=", "modified=", "demo"} {
		if !strings.Contains(long, want) {
			t.Errorf("list --long missing %q:\n%s", want, long)
		}
	}
	// And they are machine-readable too.
	js := captureStdout(t, func() { runList(true, true, false, false) })
	if !strings.Contains(js, `"size_bytes"`) || !strings.Contains(js, `"modified"`) {
		t.Errorf("list --json missing size/modified:\n%s", js)
	}
	for _, want := range []string{`"license": "MIT"`, `"compatibility": "any"`, `"category": "dev"`} {
		if !strings.Contains(js, want) {
			t.Errorf("list --json missing %s:\n%s", want, js)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0: "0 B", 512: "512 B", 1024: "1.0 KiB", 1536: "1.5 KiB",
		1024 * 1024: "1.0 MiB", 3 * 1024 * 1024 * 1024: "3.0 GiB",
	}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// Piped output must be plain: glamour emitted an escape sequence per
// padding space, so `view x > out.md` produced a bloated, mangled file
// (review D3).
func TestPipedViewIsPlainByDefault(t *testing.T) {
	skills, _ := isolatedEnv(t)
	md := "---\nname: demo\ndescription: d\n---\n\n# Heading\n\nSome **bold** text and a list:\n\n- one\n- two\n"
	writeSkill(t, skills, "demo", md)

	// The test binary's stdout is not a terminal, which is exactly the
	// case under test.
	if !plainOutput(false) {
		t.Error("plainOutput is false with stdout not a terminal")
	}
	out := captureStdout(t, func() { runView("demo", plainOutput(false)) })
	if strings.Contains(out, "\x1b") {
		t.Errorf("piped view is not plain:\n%q", out)
	}
	// With the escape hatch, colour comes back.
	t.Setenv("SKILLMAN_COLOR", "always")
	if plainOutput(false) {
		t.Error("SKILLMAN_COLOR=always did not override the non-terminal default")
	}
	styled := captureStdout(t, func() { runView("demo", plainOutput(false)) })
	if !strings.Contains(styled, "\x1b") {
		t.Error("SKILLMAN_COLOR=always produced no colour; the hatch does nothing")
	}
	// --plain still wins.
	if !plainOutput(true) {
		t.Error("--plain did not win over SKILLMAN_COLOR=always")
	}
}

func TestVersionFlag(t *testing.T) {
	out := captureStdout(t, func() {
		if got := run([]string{"--version"}); got != 0 {
			t.Errorf("--version = %d, want 0", got)
		}
	})
	if !strings.Contains(out, version) {
		t.Errorf("--version printed %q, want it to contain %q", out, version)
	}
}
