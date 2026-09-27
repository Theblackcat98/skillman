package main

import (
	"os"
	"path/filepath"
	"testing"
)

// CLI exit-code contract: 0 ok, 1 issues/not found, 2 missing
// SKILL.md or bad usage, 3 destructive without --yes.

func TestListExitCodes(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	if code := runList(false, true); code != 0 {
		t.Errorf("empty list exit = %d, want 0", code)
	}
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if code := runList(false, true); code != 0 {
		t.Errorf("list exit = %d, want 0", code)
	}
	if code := runList(true, true); code != 0 {
		t.Errorf("list --json exit = %d, want 0", code)
	}
}

func TestViewExitCodes(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if code := runView("demo", true); code != 0 {
		t.Errorf("view found exit = %d, want 0", code)
	}
	if code := runView("nope", true); code != 1 {
		t.Errorf("view missing exit = %d, want 1", code)
	}
}

func TestValidateExitCodes(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)

	writeSkill(t, skills, "good", validFM("good", "fine"))
	if code := runValidate(false); code != 0 {
		t.Errorf("all-valid exit = %d, want 0", code)
	}

	writeSkill(t, skills, "mismatch", validFM("other", "fine"))
	if code := runValidate(false); code != 1 {
		t.Errorf("warn exit = %d, want 1", code)
	}

	if err := os.MkdirAll(filepath.Join(skills, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := runValidate(false); code != 2 {
		t.Errorf("missing SKILL.md exit = %d, want 2", code)
	}
	if code := runValidate(true); code != 2 {
		t.Errorf("missing SKILL.md --json exit = %d, want 2", code)
	}
}

func TestDeleteCLIRequiresYes(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))

	if code := runDeleteCLI("demo", false); code != 3 {
		t.Errorf("delete without --yes exit = %d, want 3", code)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("skill removed without --yes: %v", err)
	}

	if code := runDeleteCLI("demo", true); code != 0 {
		t.Errorf("delete --yes exit = %d, want 0", code)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("skill dir still present after delete --yes")
	}
	if code := runDeleteCLI("demo", true); code != 1 {
		t.Errorf("delete missing exit = %d, want 1", code)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	t.Setenv("SKILLMAN_SKILLS", t.TempDir())
	if code := run([]string{"bogus"}); code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
	if code := run([]string{"--nope"}); code != 2 {
		t.Errorf("unknown flag exit = %d, want 2", code)
	}
}

// Regression: audit B8 — bare --json must print JSON, never start a TUI.
func TestBareJSONPrintsList(t *testing.T) {
	skills := t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	if code := run([]string{"--json"}); code != 0 {
		t.Errorf("bare --json exit = %d, want 0", code)
	}
}
