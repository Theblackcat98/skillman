package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// Golden frames.
//
// A substring assertion says "the badge is somewhere in the output".
// A golden frame says "this is exactly what the user sees", so a
// regression anywhere in the layout shows up as a diff instead of as a
// user bug report. The review's A2, E5, E6 and E7 were all invisible to
// strings.Contains and obvious here.
//
// Regenerate after an intentional layout change:
//
//	go test -run TestGoldenFrames -update
//
// The frames are checked in, so the diff is the review artefact.

var update = flag.Bool("update", false, "rewrite the golden frames")

// goldenSizes is the size table. The breakpoints are chosen because
// something changes at each of them: 20 is the clamp floor, 36 and 40
// are around the header-overflow width, 50 is a real phone terminal,
// 69 and 70 straddle the pane-split breakpoint, 80 is the reference
// frame, and 120 is a wide desktop.
var goldenSizes = [][2]int{
	{20, 10}, {36, 12}, {40, 24}, {50, 20}, {69, 24}, {70, 24}, {80, 24}, {120, 40},
}

func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}

// goldenModel is a deterministic model: no spinner, no clock, no
// environment. A golden frame is only a test if the same input always
// produces the same output.
func goldenModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := NewModel(false, true, Config{Accent: defaultAccent})
	send(t, &m, terminalSize(w, h))
	m.ready = true
	m.loading = false
	m.skills = makeSkills(20)
	m.skills[2].Category = "analysis"
	m.skills[2].License = "MIT"
	m.skills[2].Issues = []Issue{{Code: "test", Msg: "frontmatter name \"skill-03\" != dirname \"other\"", Sev: SevWarn}}
	m.skills[5].Desc = "A deliberately long description that has to be cut down to whatever the status bar can afford"
	m.applyFilter()
	m.cursor = 3
	m.refreshPreview()
	return m
}

func terminalSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

func TestGoldenFrames(t *testing.T) {
	for _, sz := range goldenSizes {
		for _, plain := range []bool{false, true} {
			name := fmt.Sprintf("golden-%dx%d-%s.golden", sz[0], sz[1], modeName(plain))
			t.Run(name, func(t *testing.T) {
				m := goldenModel(t, sz[0], sz[1])
				m.plain = plain
				m.theme = NewTheme(plain, defaultAccent)
				m.invalidatePreview()
				m.refreshPreview()
				frame := m.View()

				// The structural invariants hold whatever the golden
				// file says, so a regenerated file cannot bless a broken
				// frame.
				assertFrameShape(t, frame, sz[0], sz[1])
				checkGolden(t, name, frame)
			})
		}
	}
}

// assertFrameShape is the invariant set: exact size, valid UTF-8, no
// line wider than the terminal, and borders that line up.
func assertFrameShape(t *testing.T, frame string, w, h int) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if len(lines) != h {
		t.Errorf("rendered %d lines, want %d", len(lines), h)
		return
	}
	for i, ln := range lines {
		if got := lipWidth(ln); got > w {
			t.Errorf("line %d is %d cells, wider than %d: %q", i+1, got, w, ln)
		}
		if !utf8.ValidString(ln) {
			t.Errorf("line %d is not valid UTF-8: %q", i+1, ln)
		}
	}
	// The right-hand pane border must sit in the last column on every
	// box row, which is the only way to see that the frame is flush
	// rather than ragged by a cell. Status and footer lines are not part
	// of the box and are skipped.
	if w >= 70 {
		bare := strings.Split(stripANSI(frame), "\n")
		for i := 1; i < len(bare); i++ {
			row := bare[i]
			if row == "" || (row[0] != '|' && row[0] != '+') {
				continue
			}
			if got := lipWidth(row); got != w {
				t.Errorf("line %d is %d cells, want %d: %q", i+1, got, w, row)
			}
		}
	}
}

// TestGoldenFramesCoverOverlays locks the two modal frames too, since a
// modal that renders over a dimmed background is the one place the
// compositor can shift the frame by a row.
func TestGoldenFramesCoverOverlays(t *testing.T) {
	for _, mode := range []mode{modeHelp, modeConfirm} {
		name := fmt.Sprintf("golden-80x24-overlay-%d.golden", mode)
		t.Run(name, func(t *testing.T) {
			m := goldenModel(t, 80, 24)
			m.appMode = mode
			frame := m.View()
			assertFrameShape(t, frame, 80, 24)
			checkGolden(t, name, frame)
		})
	}
}

// TestGoldenFramesCoverNoSkills pins the empty states. A blank frame and
// a "(no skills — r to rescan)" frame are the same bytes to a substring
// test, and completely different to a user.
func TestGoldenFramesCoverEmptyStates(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) Model
	}{
		{"empty-dir", func(t *testing.T) Model {
			m := NewModel(false, true, Config{Accent: defaultAccent})
			// A fixed path, so the frame is byte-identical on every
			// machine: a golden that embeds the developer's home
			// directory fails in CI and passes locally.
			t.Setenv("SKILLMAN_SKILLS", "/skills")
			send(t, &m, terminalSize(80, 24))
			m.ready = true
			m.loading = false
			m.skills = nil
			m.applyFilter()
			m.refreshPreview()
			return m
		}},
		{"no-match", func(t *testing.T) Model {
			m := goldenModel(t, 80, 24)
			m.query = "nomatch"
			m.filter.SetValue("nomatch")
			m.applyFilter()
			return m
		}},
		{"preview-focused", func(t *testing.T) Model {
			m := goldenModel(t, 80, 24)
			m.focusPreview = true
			return m
		}},
	}
	for _, c := range cases {
		name := "golden-80x24-" + c.name + ".golden"
		t.Run(c.name, func(t *testing.T) {
			frame := c.build(t).View()
			assertFrameShape(t, frame, 80, 24)
			checkGolden(t, name, frame)
		})
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v\nrun: go test -run TestGoldenFrames -update", path, err)
	}
	if string(want) != got {
		t.Errorf("frame differs from %s\n%s", path, firstDiff(string(want), got))
	}
}

// firstDiff reports the first differing line, which is where the reader
// should look. A whole-frame dump would be unreadable in CI output.
func firstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("first difference at line %d:\n  golden: %q\n  actual: %q", i+1, w, g)
		}
	}
	return "identical lines, different bytes"
}

func modeName(plain bool) string {
	if plain {
		return "plain"
	}
	return "color"
}
