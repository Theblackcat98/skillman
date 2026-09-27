package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func makeSkills(n int) []Skill {
	out := make([]Skill, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, Skill{
			Name: fmt.Sprintf("skill-%02d", i),
			Desc: fmt.Sprintf("Test skill number %02d", i),

			Body: "body",
			Dir:  "/nonexistent/" + fmt.Sprintf("skill-%02d", i),
		})
	}
	return out
}

// newTestModel builds a ready-to-render model at the given size.
//
// The size arrives through Update(WindowSizeMsg) rather than by
// assignment, so the clamps that clamp a tiny or huge terminal actually
// run. Assigning m.width directly is what let the header-overflow and
// 70-column-footer bugs hide from a suite that only tests 55 and 80
// columns (review G2).
func newTestModel(t *testing.T, width, height int, plain bool) Model {
	t.Helper()
	m := NewModel(plain, true, Config{Accent: defaultAccent})
	send(t, &m, tea.WindowSizeMsg{Width: width, Height: height})
	m.ready = true
	m.loading = false
	m.skills = makeSkills(20)
	m.applyFilter()
	m.sizePanes()
	return m
}

// assertLinesFit is the width invariant, and it counts display cells
// because runewidth is the whole point: a helper built on
// utf8.RuneCountInString reports PASS on a frame that is 92 cells wide
// (review G3). Valid UTF-8 is checked here too, since a frame that is
// not valid UTF-8 is corrupt in the same way.
func assertLinesFit(t *testing.T, view string, w int) {
	t.Helper()
	for i, ln := range strings.Split(view, "\n") {
		if got := lipWidth(ln); got > w {
			t.Errorf("line %d overflows: %d > %d cols: %q", i+1, got, w, ln)
		}
		if !utf8.ValidString(ln) {
			t.Errorf("line %d is not valid UTF-8: %q", i+1, ln)
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
	l := m.layout()
	if m.preview.Height != l.bodyH {
		t.Errorf("preview.Height=%d, want the layout bodyH=%d", m.preview.Height, l.bodyH)
	}
}

// Layout is the single owner of frame geometry. This is the guard against
// the duplication that produced audit B2: the box interior that box() draws
// must be exactly the interior the layout reports, the two panes plus the
// join gap must add up to the width, and the viewport must never be
// wrapped wider than the pane that clips it (review F3, A12).
func TestLayoutAgreesWithBox(t *testing.T) {
	for w := 20; w <= 200; w++ {
		m := newTestModel(t, w, 24, false)
		l := m.layout()
		if got := innerOf(l.listW); got != l.listInner {
			t.Errorf("w=%d listInner=%d, innerOf=%d", w, l.listInner, got)
		}
		if l.narrow {
			if l.listW != l.width || l.prevW != l.width {
				t.Errorf("w=%d narrow panes must both be full width, got %d/%d", w, l.listW, l.prevW)
			}
		} else if got := l.listW + 1 + l.prevW; got != l.width {
			t.Errorf("w=%d panes %d + gap + %d = %d, want %d", w, l.listW, l.prevW, got, l.width)
		}
		if l.previewWrap() > l.prevInner && l.prevInner >= 10 {
			t.Errorf("w=%d wraps at %d inside a %d cell pane; box() would clip it",
				w, l.previewWrap(), l.prevInner)
		}
		// The drawn box must be exactly the interior the layout claims.
		drawn := box("t", "x", l.listW, l.boxH, false, m.theme, false)
		first := strings.Split(drawn, "\n")[1] // the top border
		if got := lipWidth(first); got != l.listW {
			t.Errorf("w=%d list box row is %d cells, layout says %d", w, got, l.listW)
		}
	}
}

// Phase 10: the frame must use every row and never exceed the width,
// at every supported size, in both colour modes.
func TestFrameFillsTerminal(t *testing.T) {
	sizes := [][2]int{{20, 10}, {36, 12}, {50, 20}, {69, 24}, {70, 24}, {80, 24}, {120, 40}}
	for _, sz := range sizes {
		for _, plain := range []bool{false, true} {
			m := newTestModel(t, sz[0], sz[1], plain)
			m.cursor = 6
			m.refreshPreview()
			lines := strings.Split(m.View(), "\n")
			if len(lines) != sz[1] {
				t.Errorf("%dx%d plain=%v: rendered %d lines, want %d", sz[0], sz[1], plain, len(lines), sz[1])
			}
			assertLinesFit(t, m.View(), sz[0])
		}
	}
}

// Phase 10 / audit B16: the header was composed but never truncated, so
// a narrow terminal wrapped the whole frame.
func TestHeaderFitsNarrow(t *testing.T) {
	for _, w := range []int{20, 30, 36, 40, 55} {
		m := newTestModel(t, w, 24, false)
		head := strings.Split(m.View(), "\n")[0]
		if lipWidth(head) > w {
			t.Errorf("header %d cols at width %d: %q", lipWidth(head), w, head)
		}
	}
}

// Phase 10 / audit B9: the footer used a fixed 70-col breakpoint that
// cut "? help · q quit" away at exactly 70.
func TestFooterKeepsQuitHint(t *testing.T) {
	for _, w := range []int{20, 40, 60, 69, 70, 71, 80, 120} {
		m := newTestModel(t, w, 24, false)
		lines := strings.Split(m.View(), "\n")
		footer := lines[len(lines)-1]
		if !strings.Contains(footer, "q quit") {
			t.Errorf("footer lost q quit at width %d: %q", w, footer)
		}
	}
}

// Phase 10: the list title carries the scroll position.
func TestListTitleShowsPosition(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	m.cursor = 0
	m.applyFilter()
	if got := strings.Split(m.View(), "\n")[1]; !strings.Contains(got, "1–18/20") {
		t.Errorf("list title missing scroll position: %q", got)
	}
	m.cursor = 19
	m.refreshPreview()
	// 18 rows fit, so the last 18 of 20 are shown: 3–20.
	if got := strings.Split(m.View(), "\n")[1]; !strings.Contains(got, "3–20/20") {
		t.Errorf("list title wrong position at bottom: %q", got)
	}
}

// Phase 10: the active query must not be printed twice.
func TestFilterPromptNotDuplicated(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	m.query = "skill-0"
	m.filter.SetValue("skill-0")
	m.applyFilter()
	lines := strings.Split(m.View(), "\n")
	status := lines[len(lines)-2]
	if n := strings.Count(status, "/skill-0"); n != 1 {
		t.Errorf("query printed %d times in status line: %q", n, status)
	}
	if strings.Contains(lines[1], "/skill-0") {
		t.Errorf("query duplicated in list title: %q", lines[1])
	}
}

// Phase 10: the preview banner carries metadata and validation issues.
func TestPreviewBanner(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	m.skills[0].License = "MIT"
	m.skills[0].Compat = "any"
	m.skills[0].Category = "analysis"
	m.skills[0].Issues = []Issue{{Code: "test", Msg: "missing description", Sev: SevWarn}}
	m.invalidatePreview()
	m.applyFilter()
	view := m.View()
	for _, want := range []string{"skill-01", "license: MIT", "compat: any", "analysis", "missing description"} {
		if !strings.Contains(view, want) {
			t.Errorf("preview banner missing %q", want)
		}
	}
}

// Phase 10 / audit B11: modals composite over a dimmed background, keep
// the frame size, and drop the contradictory "--- [Esc] close ---" line.
func TestOverlayComposites(t *testing.T) {
	for _, mode := range []mode{modeHelp, modeConfirm} {
		for _, sz := range [][2]int{{50, 20}, {80, 24}} {
			m := newTestModel(t, sz[0], sz[1], false)
			m.appMode = mode
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) != sz[1] {
				t.Errorf("mode %d at %dx%d: %d lines, want %d", mode, sz[0], sz[1], len(lines), sz[1])
			}
			assertLinesFit(t, view, sz[0])
			if strings.Contains(view, "--- [Esc] close ---") {
				t.Errorf("mode %d still appends the close line (B11)", mode)
			}
			// The background stays on screen behind a modal that fits.
			// A modal as tall as the terminal covers it by definition.
			modalH := len(m.helpLines()) + 3
			if mode == modeConfirm {
				modalH = 6
			}
			if modalH < sz[1] && !strings.Contains(view, "SkillMan") {
				t.Errorf("mode %d blanked the background (B11)", mode)
			}
		}
	}
}

// Phase 10: the help key table is complete and nothing is cut at 80x24.
func TestHelpKeyTableComplete(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	m.openHelp()
	view := stripANSI(m.View())
	for _, want := range []string{"move selection down", "move selection up", "first skill",
		"last skill", "switch pane", "H / ?", "filter", "command: edit",
		"$EDITOR", "delete to trash", "undo a delete", "validate all",
		"rescan", "quit from the base layer", "back one layer", "NO_COLOR"} {
		if !strings.Contains(view, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
}

// Phase 10: a long body must reach its last line, and a resize must not
// throw the reader back to the top.
func TestPreviewScrollsToEndAndSurvivesResize(t *testing.T) {
	m := longBodyModel(t, 80, 24)
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // focus preview
	send(t, &m, keyPress('G'))
	if !m.preview.AtBottom() {
		t.Fatalf("G did not reach the end of the body: offset %d", m.preview.YOffset)
	}
	if !strings.Contains(m.preview.View(), "line 120") {
		t.Errorf("last body line not visible after G: %q", m.preview.View())
	}
	before := m.preview.YOffset
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // back to the list, then resize
	send(t, &m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.preview.YOffset == 0 && before != 0 {
		t.Errorf("resize reset preview scroll %d -> %d", before, m.preview.YOffset)
	}
	if !m.preview.AtBottom() {
		t.Errorf("resize lost the end of the body: offset %d", m.preview.YOffset)
	}
}

// Phase 10 / audit A1: width must be counted in terminal cells. Counting
// runes pushed the box borders off the right edge for CJK and emoji
// names, which destroyed the whole frame.
func TestWideCharacterRowsKeepFrame(t *testing.T) {
	names := []string{"日本語スキル名", "skill🎉🎉🎉", "Ωmega-skill", "a"}
	for _, size := range [][2]int{{80, 24}, {50, 20}, {20, 10}} {
		for _, plain := range []bool{false, true} {
			m := newTestModel(t, size[0], size[1], plain)
			m.skills = makeSkills(4)
			for i := range m.skills {
				m.skills[i].Name = names[i]
				m.skills[i].Category = "日本語"
			}
			m.invalidatePreview()
			m.applyFilter()
			for _, mode := range []mode{modeNormal, modeHelp, modeConfirm} {
				m.appMode = mode
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) != size[1] {
					t.Errorf("%dx%d mode %d: %d lines, want %d", size[0], size[1], mode, len(lines), size[1])
				}
				for i, ln := range lines {
					if lipWidth(ln) > size[0] {
						t.Errorf("%dx%d mode %d line %d: %d cells > %d: %q",
							size[0], size[1], mode, i+1, lipWidth(ln), size[0], stripANSI(ln))
					}
					if !utf8.ValidString(ln) {
						t.Errorf("%dx%d mode %d line %d: invalid UTF-8 %q",
							size[0], size[1], mode, i+1, ln)
					}
				}
			}
		}
	}
}

// Cut cells must never land inside an escape sequence.
func TestTruncateKeepsEscapesIntact(t *testing.T) {
	styled := "\x1b[1;38;5;189mhello world\x1b[0m"
	got := truncate(styled, 8)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("no ellipsis: %q", got)
	}
	if strings.Count(got, "\x1b")%2 != 0 {
		t.Errorf("escape sequence split: %q", got)
	}
	if lipWidth(got) > 8 {
		t.Errorf("width %d > 8: %q", lipWidth(got), got)
	}
}

func TestSkipEscapeForms(t *testing.T) {
	cases := map[string]int{
		"\x1b[31m":           5, // CSI
		"\x1b[1;38;5;1m":     11,
		"\x1b]0;title\x07":   10, // OSC with BEL
		"\x1b]8;;http\x1b\\": 11, // OSC with ST
		"\x1bM":              2,  // two-character
	}
	for seq, want := range cases {
		if got := skipEscape(seq, 0); got != want {
			t.Errorf("skipEscape(%q) = %d, want %d", seq, got, want)
		}
		if stripANSI(seq) != "" {
			t.Errorf("stripANSI(%q) = %q", seq, stripANSI(seq))
		}
	}
}

func TestLipWidthCountsCells(t *testing.T) {
	cases := map[string]int{
		"abc":              3,
		"日本語":              6,
		"🎉":                2,
		"\x1b[1mab\x1b[0m": 2,
	}
	for s, want := range cases {
		if got := lipWidth(s); got != want {
			t.Errorf("lipWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

// longBodyModel is a model with one skill whose body has 120 lines.
func longBodyModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := NewModel(false, true, Config{Accent: defaultAccent})
	var body strings.Builder
	for i := 1; i <= 120; i++ {
		fmt.Fprintf(&body, "line %d of the long body\n", i)
	}
	m.skills = []Skill{{
		Name:   "longbody",
		Desc:   "A skill with a very long body for scroll tests",
		Body:   body.String(),
		Dir:    "/nonexistent/longbody",
		Issues: []Issue{{Code: "test", Msg: "missing description", Sev: SevWarn}},
	}}
	send(t, &m, tea.WindowSizeMsg{Width: w, Height: h})
	m.loading = false
	m.applyFilter()
	return m
}

func send(t *testing.T, m *Model, msg tea.Msg) {
	t.Helper()
	mm, _ := m.Update(msg)
	*m = mm.(Model)
}

func keyPress(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// The help keymap is taller than a short terminal. Before the viewport it
// was silently cut off with no scroll and no indication (review E2).
func TestHelpOverlayScrollsOnAShortTerminal(t *testing.T) {
	m := newTestModel(t, 36, 12, false)
	m.openHelp()
	total := m.helpVP.TotalLineCount()
	l := m.layout()
	if l.helpBodyH >= total {
		t.Skipf("the keymap fits in %d lines; nothing to scroll", l.helpBodyH)
	}
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "1–") {
		t.Fatalf("a scrollable help box does not say where it is:\n%s", frame)
	}
	if !strings.Contains(frame, fmt.Sprintf("/%d", total)) {
		t.Errorf("help title does not give the total line count:\n%s", frame)
	}
	// The key column is what must survive a narrow box: the descriptions
	// may be clipped, but "which key does what" cannot.
	for _, want := range []string{"j / down", "g / home", "G / end"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the visible part of the keymap is missing %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "quit from the base layer") {
		t.Errorf("the bottom of the keymap should not fit yet:\n%s", frame)
	}

	// j scrolls, and the title follows.
	mm, _ := m.Update(keyPress('j'))
	m = mm.(Model)
	if m.helpVP.YOffset == 0 {
		t.Error("j did not scroll the help overlay")
	}
	if !strings.Contains(stripANSI(m.View()), "2–") {
		t.Error("the help title did not follow the scroll position")
	}
	// Esc closes.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.appMode != modeNormal {
		t.Error("esc did not close the help overlay")
	}
}

// At 20 columns the old row gave the name 6 cells and every skill read
// "skill-", so the list was unidentifiable (review E1).
func TestRowsStayIdentifiableWhenNarrow(t *testing.T) {
	m := newTestModel(t, 20, 10, true)
	seen := map[string]bool{}
	for i := 0; i < len(m.filtered) && i < 8; i++ {
		row := stripANSI(m.renderRow(i, m.layout().row))
		if lipWidth(row) > m.layout().listInner {
			t.Errorf("row %d is %d cells, pane interior is %d: %q",
				i, lipWidth(row), m.layout().listInner, row)
		}
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(row), "> "))
		// Strip the trailing severity indicator to get back the name.
		name = strings.TrimRight(name, " !x")
		if lipWidth(name) < 7 {
			t.Errorf("row %d shows only %d cells of name %q, too few to identify the skill", i, lipWidth(name), name)
		}
		if seen[name] {
			t.Errorf("row %d repeats the name %q", i, name)
		}
		seen[name] = true
	}
}

// The severity word is a luxury; when the row cannot afford it, a one-cell
// mark takes over, and a mark always means a problem.
func TestNarrowRowsUseASeverityMark(t *testing.T) {
	m := newTestModel(t, 20, 10, true)
	r := m.layout().row
	if !r.mark || r.severity != 1 {
		t.Fatalf("a 20-cell pane should use a 1-cell mark, got mark=%v severity=%d", r.mark, r.severity)
	}
	if got := severityMark(SevErr); got != "x" {
		t.Errorf("error mark = %q, want x", got)
	}
	if got := severityMark(SevWarn); got != "!" {
		t.Errorf("warn mark = %q, want !", got)
	}
	if got := severityMark(SevOK); got != " " {
		t.Errorf("ok mark = %q, want a blank so a mark always means a problem", got)
	}
	// A wide pane keeps the word.
	wide := newTestModel(t, 80, 24, true).layout().row
	if wide.mark || wide.severity != rowBadgeW {
		t.Errorf("an 80-cell pane should keep the word badge, got mark=%v severity=%d", wide.mark, wide.severity)
	}
}

// The row budget must always add up: a plan that overflows is a frame
// that wraps, and one that wastes cells is a frame with dead space.
func TestRowPlanFitsItsPane(t *testing.T) {
	for inner := 10; inner <= 120; inner++ {
		r := planRow(inner)
		if r.name < 1 {
			t.Errorf("inner=%d leaves no room for a name", inner)
		}
		used := r.marker + r.name + rowSepW + r.severity
		if r.category > 0 {
			used += rowSepW + r.category
		}
		if used > inner {
			t.Errorf("inner=%d: plan uses %d cells (%+v)", inner, used, r)
		}
	}
}
