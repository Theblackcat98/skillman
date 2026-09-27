package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func validFM(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\nbody\n"
}

func TestParseFrontmatterValid(t *testing.T) {
	fm, body, err := parseFrontmatter("---\nname: x\ndescription: d\nlicense: MIT\n---\n\nhello\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "x" || fm.Description != "d" || fm.License != "MIT" {
		t.Errorf("parsed wrong: %+v", fm)
	}
	if !strings.Contains(body, "hello") {
		t.Errorf("body lost: %q", body)
	}
}

func TestParseFrontmatterCRLF(t *testing.T) {
	fm, _, err := parseFrontmatter("---\r\nname: win\r\ndescription: dos\r\n---\r\nbody\r\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "win" {
		t.Errorf("CRLF name = %q, want win", fm.Name)
	}
}

func TestParseFrontmatterInvalidYAML(t *testing.T) {
	_, _, err := parseFrontmatter("---\nname: [unclosed\n---\nbody\n")
	if err == nil {
		t.Fatal("want error for invalid YAML, got nil")
	}
}

func TestParseFrontmatterUnterminated(t *testing.T) {
	_, _, err := parseFrontmatter("---\nname: x\n")
	if err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("want unterminated error, got %v", err)
	}
}

func TestParseFrontmatterNone(t *testing.T) {
	fm, body, err := parseFrontmatter("just text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "" || body != "just text" {
		t.Errorf("fm=%+v body=%q", fm, body)
	}
}

func TestFilterSkills(t *testing.T) {
	skills := []Skill{
		{Name: "alpha-tool", Desc: "Does alpha things"},
		{Name: "beta-tool", Desc: "Does beta things", Category: "dev"},
	}
	if got := FilterSkills(skills, ""); len(got) != 2 {
		t.Errorf("empty query: got %d, want 2", len(got))
	}
	if got := FilterSkills(skills, "ALPH"); len(got) != 1 || got[0].Name != "alpha-tool" {
		t.Errorf("case-insensitive match failed: %v", got)
	}
	if got := FilterSkills(skills, "dev"); len(got) != 1 || got[0].Name != "beta-tool" {
		t.Errorf("category match failed: %v", got)
	}
	if got := FilterSkills(skills, "zzz"); len(got) != 0 {
		t.Errorf("want no matches, got %v", got)
	}
}

func TestBadge(t *testing.T) {
	cases := []struct {
		issues []string
		want   string
	}{
		{nil, "ok"},
		{[]string{"missing description"}, "warn"},
		{[]string{"missing SKILL.md"}, "err"},
	}
	for _, c := range cases {
		s := Skill{Issues: c.issues}
		got, _ := s.Badge()
		if got != c.want {
			t.Errorf("Badge(%v) = %q, want %q", c.issues, got, c.want)
		}
	}
}

func TestScanSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "good", validFM("good", "fine"))
	writeSkill(t, dir, "mismatch", validFM("other-name", "fine"))
	writeSkill(t, dir, "broken", "---\nname: [oops\n---\nbody\n")
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	skills, err := ScanSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 4 {
		t.Fatalf("got %d skills, want 4", len(skills))
	}
	byName := map[string]Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}
	if !byName["good"].Valid {
		t.Errorf("good should be valid: %v", byName["good"].Issues)
	}
	if byName["mismatch"].Valid {
		t.Errorf("name mismatch should be invalid")
	}
	if byName["broken"].Valid || !strings.Contains(strings.Join(byName["broken"].Issues, ";"), "invalid frontmatter") {
		t.Errorf("broken yaml not surfaced: %v", byName["broken"].Issues)
	}
	if byName["empty"].Valid || !strings.Contains(strings.Join(byName["empty"].Issues, ";"), "missing SKILL.md") {
		t.Errorf("missing SKILL.md not surfaced: %v", byName["empty"].Issues)
	}

	ok, warn, bad := summarize(skills)
	if ok != 1 || warn != 2 || bad != 1 {
		t.Errorf("summarize = %d/%d/%d, want 1/2/1", ok, warn, bad)
	}
}

func TestScanSkillsMissingDir(t *testing.T) {
	skills, err := ScanSkills(filepath.Join(t.TempDir(), "nope"))
	if err != nil || skills != nil {
		t.Errorf("want nil,nil for missing dir, got %v,%v", skills, err)
	}
}
