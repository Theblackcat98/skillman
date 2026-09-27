package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The editor path had no tests at all, including the $EDITOR argument
// splitting that review B10's fix added (review G5).

func TestEditorForPrefersVisualOverEditor(t *testing.T) {
	t.Setenv("VISUAL", "code -w")
	t.Setenv("EDITOR", "nano")
	got := editorFor()
	want := []string{"code", "-w"}
	if len(got) != len(want) {
		t.Fatalf("editorFor() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("editorFor()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEditorForFallsBackToEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "emacs -nw")
	got := editorFor()
	if len(got) != 2 || got[0] != "emacs" || got[1] != "-nw" {
		t.Errorf("editorFor() = %v, want [emacs -nw]", got)
	}
}

func TestEditorForDefaultsToVi(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	got := editorFor()
	if len(got) != 1 || got[0] != "vi" {
		t.Errorf("editorFor() = %v, want [vi]", got)
	}
	// Whitespace only is not a command.
	t.Setenv("EDITOR", "   \t  ")
	if got := editorFor(); len(got) != 1 || got[0] != "vi" {
		t.Errorf("editorFor() with blank EDITOR = %v, want [vi]", got)
	}
}

// The file being edited must be last on the command line, after the
// editor's own flags, or "code -w path" becomes "code path -w".
func TestEditorCmdPutsPathLast(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code -w --wait")
	cmd := editorCmd("/skills/demo/SKILL.md")
	args := cmd.Args
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4 elements", args)
	}
	if args[len(args)-1] != "/skills/demo/SKILL.md" {
		t.Errorf("path is not last: %v", args)
	}
}

func TestSkillEditPathPrefersSkillMD(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := Skill{Name: "demo", Dir: filepath.Join(dir, "demo")}
	// No SKILL.md yet: the directory is what is left, and the caller
	// has to say so rather than opening a file listing.
	if got := skillEditPath(s); got != s.Dir {
		t.Errorf("skillEditPath with no SKILL.md = %q, want the dir %q", got, s.Dir)
	}
	md := filepath.Join(s.Dir, "SKILL.md")
	if err := os.WriteFile(md, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := skillEditPath(s); got != md {
		t.Errorf("skillEditPath = %q, want %q", got, md)
	}
}

// A skill whose name came out of the sanitize boundary must still open
// the right file, because the sanitized name is not the on-disk name.
func TestSkillEditPathUsesTheOnDiskDirectory(t *testing.T) {
	root := t.TempDir()
	name := "evil\tTAB"
	d := filepath.Join(root, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: evil\ndescription: d\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := ScanSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]
	if strings.Contains(s.Name, "\t") {
		t.Errorf("the display name still has a tab: %q", s.Name)
	}
	// Dir is the real path, not the sanitized one, or editing and
	// deleting would target a file that does not exist.
	if got := skillEditPath(s); got != filepath.Join(d, "SKILL.md") {
		t.Errorf("skillEditPath = %q, want the real on-disk path", got)
	}
	if _, err := os.Stat(s.Dir); err != nil {
		t.Errorf("Dir does not exist on disk: %q (%v)", s.Dir, err)
	}
}
