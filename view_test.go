package main

import (
	"fmt"
	"strings"
	"testing"
)

func makeSkills(n int) []Skill {
	out := make([]Skill, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, Skill{
			Name:  fmt.Sprintf("skill-%02d", i),
			Desc:  fmt.Sprintf("Test skill number %02d", i),
			Valid: true,
			Body:  "body",
			Dir:   "/nonexistent/" + fmt.Sprintf("skill-%02d", i),
		})
	}
	return out
}

// newTestModel builds a ready-to-render model at the given size.
func newTestModel(t *testing.T, width, height int, plain bool) Model {
	t.Helper()
	m := NewModel(plain, true)
	m.width, m.height = width, height
	m.ready = true
	m.loading = false
	m.skills = makeSkills(20)
	m.applyFilter()
	m.sizePanes()
	return m
}

func assertLinesFit(t *testing.T, view string, w int) {
	t.Helper()
	for i, ln := range strings.Split(view, "\n") {
		if lipWidth(ln) > w {
			t.Errorf("line %d overflows: %d > %d cols: %q", i+1, lipWidth(ln), w, ln)
		}
	}
}

func TestViewFits80x24(t *testing.T) {
	for _, plain := range []bool{false, true} {
		m := newTestModel(t, 80, 24, plain)
		assertLinesFit(t, m.View(), 80)
	}
}

func TestViewFitsNarrow(t *testing.T) {
	m := newTestModel(t, 55, 24, false)
	assertLinesFit(t, m.View(), 55)
	// Narrow Tab overlay too.
	m.showPreviewOv = true
	assertLinesFit(t, m.View(), 55)
}

// Regression: audit B1 — badge must be visible at 80 cols.
func TestBadgeVisibleAt80(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	if !strings.Contains(m.View(), "[ok]") {
		t.Error("status badge truncated from list rows at 80 cols (B1)")
	}
}

// Regression: audit B2 — selected row must render at the bottom edge.
func TestCursorRowVisibleAtBottom(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	m.cursor = len(m.filtered) - 1
	m.refreshPreview()
	view := m.View()
	if !strings.Contains(view, "> skill-20") {
		t.Error("selected row missing at bottom of list (B2)")
	}
}

// Regression: audit B9 — footer must fit 80 cols ending in q quit.
func TestFooterFits80(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	lines := strings.Split(m.View(), "\n")
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "q quit") {
		t.Errorf("footer lost q quit: %q", footer)
	}
	if lipWidth(footer) > 80 {
		t.Errorf("footer width %d > 80", lipWidth(footer))
	}
}

// Regression: audit B7 — no-match state must name the query and Esc.
func TestNoMatchState(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	m.query = "zzz"
	m.filter.SetValue("zzz")
	m.applyFilter()
	view := m.View()
	if !strings.Contains(view, "no match for /zzz") {
		t.Error("no-match state missing query (B7)")
	}
	if !strings.Contains(view, "esc clears the filter") {
		t.Error("no-match state missing Esc hint (B7)")
	}
}

func TestPlainViewHasNoANSI(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	if strings.Contains(m.View(), "\x1b") {
		t.Error("plain view contains escape sequences")
	}
}

func TestOverlaysFit(t *testing.T) {
	for _, mode := range []mode{modeHelp, modeConfirm} {
		m := newTestModel(t, 80, 24, false)
		m.appMode = mode
		assertLinesFit(t, m.View(), 80)
	}
	m := newTestModel(t, 55, 24, false)
	m.appMode = modeHelp
	assertLinesFit(t, m.View(), 55)
}

// Viewport height must match the box content area or the last preview
// lines can never scroll into view.
func TestPreviewHeightMatchesBox(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	if m.preview.Height != contentH(m.mainBoxH()) {
		t.Errorf("preview.Height=%d, want contentH(mainBoxH())=%d", m.preview.Height, contentH(m.mainBoxH()))
	}
}
