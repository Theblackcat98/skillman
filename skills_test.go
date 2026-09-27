package main

import (
	"errors"
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

// The badge, the severity, Valid and the exit code all read one value.
// The old code decided severity by asking whether an issue *message*
// started with "missing SKILL.md", so rewording that string silently
// changed exit codes (review A4).
func TestBadgeAndSeverity(t *testing.T) {
	cases := []struct {
		name   string
		issues []Issue
		want   Severity
	}{
		{"clean", nil, SevOK},
		{"warn", []Issue{{Code: CodeDescMissing, Msg: "missing description", Sev: SevWarn}}, SevWarn},
		{"err", []Issue{{Code: CodeFileMissing, Msg: "missing SKILL.md", Sev: SevErr}}, SevErr},
		{"worst wins", []Issue{
			{Code: CodeDescMissing, Msg: "missing description", Sev: SevWarn},
			{Code: CodeFileMissing, Msg: "missing SKILL.md", Sev: SevErr},
			{Code: CodeNameMissing, Msg: "missing name", Sev: SevWarn},
		}, SevErr},
		// The order must not matter, which is the whole point.
		{"worst first", []Issue{
			{Code: CodeFileMissing, Msg: "missing SKILL.md", Sev: SevErr},
			{Code: CodeDescMissing, Msg: "missing description", Sev: SevWarn},
		}, SevErr},
	}
	for _, c := range cases {
		s := Skill{Issues: c.issues}
		if got := s.Severity(); got != c.want {
			t.Errorf("%s: Severity() = %v, want %v", c.name, got, c.want)
		}
		if got := s.Badge(); got != c.want.String() {
			t.Errorf("%s: Badge() = %q, want %q", c.name, got, c.want)
		}
		if got := s.Valid(); got != (c.want == SevOK) {
			t.Errorf("%s: Valid() = %v", c.name, got)
		}
	}
}

// Renaming an issue message must not change any exit code. That
// coupling was the defect.
func TestIssueMessagesDoNotDriveSeverity(t *testing.T) {
	// A reworded "missing SKILL.md" still has to be an error, because
	// the Code says so.
	s := Skill{Issues: []Issue{{
		Code: CodeFileMissing,
		Msg:  "no SKILL.md in this directory",
		Sev:  SevErr,
	}}}
	if s.Severity() != SevErr {
		t.Error("severity follows the message text instead of the code")
	}
	if got := ValidateExitCode([]Skill{s}); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
	// And a message that merely *contains* those words must not.
	other := Skill{Issues: []Issue{{
		Code: CodeNameMissing,
		Msg:  "missing SKILL.md-ish name",
		Sev:  SevWarn,
	}}}
	if got := ValidateExitCode([]Skill{other}); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
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
	if !byName["good"].Valid() {
		t.Errorf("good should be valid: %v", byName["good"].Issues)
	}
	if byName["mismatch"].Valid() {
		t.Errorf("name mismatch should be invalid")
	}
	if byName["broken"].Valid() || !strings.Contains(strings.Join(byName["broken"].issueTexts(), ";"), "invalid frontmatter") {
		t.Errorf("broken yaml not surfaced: %v", byName["broken"].Issues)
	}
	if byName["empty"].Valid() || !strings.Contains(strings.Join(byName["empty"].issueTexts(), ";"), "missing SKILL.md") {
		t.Errorf("missing SKILL.md not surfaced: %v", byName["empty"].Issues)
	}

	// A malformed SKILL.md is one error, not three warnings. The old
	// code reported "invalid frontmatter" *and* "missing name" *and*
	// "missing description" for the same file, because the
	// zero-valued frontmatter had empty fields (review A6), and the
	// badge was then a warning for a file nothing can read.
	broken := byName["broken"]
	if len(broken.Issues) != 1 {
		t.Errorf("malformed frontmatter produced %d issues %v, want exactly 1",
			len(broken.Issues), broken.issueTexts())
	}
	if broken.Issues[0].Code != CodeFMMalformed {
		t.Errorf("code = %q, want %q", broken.Issues[0].Code, CodeFMMalformed)
	}
	if broken.Severity() != SevErr {
		t.Errorf("malformed frontmatter severity = %v, want err", broken.Severity())
	}
	if !strings.Contains(broken.Issues[0].Msg, "invalid frontmatter") {
		t.Errorf("message = %q, want it to say the frontmatter is invalid", broken.Issues[0].Msg)
	}

	// A skill with no SKILL.md is the only thing that maps to exit 2.
	empty := byName["empty"]
	if len(empty.Issues) != 1 || empty.Issues[0].Code != CodeFileMissing {
		t.Errorf("empty dir issues = %v, want one %s", empty.issueTexts(), CodeFileMissing)
	}
	if got := ValidateExitCode([]Skill{byName["good"], byName["mismatch"]}); got != 1 {
		t.Errorf("warn-only set = %d, want 1", got)
	}
	if got := ValidateExitCode([]Skill{byName["good"], empty}); got != 2 {
		t.Errorf("set with a missing SKILL.md = %d, want 2", got)
	}
	if got := ValidateExitCode([]Skill{byName["good"]}); got != 0 {
		t.Errorf("clean set = %d, want 0", got)
	}

	ok, warn, bad := summarize(skills)
	if ok != 1 || warn != 1 || bad != 2 {
		t.Errorf("summarize = %d/%d/%d, want 1/1/2", ok, warn, bad)
	}
}

// A missing skills directory is not an error. The assertion is on the
// behaviour a caller needs — an empty list it can render — not on the
// specific nil, nil the implementation happens to return, which is what
// the old version of this test froze (review G8).
func TestScanSkillsMissingDirIsAnEmptyList(t *testing.T) {
	skills, err := ScanSkills(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Errorf("a missing skills dir returned an error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("a missing skills dir returned %d skills, want 0", len(skills))
	}
	// And it renders as the empty state, not a crash.
	m := newTestModel(t, 80, 24, true)
	m.skills = skills
	m.loading = false
	m.applyFilter()
	if !strings.Contains(m.View(), "no skills") {
		t.Error("a missing skills dir does not render the empty state")
	}
}

// Every issue carries a stable code and a severity, and the worst
// severity over a set is what the exit code is built from. Order never
// matters (review A3, A4).
func TestWorstSeverityIsOrderIndependent(t *testing.T) {
	warn := Skill{Name: "aaa", Issues: []Issue{
		{Code: CodeNameMissing, Msg: "missing name", Sev: SevWarn},
	}}
	errSkill := Skill{Name: "zzz", Issues: []Issue{
		{Code: CodeFileMissing, Msg: "missing SKILL.md", Sev: SevErr},
	}}
	for _, set := range [][]Skill{
		{warn, errSkill},
		{errSkill, warn},
	} {
		if got := WorstSeverity(set); got != SevErr {
			t.Errorf("WorstSeverity(%v) = %v, want err", skillNames(set), got)
		}
		if got := ValidateExitCode(set); got != 2 {
			t.Errorf("ValidateExitCode(%v) = %d, want 2", skillNames(set), got)
		}
	}
	// A malformed frontmatter is an error, not a warning, so a set with
	// one is exit 2 even though nothing is missing a file.
	broken := Skill{Name: "broken", Issues: []Issue{
		{Code: CodeFMMalformed, Msg: "invalid frontmatter: …", Sev: SevErr},
	}}
	if got := ValidateExitCode([]Skill{broken}); got != 2 {
		t.Errorf("malformed frontmatter = %d, want 2", got)
	}
	// Warnings only is 1.
	if got := ValidateExitCode([]Skill{warn}); got != 1 {
		t.Errorf("warn only = %d, want 1", got)
	}
	// Clean is 0.
	if got := ValidateExitCode(nil); got != 0 {
		t.Errorf("empty set = %d, want 0", got)
	}
}

func TestSeverityString(t *testing.T) {
	if SevOK.String() != "ok" || SevWarn.String() != "warn" || SevErr.String() != "err" {
		t.Errorf("severity words = %q/%q/%q", SevOK, SevWarn, SevErr)
	}
}

// A failed scan must not empty the list and must not claim success.
// The old delete and undo paths called ScanSkills inline and discarded
// the error, so a transient read failure produced an empty list while
// the status bar still said the skill was deleted (review B2).
func TestScanFailureKeepsTheListAndSaysSo(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	before := len(m.skills)
	if before == 0 {
		t.Fatal("setup: no skills")
	}
	mm, cmd := m.Update(skillsLoadedMsg{err: errors.New("permission denied")})
	m = mm.(Model)
	if len(m.skills) != before {
		t.Errorf("a failed scan left %d skills, want the previous %d", len(m.skills), before)
	}
	if m.loading {
		t.Error("the model is still loading after a failed scan")
	}
	if m.errMsg == "" || !strings.Contains(m.errMsg, "permission denied") {
		t.Errorf("errMsg = %q, want the scan error", m.errMsg)
	}
	if cmd == nil {
		t.Fatal("a failed scan scheduled no dismissal for its error toast")
	}
	if !m.toastErr || !strings.Contains(m.toast, "scan failed") {
		t.Errorf("toast = %q (err=%v), want an error toast saying the scan failed", m.toast, m.toastErr)
	}
	// And the frame says so rather than looking ready.
	if !strings.Contains(m.View(), "error") && !strings.Contains(m.toast, "scan failed") {
		t.Errorf("the frame hides a failed scan: %q", m.View())
	}

	// A later good scan clears the error.
	mm, _ = m.Update(skillsLoadedMsg{skills: makeSkills(3)})
	m = mm.(Model)
	if m.errMsg != "" {
		t.Errorf("errMsg = %q after a good scan, want it cleared", m.errMsg)
	}
}
