package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedEnv points skills and data dirs at throwaway locations.
func isolatedEnv(t *testing.T) (skills, data string) {
	t.Helper()
	skills = t.TempDir()
	data = t.TempDir()
	t.Setenv("SKILLMAN_SKILLS", skills)
	t.Setenv("XDG_DATA_HOME", data)
	return skills, data
}

func TestDeleteToTrashAndUndo(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))

	s := Skill{Name: "demo", Dir: dir}
	dest, err := deleteSkillToTrash(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("source dir still exists after delete")
	}
	if !strings.Contains(dest, trashDir()) {
		t.Errorf("dest %q not under trash dir %q", dest, trashDir())
	}

	if err := undoDelete("demo", dest); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("skill not restored: %v", err)
	}
}

// Regression: audit B14 — undo must refuse when the name was re-created.
func TestUndoRefusesWhenNameRecreated(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))

	s := Skill{Name: "demo", Dir: dir}
	dest, err := deleteSkillToTrash(s)
	if err != nil {
		t.Fatal(err)
	}
	writeSkill(t, skills, "demo", validFM("demo", "recreated"))

	err = undoDelete("demo", dest)
	if err == nil || !strings.Contains(err.Error(), "exists again") {
		t.Fatalf("want refusal error, got %v", err)
	}
	// Refused: copy must still be in trash.
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("refused entry missing from trash: %v", err)
	}
}

// Regression: audit B13 — two deletes in the same second must not collide.
func TestTrashDestinationsUnique(t *testing.T) {
	skills, _ := isolatedEnv(t)
	d1 := writeSkill(t, skills, "one", validFM("one", "d"))
	d2 := writeSkill(t, skills, "two", validFM("two", "d"))

	dest1, err := deleteSkillToTrash(Skill{Name: "one", Dir: d1})
	if err != nil {
		t.Fatal(err)
	}
	dest2, err := deleteSkillToTrash(Skill{Name: "two", Dir: d2})
	if err != nil {
		t.Fatal(err)
	}
	if dest1 == dest2 {
		t.Errorf("trash destinations collided: %q", dest1)
	}
	// A second delete of the same name in the same second must also
	// get a fresh path (name+ts would collide).
	base := filepath.Base(dest1)
	if err := os.MkdirAll(dest1, 0o755); err != nil {
		t.Fatal(err)
	}
	d3 := writeSkill(t, skills, "one", validFM("one", "d"))
	dest3, err := deleteSkillToTrash(Skill{Name: "one", Dir: d3})
	if err != nil {
		t.Fatalf("same-second re-delete collided: %v", err)
	}
	if filepath.Base(dest3) == base {
		t.Errorf("destination reused: %q", dest3)
	}
}

func TestCopyDirPreservesTreeAndModes(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "copy")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil || string(got) != "hello" {
		t.Errorf("file content: %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dst, "sub", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("exec bit lost: %v", info.Mode().Perm())
	}
}
