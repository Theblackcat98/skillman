package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// --- trash CLI backing functions ---

func TestListTrashRecoversOriginalName(t *testing.T) {
	skills, _ := isolatedEnv(t)
	// A skill name that itself contains dashes, to prove the timestamp
	// is what anchors the slug match.
	d1 := writeSkill(t, skills, "my-long-skill", validFM("my-long-skill", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "my-long-skill", Dir: d1}); err != nil {
		t.Fatal(err)
	}
	d2 := writeSkill(t, skills, "plain", validFM("plain", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "plain", Dir: d2}); err != nil {
		t.Fatal(err)
	}

	entries, err := listTrash()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	names := map[string]TrashEntry{}
	for _, e := range entries {
		names[e.Name] = e
	}
	for _, want := range []string{"my-long-skill", "plain"} {
		e, ok := names[want]
		if !ok {
			t.Fatalf("original name %q not recovered: %+v", want, entries)
		}
		if !trashSlug.MatchString(e.Slug) {
			t.Errorf("slug %q does not look like <name>-<ts>", e.Slug)
		}
		if e.Deleted.IsZero() {
			t.Errorf("%s has no timestamp", want)
		}
	}
}

func TestListTrashEmptyAndMissing(t *testing.T) {
	skills, _ := isolatedEnv(t)
	entries, err := listTrash()
	if err != nil || len(entries) != 0 {
		t.Errorf("missing trash dir: %v, %v", entries, err)
	}
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	dest, err := deleteSkillToTrash(Skill{Name: "demo", Dir: filepath.Join(skills, "demo")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dest); err != nil {
		t.Fatal(err)
	}
	entries, err = listTrash()
	if err != nil || len(entries) != 0 {
		t.Errorf("empty trash: %v, %v", entries, err)
	}
}

func TestRestoreFromTrashByNameAndSlug(t *testing.T) {
	skills, _ := isolatedEnv(t)
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "demo", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	// By the original name.
	name, err := restoreFromTrash("demo")
	if err != nil {
		t.Fatalf("restore by name: %v", err)
	}
	if name != "demo" {
		t.Errorf("restored %q", name)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("not restored: %v", err)
	}

	// By the trash directory name.
	dest2, err := deleteSkillToTrash(Skill{Name: "demo", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreFromTrash(filepath.Base(dest2)); err != nil {
		t.Fatalf("restore by slug: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("not restored by slug: %v", err)
	}
}

func TestRestoreFromTrashErrors(t *testing.T) {
	skills, _ := isolatedEnv(t)
	if _, err := restoreFromTrash("nope"); err == nil {
		t.Error("empty trash should error")
	}
	dir := writeSkill(t, skills, "demo", validFM("demo", "d"))
	dest, err := deleteSkillToTrash(Skill{Name: "demo", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreFromTrash("not-there"); err == nil {
		t.Error("unknown name should error")
	}
	// Name taken again: refuse, keep the copy in the trash.
	writeSkill(t, skills, "demo", validFM("demo", "new"))
	if _, err := restoreFromTrash("demo"); err == nil {
		t.Error("restore over an existing skill must refuse")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("refused entry must stay in the trash: %v", err)
	}
}

func TestPurgeTrashRespectsAge(t *testing.T) {
	skills, data := isolatedEnv(t)
	oldDir := writeSkill(t, skills, "old", validFM("old", "d"))
	oldDest, err := deleteSkillToTrash(Skill{Name: "old", Dir: oldDir})
	if err != nil {
		t.Fatal(err)
	}
	recentDir := writeSkill(t, skills, "recent", validFM("recent", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "recent", Dir: recentDir}); err != nil {
		t.Fatal(err)
	}
	// Backdate one entry so an age-based purge can see the difference.
	old := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(oldDest, old, old); err != nil {
		t.Skipf("cannot backdate: %v", err)
	}

	purged, err := purgeTrash(30 * 24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || !strings.HasPrefix(purged[0], "old-") {
		t.Errorf("want only the aged entry purged, got %v", purged)
	}
	if _, err := os.Stat(oldDest); !os.IsNotExist(err) {
		t.Error("aged entry still present")
	}
	entries, err := listTrash()
	if err != nil || len(entries) != 1 || entries[0].Name != "recent" {
		t.Errorf("trash should hold only the recent entry: %v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(data, "skillman", "trash")); err != nil {
		t.Errorf("trash dir itself must survive a purge: %v", err)
	}
}

func TestPurgeTrashZeroAgePurgesAll(t *testing.T) {
	skills, _ := isolatedEnv(t)
	for _, n := range []string{"one", "two"} {
		d := writeSkill(t, skills, n, validFM(n, "d"))
		if _, err := deleteSkillToTrash(Skill{Name: n, Dir: d}); err != nil {
			t.Fatal(err)
		}
	}
	purged, err := purgeTrash(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 2 {
		t.Errorf("purge all got %v", purged)
	}
}

// The trash holds whole skills, so it must not be world readable.
func TestTrashDirIsPrivate(t *testing.T) {
	skills, _ := isolatedEnv(t)
	d := writeSkill(t, skills, "demo", validFM("demo", "d"))
	if _, err := deleteSkillToTrash(Skill{Name: "demo", Dir: d}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(trashDir())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("trash dir mode %v is not private", perm)
	}
}
