package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `go test -fuzz` needs a supported platform and refuses to run on
// android/arm64, so on Termux the fuzz targets in fuzz_test.go only
// execute their seed corpus. These tests do the same job with a seeded
// PRNG: a fixed seed means a failure is reproducible, and a few hundred
// thousand cases run in seconds without the fuzzer infrastructure.

func TestStressParseFrontmatter(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []string{
		"---", "--", "-", "\n", "\r\n", "name:", "description:", ":", " ",
		"a", "日本語", "🎉", "\t", "\x00", "\x07", "\x1b[2J", "\x9b",
		"\ufeff", "\u200b", "\u202e", "|", ">", "[", "]", "{", "}",
		"#", "*", "`", "'", `"`, "\\", "metadata:", "category:", "null", "0",
	}
	for i := 0; i < 200000; i++ {
		var b strings.Builder
		n := rng.Intn(24)
		for j := 0; j < n; j++ {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		raw := b.String()
		fm, body, err := parseFrontmatter(raw)
		_ = safeBody(body)
		if err != nil {
			_ = safeText(err.Error())
		}
		if err == nil && fm.Name != "" && !strings.Contains(raw, "name:") {
			t.Fatalf("a name appeared from nowhere for %q: %q", raw, fm.Name)
		}
	}
}

func TestStressSanitize(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 200000; i++ {
		n := rng.Intn(40)
		buf := make([]byte, n)
		for j := range buf {
			switch rng.Intn(4) {
			case 0:
				buf[j] = byte(rng.Intn(256)) // any byte, valid UTF-8 or not
			case 1:
				buf[j] = byte(rng.Intn(32)) // C0 controls
			case 2:
				buf[j] = byte(0xC0 + rng.Intn(64)) // UTF-8 lead/continuation
			default:
				buf[j] = "a日本語🎉"[rng.Intn(len("a日本語🎉"))]
			}
		}
		s := string(buf)
		text := safeText(s)
		if hasControlByte(text) {
			t.Fatalf("safeText leaked a control byte for %q: %q", s, text)
		}
		body := safeBody(s)
		if hasControlByte(body) {
			t.Fatalf("safeBody leaked a control byte for %q: %q", s, body)
		}
		if safeText(text) != text || safeBody(body) != body {
			t.Fatalf("sanitize is not idempotent for %q: %q -> %q", s, text, safeText(text))
		}
	}
}

func TestStressScan(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	names := []string{"a", "日本", "🎉", "with-dash", "with_underscore",
		"UPPER", "0digit", "long-" + strings.Repeat("x", 60), "dot.name"}
	contents := []string{
		"---\nname: x\ndescription: d\n---\nbody\n",
		"---\nname: x\n---\nbody\n",
		"---\ndescription: d\n---\nbody\n",
		"---\nmetadata:\n  category: c\n---\nbody\n",
		"---\nname: [bad\n---\nbody\n",
		"body only\n",
		"",
		"\x1b[2Jbody\x1b]0;x\x07\n",
		"---\nname: x\ndescription: |\n  folded\n  text\n---\nbody\n",
	}
	for i := 0; i < 1200; i++ {
		root := t.TempDir()
		count := 1 + rng.Intn(6)
		for j := 0; j < count; j++ {
			name := names[rng.Intn(len(names))]
			// Occasionally a name that is not filesystem-friendly.
			if rng.Intn(20) == 0 {
				name += "\t" + string(rune('a'+rng.Intn(26)))
			}
			d := filepath.Join(root, name)
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatalf("mkdir %q: %v", name, err)
			}
			content := contents[rng.Intn(len(contents))]
			if rng.Intn(3) == 0 {
				content = strings.Repeat(content, 1+rng.Intn(5))
			}
			if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0o644); err != nil {
				t.Fatalf("write %q: %v", name, err)
			}
		}
		skills, err := ScanSkills(root)
		if err != nil {
			continue
		}
		if len(skills) == 0 {
			t.Fatalf("iteration %d: %d skills written, none scanned", i, count)
		}
		for _, s := range skills {
			if hasControlByte(s.Name) {
				t.Fatalf("name has a control byte: %q", s.Name)
			}
			if hasControlByte(s.Desc) {
				t.Fatalf("desc has a control byte: %q", s.Desc)
			}
			if hasControlByte(s.Body) {
				t.Fatalf("body has a control byte: %q", s.Body)
			}
			badge, _ := s.Badge()
			if badge != "ok" && badge != "warn" && badge != "err" {
				t.Fatalf("badge = %q", badge)
			}
		}
		// The frame must survive whatever the tree contained.
		m := newTestModel(t, 80, 24, true)
		m.skills = skills
		m.applyFilter()
		frame := m.View()
		for _, ln := range strings.Split(frame, "\n") {
			if lipWidth(ln) > 80 {
				t.Fatalf("frame overflowed with a hostile tree: %q", stripANSI(ln))
			}
			if strings.Contains(ln, "\x1b") {
				t.Fatalf("plain frame has an escape with a hostile tree: %q", ln)
			}
		}
	}
}
