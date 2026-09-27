package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
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
	case modeConfirmCreate:
		return m.renderCreateModal(view)
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
	// Compose, then truncate, then style. Truncating a styled string can
	// cut an escape sequence in half and leak raw bytes to the terminal.
	gap := m.width - lipWidth(title) - lipWidth(right)
	if gap < 1 {
		gap = 1
	}
	line := truncate(title+strings.Repeat(" ", gap)+right, m.width)
	if m.plain {
		return line
	}
	// Style the two halves separately so truncation cannot cut an escape
	// sequence. Split on runes, not bytes: truncation shortens the string
	// and a byte offset can land inside a multi-byte character.
	head := []rune(title)
	all := []rune(line)
	if len(all) <= len(head) {
		return m.theme.Title.Render(line)
	}
	return m.theme.Title.Render(string(all[:len(head)])) + m.theme.Dim.Render(string(all[len(head):]))
}

func (m Model) renderMain() string {
	h := m.mainBoxH()
	listW, prevW := m.paneW()
	if m.width < 70 {
		// Narrow: toggle between list and preview.
		if m.showPreviewOv {
			return m.renderPreviewBox(listW, h)
		}
		return m.renderListBox(listW, h)
	}
	return joinH(m.renderListBox(listW, h), m.renderPreviewBox(prevW, h))
}

func (m Model) renderListBox(w, h int) string {
	active := !m.focusPreview
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
	// Scroll position in the title: with more skills than rows, "which
	// of the 40 am I looking at" is otherwise unknowable.
	title := "skills"
	if len(m.filtered) > 0 {
		title = fmt.Sprintf("skills  %d–%d/%d", start+1, end, len(m.filtered))
	}
	var rows []string
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
		// "preview · " is 10 cells; box() trims the rest to the interior.
		title = "preview · " + truncRunes(sel.Name, w-12)
	}
	// The viewport is sized in sizePanes from paneW(); assigning
	// m.preview.Width here wrote to a value copy and did nothing.
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
			left = sel.Name
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
	case modeConfirmCreate:
		mod = "confirm create"
	case modeHelp:
		mod = "help"
	}
	if m.focusPreview {
		mod += " · preview-focus"
	}
	// The active query is shown here, not in the list title: both
	// showing it made the status line prompt twice (audit B15).
	if m.query != "" && m.appMode != modeFilter {
		mod += " · /" + m.query
	}
	// The mode indicator is never truncated — the user needs it most
	// exactly when the frame is tight. Only the left side gives way.
	tail := "  |  " + mod
	avail := m.width - lipWidth(tail)
	if avail < 0 {
		avail = 0
	}
	if desc := selectedDesc(m.selected(), avail-lipWidth(left)-3); desc != "" {
		left += " · " + desc
	}
	line := truncate(left, avail) + tail
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

// selectedDesc fits the selected skill description into budget cells.
func selectedDesc(sel *Skill, budget int) string {
	if sel == nil || sel.Desc == "" {
		return ""
	}
	return truncRunes(sel.Desc, budget)
}

// hintSets are ordered longest-first. The widest set that fits the
// terminal is used, so "? help · q quit" survives as long as there is
// room instead of being truncated away at a fixed breakpoint (B9).
var hintSets = []string{
	"j/k move · / filter · : cmd · e edit · d del · u undo · ? help · q quit",
	"j/k move · / find · : cmd · e edit · d del · ? help · q quit",
	"j/k · / find · : cmd · ? help · q quit",
	"q quit",
}

func fitHint(sets []string, w int) string {
	last := sets[len(sets)-1]
	for _, s := range sets {
		if lipWidth(s) <= w {
			return s
		}
	}
	return truncRunes(last, w)
}

func (m Model) renderFooter() string {
	var hints string
	switch m.appMode {
	case modeFilter:
		hints = "enter keep · esc back"
	case modeCommand:
		hints = "enter run · esc cancel · try: validate, reload, clear"
	case modeConfirm, modeConfirmCreate:
		hints = "y confirm · n cancel"
	case modeHelp:
		hints = "esc ? q close"
	default:
		// The full keymap lives in the ? overlay; the footer only has
		// to name the keys that fit.
		hints = fitHint(hintSets, m.width)
	}
	hints = truncate(hints, m.width)
	if m.plain {
		return hints
	}
	return m.theme.Footer.Render(hints)
}

// helpLines is the ? overlay body: the canonical keymap, then the
// environment gates. Both come from the single definition in keymap.go,
// so the overlay can never disagree with --help or the README.
func (m Model) helpLines() []string {
	lines := keyTableLines(m.cfg)
	lines = append(lines, "")
	return append(lines, envGateLines...)
}

func (m Model) renderHelpModal(under string) string {
	w := m.width - 4
	if w > 78 {
		w = 78
	}
	helpLines := m.helpLines()
	// Size the box to its content instead of a hardcoded height that
	// truncated the keymap on short terminals (audit E2).
	h := len(helpLines) + 3
	if h > m.height-1 {
		h = m.height - 1
	}
	modal := box("help · ?", strings.Join(helpLines, "\n"), w, h, true, m.theme, m.plain)
	return m.overlay(under, modal, m.width, m.height)
}

func (m Model) renderConfirmModal(under string) string {
	name := ""
	if sel := m.selected(); sel != nil {
		name = sel.Name
	}
	body := fmt.Sprintf("Delete skill %q?\nIt moves to the skillman trash. `u`\nrestores it for 30s, in this session only.",
		name)
	w := 52
	if w > m.width-4 {
		w = m.width - 4
	}
	// No trailing "--- [Esc] close ---": the footer already shows
	// "y confirm · n cancel" (audit B11).
	h := strings.Count(body, "\n") + 4
	modal := box("confirm delete", body, w, h, true, m.theme, m.plain)
	return m.overlay(under, modal, m.width, m.height)
}

// renderCreateModal asks before writing a file that does not exist yet.
// The wording names the exact file, because "create?" with no path is a
// question the user can only answer by guessing.
func (m Model) renderCreateModal(under string) string {
	name := ""
	if sel := m.selected(); sel != nil {
		name = sel.Name
	}
	body := fmt.Sprintf("%s has no SKILL.md.\nCreate one from a template and open\nit in $EDITOR?",
		name)
	w := 52
	if w > m.width-4 {
		w = m.width - 4
	}
	h := strings.Count(body, "\n") + 4
	modal := box("create SKILL.md", body, w, h, true, m.theme, m.plain)
	return m.overlay(under, modal, m.width, m.height)
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

// overlay composites the modal on top of the background frame and keeps
// the background visible but dimmed (audit B11). The old version threw
// the base frame away and appended a "--- [Esc] close ---" line that
// contradicted the footer hints.
//
// The result is exactly h lines wide w, so an open modal never makes
// the frame scroll.
func (m Model) overlay(under, modal string, w, h int) string {
	ul := strings.Split(under, "\n")
	ml := strings.Split(modal, "\n")
	mw := 0
	for _, ln := range ml {
		if lipWidth(ln) > mw {
			mw = lipWidth(ln)
		}
	}
	if mw > w {
		mw = w
	}
	pad := (w - mw) / 2
	if pad < 0 {
		pad = 0
	}
	trail := w - pad - mw
	if trail < 0 {
		trail = 0
	}
	top := (h - len(ml)) / 2
	if top < 0 {
		top = 0
	}
	var b strings.Builder
	for i := 0; i < h; i++ {
		switch {
		case i >= top && i < top+len(ml):
			ln := strings.Repeat(" ", pad) + ml[i-top] + strings.Repeat(" ", trail)
			b.WriteString(truncate(ln, w) + "\n")
		case i < len(ul):
			// Strip styling first: dimming an already coloured line
			// would fight the palette instead of receding.
			bg := padRight(truncRunes(stripANSI(ul[i]), w), w)
			if m.plain {
				b.WriteString(bg + "\n")
			} else {
				b.WriteString(m.theme.Dim.Render(bg) + "\n")
			}
		default:
			blank := strings.Repeat(" ", w)
			if m.plain {
				b.WriteString(blank + "\n")
			} else {
				b.WriteString(m.theme.Dim.Render(blank) + "\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// truncate shortens s to at most w display cells, marking the cut with
// an ellipsis. ANSI escape sequences are copied through whole and never
// split, and a cut inside styled text is reset so the style cannot bleed
// into the rest of the frame.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	cut := cutCells(s, w-1)
	if strings.Contains(cut, "\x1b") {
		cut += "\x1b[0m"
	}
	return cut + "…"
}

// cutCells returns the longest prefix of s that occupies at most
// maxCells display cells.
func cutCells(s string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	var b strings.Builder
	cells := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := skipEscape(s, i)
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := runewidth.RuneWidth(r)
		if cells+w > maxCells {
			break
		}
		b.WriteRune(r)
		cells += w
		i += size
	}
	return b.String()
}

// truncRunes is truncate's plain-text form: keep the leading n cells.
func truncRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipWidth(s) <= n {
		return s
	}
	return cutCells(s, n)
}

func padRight(s string, w int) string {
	d := w - lipWidth(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// lipWidth is the visible width of s in terminal cells. It must count
// cells, not runes: a CJK or emoji character occupies two cells, and
// counting runes overflowed the box borders and wrapped the frame.
func lipWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

// skipEscape returns the index just past the ANSI escape sequence that
// starts at s[i] (which must be ESC). CSI, OSC and two-character
// sequences are all recognised; anything else is treated as a single
// byte so the walk always makes progress.
func skipEscape(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[': // CSI: final byte is in the range @-~
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j < len(s) {
			j++
		}
		return j
	case ']': // OSC: ends at BEL or ST (ESC backslash)
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	default:
		return i + 2
	}
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEscape(s, i)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
