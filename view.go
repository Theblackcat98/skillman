package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// View renders header / list+preview / status / footer + overlays.

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.renderHeader() + "\n")
	b.WriteString(m.renderMain() + "\n")
	b.WriteString(m.renderStatus() + "\n")
	b.WriteString(m.renderFooter())

	view := b.String()
	// Modals overlay the whole screen.
	switch m.appMode {
	case modeHelp:
		return m.renderHelpModal(view)
	case modeConfirm:
		return m.renderConfirmModal(view)
	}
	return view
}

func (m Model) renderHeader() string {
	n := len(m.filtered)
	total := len(m.skills)
	title := fmt.Sprintf("SkillMan  %d/%d skills", n, total)
	if m.loading && !m.noAnim {
		title += "  " + m.spinner.View() + " scanning…"
	} else if m.loading {
		title += "  … scanning…"
	}
	right := "?help  /search"
	if m.plain {
		line := title + "  " + right
		return truncate(line, m.width)
	}
	t := m.theme.Title.Render(title)
	r := m.theme.Dim.Render(right)
	// Pad to full width.
	gap := m.width - lipWidth(title) - lipWidth(right)
	if gap < 1 {
		gap = 1
	}
	return t + strings.Repeat(" ", gap) + r
}

func (m Model) renderMain() string {
	h := m.mainBoxH()
	if m.width < 70 {
		// Narrow: toggle between list and preview.
		if m.showPreviewOv {
			return m.renderPreviewBox(m.width-2, h)
		}
		return m.renderListBox(m.width-2, h)
	}
	lw := m.width/2 - 1
	rw := m.width - lw - 3
	return joinH(m.renderListBox(lw, h), m.renderPreviewBox(rw, h))
}

func (m Model) renderListBox(w, h int) string {
	active := !m.focusPreview
	title := "skills"
	if m.query != "" {
		title = fmt.Sprintf("skills  /%s", m.query)
	}
	var rows []string
	// box() renders h - 3 content lines (title + two borders); the old
	// h - 2 made the selected row unrenderable at the bottom edge (B2).
	vis := contentH(h)
	start := 0
	if m.cursor >= vis {
		start = m.cursor - vis + 1
	}
	end := start + vis
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	if len(m.filtered) == 0 {
		switch {
		case m.loading:
			rows = append(rows, "  scanning…")
		case len(m.skills) == 0:
			rows = append(rows, "  (no skills — r to rescan)")
		default:
			// A query is active and matched nothing: rescanning will
			// not help, so offer the filter escape hatch instead (B7).
			rows = append(rows, "  (no match for /"+m.query+")", "  esc clears the filter")
		}
	} else {
		for i := start; i < end; i++ {
			rows = append(rows, m.renderRow(i, w-4))
		}
	}
	for len(rows) < vis {
		rows = append(rows, "")
	}
	body := strings.Join(rows, "\n")
	return box(title, body, w, h, active, m.theme, m.plain)
}

func (m Model) renderRow(i, w int) string {
	s := m.filtered[i]
	badge, level := s.Badge()
	marker := "  "
	if i == m.cursor {
		marker = "> "
	}
	// Priority layout: marker + name + badge must always fit; the
	// category column only appears when room remains. The old fixed
	// layout truncated the badge away entirely at 70-97 cols, leaving
	// status conveyed by color alone (audit B1).
	const nameCap, badgeCol = 22, 6
	nameW := w - 2 - 1 - badgeCol - 1 // marker, gap, badge column, gap
	if nameW > nameCap {
		nameW = nameCap
	}
	if nameW < 1 {
		nameW = 1
	}
	line := marker + padRight(truncRunes(s.Name, nameW), nameW) + " " + padRight("["+badge+"]", badgeCol)
	if s.Category != "" {
		budget := w - lipWidth(line) - 1
		if budget > 12 {
			budget = 12
		}
		if budget >= 4 {
			line += " " + truncRunes(s.Category, budget)
		}
	}
	line = truncate(line, w)
	if i != m.cursor {
		if m.plain {
			return line
		}
		switch level {
		case "err":
			return m.theme.Error.Render(line)
		case "warn":
			return m.theme.Warning.Render(line)
		default:
			return line
		}
	}
	if m.plain {
		return line
	}
	return m.theme.Selected.Render(padRight(line, w))
}

func (m Model) renderPreviewBox(w, h int) string {
	active := m.focusPreview
	sel := m.selected()
	title := "preview"
	if sel != nil {
		title = "preview · " + truncRunes(sel.Name, w-14)
	}
	m.preview.Width = w - 4
	// Height is managed in sizePanes; just render viewport view clipped.
	content := m.preview.View()
	return box(title, content, w, h, active, m.theme, m.plain)
}

func (m Model) renderStatus() string {
	left := ""
	if m.toast != "" {
		left = m.toast
	} else if m.errMsg != "" {
		left = "error: " + m.errMsg
	} else if m.loading {
		left = "working…"
	} else {
		sel := m.selected()
		switch {
		case sel != nil:
			left = sel.Name + " · " + truncRunes(sel.Desc, m.width-30)
		case m.query != "":
			// Never claim "ready" while a filter is hiding everything (B15).
			left = "no match for /" + m.query
		default:
			left = "ready"
		}
	}
	mod := "normal"
	switch m.appMode {
	case modeFilter:
		// The input's own "/" prompt is the mode indicator.
		mod = m.filter.View()
	case modeCommand:
		mod = m.cmdline.View()
	case modeConfirm:
		mod = "confirm"
	case modeHelp:
		mod = "help"
	}
	if m.focusPreview {
		mod += " · preview-focus"
	}
	line := left + "  |  " + mod
	line = truncate(line, m.width)
	if m.plain {
		return line
	}
	if m.toastErr || m.errMsg != "" {
		return m.theme.Error.Render(line)
	}
	if m.toast != "" {
		return m.theme.Toast.Render(padRight(line, m.width))
	}
	return m.theme.Status.Render(line)
}

func (m Model) renderFooter() string {
	var hints string
	switch m.appMode {
	case modeFilter:
		hints = "enter keep · esc back"
	case modeCommand:
		hints = "enter run · esc cancel · try: validate, reload, clear"
	case modeConfirm:
		hints = "y confirm · n cancel"
	default:
		// Must fit 80 cols with `? help · q quit` intact (B9); the
		// full keymap lives in the ? overlay.
		if m.width < 70 {
			hints = "j/k · / find · : cmd · ? help · q quit"
		} else {
			hints = "j/k move · / filter · : cmd · e edit · d del · u undo · ? help · q quit"
		}
	}
	hints = truncate(hints, m.width)
	if m.plain {
		return hints
	}
	return m.theme.Footer.Render(hints)
}

func (m Model) renderHelpModal(under string) string {
	body := strings.Join([]string{
		"j/k, arrows   move selection",
		"g / G         top / bottom",
		"PgUp/PgDn     page (Ctrl-B / Ctrl-F)",
		"Tab / Enter   switch pane / focus preview",
		"/             filter (Esc keeps query, clear with Esc Esc)",
		":             command: edit delete validate reload clear quit",
		"e             open SKILL.md in $EDITOR",
		"d / u         delete to trash / undo (30s)",
		"v / r         validate all / rescan",
		"Esc           back one layer · q quits from base",
		"",
		"Respects NO_COLOR, TERM=dumb, NO_ANIMATIONS, REDUCED_MOTION, CI.",
		"press Esc, ?, or q to close",
	}, "\n")
	w := m.width - 10
	if w > 60 {
		w = 60
	}
	if w < 30 {
		w = m.width - 4
	}
	h := 20
	if h > m.height-4 {
		h = m.height - 4
	}
	return overlay(under, box("help · ?", body, w, h, true, m.theme, m.plain), m.width, m.height)
}

func (m Model) renderConfirmModal(under string) string {
	name := ""
	if sel := m.selected(); sel != nil {
		name = sel.Name
	}
	body := fmt.Sprintf("Delete skill %q?\nMoves to trash; undo with u within 30s.\n\ny confirm · n cancel", name)
	w := 52
	if w > m.width-4 {
		w = m.width - 4
	}
	return overlay(under, box("confirm delete", body, w, 9, true, m.theme, m.plain), m.width, m.height)
}

// --- small layout helpers (no deps beyond theme) ---

func box(title, body string, w, h int, active bool, th Theme, plain bool) string {
	if w < 4 {
		w = 4
	}
	lines := strings.Split(body, "\n")
	inner := w - 2
	var b strings.Builder
	top := "+" + strings.Repeat("-", inner) + "+"
	if !plain {
		top = th.Border.Render(top)
	}
	ttl := truncate(" "+title+" ", inner)
	if !plain {
		if active {
			ttl = th.Active.Render(ttl)
		} else {
			ttl = th.Dim.Render(ttl)
		}
	}
	b.WriteString(ttl + "\n")
	b.WriteString(top + "\n")
	vis := h - 3
	if vis < 0 {
		vis = 0
	}
	for i := 0; i < vis; i++ {
		var ln string
		if i < len(lines) {
			ln = truncate(lines[i], inner)
		}
		ln = padRight(ln, inner)
		edge := "|"
		if !plain {
			edge = th.Border.Render("|")
		}
		b.WriteString(edge + ln + edge + "\n")
	}
	bot := "+" + strings.Repeat("-", inner) + "+"
	if !plain {
		bot = th.Border.Render(bot)
	}
	b.WriteString(bot)
	return strings.TrimRight(b.String(), "\n")
}

func joinH(left, right string) string {
	l := strings.Split(left, "\n")
	r := strings.Split(right, "\n")
	n := len(l)
	if len(r) > n {
		n = len(r)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		var a, c string
		if i < len(l) {
			a = l[i]
		}
		if i < len(r) {
			c = r[i]
		}
		b.WriteString(a + " " + c)
		if i < n-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func overlay(under, modal string, w, h int) string {
	// Simple centered overlay: dim background by prefixing, then modal lines.
	ml := strings.Split(modal, "\n")
	mw := 0
	for _, ln := range ml {
		if lipWidth(ln) > mw {
			mw = lipWidth(ln)
		}
	}
	pad := (w - mw) / 2
	if pad < 0 {
		pad = 0
	}
	top := (h - len(ml)) / 2
	if top < 0 {
		top = 0
	}
	var b strings.Builder
	for i := 0; i < top; i++ {
		b.WriteString("\n")
	}
	for _, ln := range ml {
		b.WriteString(strings.Repeat(" ", pad) + ln + "\n")
	}
	return b.String() + "\n--- [Esc] close ---"
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipWidth(s) <= w {
		return s
	}
	if w <= 1 {
		return truncRunes(s, w)
	}
	return truncRunes(s, w-1) + "…"
}

func truncRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for idx := range s {
		if i == n {
			return s[:idx]
		}
		i++
	}
	return s
}

func padRight(s string, w int) string {
	d := w - lipWidth(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

func lipWidth(s string) int {
	// Visible width ignoring common ANSI escapes.
	stripped := stripANSI(s)
	return utf8.RuneCountInString(stripped)
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		if !inEsc {
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
				inEsc = true
				i++
				continue
			}
			b.WriteByte(s[i])
		} else {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEsc = false
			}
		}
	}
	_ = fmt.Sprint()
	return b.String()
}
