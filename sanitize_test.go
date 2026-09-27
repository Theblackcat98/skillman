package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The untrusted-content boundary. Every assertion here exists because a
// skill file is third-party input that a terminal will act on.

func TestSafeTextEscapesControlBytes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"escape", "a\x1b[2Jb", `a\x1b[2Jb`},
		{"osc title", "x\x1b]0;PWNED\x07y", `x\x1b]0;PWNED\x07y`},
		{"osc 52 clipboard", "\x1b]52;c;Zm9v\x07", `\x1b]52;c;Zm9v\x07`},
		{"tab", "a\tb", `a\x09b`},
		{"carriage return", "a\rb", `a\x0db`},
		{"nul", "a\x00b", `a\x00b`},
		{"delete", "a\x7fb", `a\x7fb`},
		{"newline", "a\nb", `a\x0ab`},
		{"c1 csi", "a\u009bb", `a\u009bb`},
		{"c1 osc", "a\u009d0;x\u009cb", `a\u009d0;x\u009cb`},
		{"zero-width space", "a\u200bb", `a\u200bb`},
		{"bidi override", "a\u202egnp.exe", `a\u202egnp.exe`},
		{"bom", "a\ufeffb", `a\ufeffb`},
		{"plain is untouched", "plain text 123", "plain text 123"},
		{"cjk is untouched", "日本語スキル名", "日本語スキル名"},
		{"emoji is untouched", "party 🎉", "party 🎉"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := safeText(c.in); got != c.want {
				t.Errorf("safeText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSafeBodyKeepsLineStructure(t *testing.T) {
	in := "line one\n\tindented\n\x1b[2J\nlast"
	want := "line one\n\tindented\n\\x1b[2J\nlast"
	if got := safeBody(in); got != want {
		t.Errorf("safeBody(%q) = %q, want %q", in, got, want)
	}
}

func TestSafeReplacesInvalidUTF8(t *testing.T) {
	// A lone 0xff is not valid UTF-8. Left alone, runewidth and the
	// renderer disagree about the width and the frame overflows.
	in := "a\xffb"
	got := safeText(in)
	if strings.Contains(got, "\xff") {
		t.Errorf("safeText kept the invalid byte: %q", got)
	}
	if !strings.Contains(got, "\ufffd") {
		t.Errorf("safeText(%q) = %q, want a replacement rune", in, got)
	}
}

func TestSafeNameIsNeverEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\x1b\x07", "\t\n"} {
		if got := safeName(in); strings.TrimSpace(got) == "" {
			t.Errorf("safeName(%q) = %q, want a visible placeholder", in, got)
		}
	}
}

// No display string may contain a raw control byte after scanning. This
// is the property the review's C1 is really about.
func TestScanSanitizesEveryDisplayField(t *testing.T) {
	dir := t.TempDir()
	// A body with a screen clear, a cursor home and an OSC title set.
	body := "before\n\x1b[2J\x1b[1;1H CLEARED \x1b]0;PWNED\x07\nafter\n"
	// A dirname with a raw tab: not YAML, so nothing else would catch it.
	skillDir := filepath.Join(dir, "evil\tTAB")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: evil\ndescription: \"esc \\u001b[2J in the desc\"\n" +
		"license: \"MIT\\u0007\"\ncompatibility: \"\\u001b]0;x\\u0007\"\n" +
		"metadata:\n  category: \"cat\\u0007\"\n---\n" + body
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	skills, err := ScanSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]

	fields := map[string]string{
		"Name":     s.Name,
		"Desc":     s.Desc,
		"License":  s.License,
		"Compat":   s.Compat,
		"Category": s.Category,
		"Body":     s.Body,
	}
	for i, is := range s.Issues {
		fields["Issue["+itoaTest(i)+"]"] = is.Msg
	}
	for name, val := range fields {
		if hasControlByte(val) {
			t.Errorf("%s still carries a control byte: %q", name, val)
		}
	}
	// The tab in the dirname must be visible, not raw.
	if !strings.Contains(s.Name, `\x09`) {
		t.Errorf("Name = %q, want the tab shown as \\x09", s.Name)
	}
	// The body keeps its newlines: it is markdown.
	if strings.Count(s.Body, "\n") < 3 {
		t.Errorf("Body lost its line structure: %q", s.Body)
	}
	// And the escape is still legible for debugging.
	if !strings.Contains(s.Body, `\x1b[2J`) {
		t.Errorf("Body = %q, want the escape shown as \\x1b[2J", s.Body)
	}
}

func hasControlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 && c != '\n' && c != '\t' {
			return true
		}
		if c == 0x7f {
			return true
		}
		if c == 0x1b {
			return true
		}
	}
	return false
}

// A C0 byte other than LF is not valid UTF-8 text, so testing the string
// directly is enough. This helper also covers the DEL case.
func TestHasControlByteHelper(t *testing.T) {
	if !hasControlByte("a\x1bb") {
		t.Error("missed ESC")
	}
	if !hasControlByte("a\x07b") {
		t.Error("missed BEL")
	}
	if hasControlByte("a\nb\tc") {
		t.Error("false positive on newline and tab")
	}
	if hasControlByte("日本語") {
		t.Error("false positive on CJK")
	}
}

// The whole rendered frame, in plain mode, must be free of raw escapes
// when the only escapes present came from skill content. Plain mode adds
// no styling of its own, so any ESC here would be attacker-controlled.
func TestPlainFrameHasNoRawEscapes(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "evil")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: evil\ndescription: \"d \\u001b[2J\"\n" +
		"metadata:\n  category: \"c \\u0007\"\n---\n" +
		"# evil\n\nbody \x1b[2J \x1b]0;PWNED\x07 text\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, 80, 24, true)
	skills, err := ScanSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.skills = skills
	m.loading = false
	m.filtered = m.skills
	m.ready = true
	m.refreshPreview()

	if got := m.View(); strings.Contains(got, "\x1b") {
		t.Errorf("plain frame carries a raw escape:\n%q", got)
	}
}

// sanitize must be idempotent, or a second scan of an already-clean file
// would keep rewriting it.
func TestSanitizeIsIdempotent(t *testing.T) {
	in := "a\x1b[2J\tb\n\x07c"
	once := safeBody(in)
	twice := safeBody(once)
	if once != twice {
		t.Errorf("not idempotent:\n once: %q\ntwice: %q", once, twice)
	}
}
