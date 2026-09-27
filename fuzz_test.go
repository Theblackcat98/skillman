package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseFrontmatter is the one function that reads bytes written by
// someone else and decides what they mean, so it is the one worth
// fuzzing. The seeds are the shapes the review's edge-case list called
// out and the code got wrong: a UTF-8 BOM, CRLF line endings, a
// thematic break that starts with three dashes, an unterminated block,
// control bytes, and a description folded across lines.

func FuzzParseFrontmatter(f *testing.F) {
	seeds := []string{
		"---\nname: a\ndescription: b\n---\nbody\n",
		"---\r\nname: a\r\ndescription: b\r\n---\r\nbody\r\n",
		"\xef\xbb\xbf---\nname: a\ndescription: b\n---\nbody\n",
		"\xef\xbb\xbf---\nname: a\ndescription: b\n---\nbody",
		"----\nname: a\n---\nbody\n",
		"---abc\nname: a\n---\nbody\n",
		"---\nname: a\ndescription: b\nbody without a closing marker\n",
		"---\nname: a\ndescription: b\n---",
		"---\n\n---\n",
		"---\n---\n",
		"---\nname:\ndescription:\n---\n",
		"---\ndescription: |\n  folded\n  description\n---\nbody\n",
		"---\ndescription: \"tab\\there\"\n---\nbody\n",
		"---\nmetadata: [not, a, map]\n---\nbody\n",
		"---\nmetadata:\n  category: 1\n---\nbody\n",
		"---\nname: \x00\x07\x1b\ndescription: x\n---\nbody\n",
		"---\nname: 日本語\ndescription: 🎉\n---\nbody\n",
		"no frontmatter at all\n",
		"",
		"\n",
		"---\n",
		"--\nname: a\n---\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		fm, body, err := parseFrontmatter(raw)
		// The invariant that matters: whatever happens, nothing panics,
		// nothing loops, and the result is safe to print.
		_ = fm
		_ = safeBody(body)
		if err != nil {
			_ = safeText(err.Error())
		}
		// A successful parse must not invent content. If there was no
		// closing marker, the body is the whole input and there is no
		// error to report.
		if err == nil && fm.Name != "" && !strings.Contains(raw, "name:") {
			t.Errorf("a name appeared from nowhere: %q", fm.Name)
		}
	})
}

// A name may never be a control byte, whatever the input. This is the
// fuzz invariant with teeth: it is the boundary the terminal cares about.
func FuzzSanitizeNeverEmitsControlBytes(f *testing.F) {
	for _, s := range []string{
		"", "plain", "\x1b[2J", "\x1b]0;x\x07", "\x00\x01\x02",
		"日本語", "🎉", "\xff\xfe", "a\tb", "a\nb", "\x9b2J",
		"\u200b\u202e", strings.Repeat("x", 5000),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := safeText(s); hasControlByte(got) {
			t.Errorf("safeText(%q) = %q still has a control byte", s, got)
		}
		if got := safeBody(s); hasControlByte(got) {
			t.Errorf("safeBody(%q) = %q still has a control byte", s, got)
		}
		// Escaping must be idempotent, or a rescan rewrites the file
		// forever.
		if once, twice := safeBody(s), safeBody(safeBody(s)); once != twice {
			t.Errorf("not idempotent for %q: %q then %q", s, once, twice)
		}
	})
}

// A scan must survive a directory tree that is nothing like a skills
// directory: no SKILL.md, binary files, deep nesting, an empty name, a
// name that is all control bytes.
func FuzzScanSurvivesOddTrees(f *testing.F) {
	f.Add("plain/SKILL.md", "---\nname: plain\ndescription: d\n---\nbody\n")
	f.Add("nomd/README.md", "no skill file here\n")
	f.Add("empty/SKILL.md", "")
	f.Add("onlyfm/SKILL.md", "---\n")
	f.Add("binary/SKILL.md", "\x00\x01\x02\xff\xfe")
	f.Add("deep/a/b/c/SKILL.md", "---\nname: deep\ndescription: d\n---\nbody\n")
	for _, tc := range [][]string{
		{"plain/SKILL.md", "---\nname: plain\ndescription: d\n---\nbody\n"},
		{"nomd/README.md", "no skill file here\n"},
		{"empty/SKILL.md", ""},
		{"onlyfm/SKILL.md", "---\n"},
		{"binary/SKILL.md", "\x00\x01\x02\xff\xfe"},
		{"deep/a/b/c/SKILL.md", "---\nname: deep\ndescription: d\n---\nbody\n"},
		{"crlf/SKILL.md", "---\r\nname: crlf\r\ndescription: d\r\n---\r\nbody\r\n"},
	} {
		f.Add(tc[0], tc[1])
	}
	f.Fuzz(func(t *testing.T, rel, content string) {
		// A path from the fuzzer must not escape the temp dir.
		if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			t.Skip()
		}
		root := t.TempDir()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Skip()
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Skip()
		}
		skills, err := ScanSkills(root)
		if err != nil {
			return // a read error is a legal outcome
		}
		for _, s := range skills {
			if hasControlByte(s.Name) {
				t.Errorf("name has a control byte: %q", s.Name)
			}
			if hasControlByte(s.Desc) {
				t.Errorf("desc has a control byte: %q", s.Desc)
			}
			if hasControlByte(s.Body()) {
				t.Errorf("body has a control byte for %q", s.Name)
			}
			if strings.TrimSpace(s.Name) == "" {
				t.Errorf("a skill has an empty name: %+v", s)
			}
			// The badge must be derivable from the issue list alone.
			b := s.Badge()
			if b != "ok" && b != "warn" && b != "err" {
				t.Errorf("badge = %q, want ok, warn or err", b)
			}
		}
	})
}
