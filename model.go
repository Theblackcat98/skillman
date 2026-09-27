package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeCommand
	modeHelp
	modeConfirm
)

type skillsLoadedMsg struct {
	skills []Skill
	err    error
}

type toastMsg struct {
	text  string
	isErr bool
}

type toastClearMsg struct{ seq int }

type undoExpireMsg struct{ seq int }

func loadSkillsCmd() tea.Cmd {
	return func() tea.Msg {
		skills, err := ScanSkills(skillsDir())
		return skillsLoadedMsg{skills: skills, err: err}
	}
}

// rescanCmd reloads skills and restarts the spinner tick (the tick chain
// stops when idle, so it must be re-armed for each new scan).
func (m *Model) rescanCmd() tea.Cmd {
	m.loading = true
	if m.noAnim {
		return loadSkillsCmd()
	}
	return tea.Batch(loadSkillsCmd(), m.spinner.Tick)
}

func toastCmd(text string, isErr bool) tea.Cmd {
	return func() tea.Msg {
		return toastMsg{text: text, isErr: isErr}
	}
}

type Model struct {
	theme   Theme
	cfg     Config
	plain   bool
	noAnim  bool
	width   int
	height  int
	ready   bool
	loading bool

	skills   []Skill
	filtered []Skill
	cursor   int
	query    string

	preview viewport.Model
	filter  textinput.Model
	cmdline textinput.Model
	spinner spinner.Model

	appMode       mode
	focusPreview  bool
	showPreviewOv bool // narrow screens: show preview instead of list

	toast      string
	toastErr   bool
	toastSeq   int // generation tokens so stale timers can't clear
	undoSeq    int // fresher state (audit B6)
	errMsg     string
	confirmIdx int

	undo pendingUndo

	// Per-skill preview scroll, so moving the list cursor does not throw
	// away where the reader was, and the positions survive a restart.
	scroll           map[string]int
	previewName      string
	pendingSelection string

	// Preview render cache: key is skill name + wrap width. Rebuilding a
	// glamour renderer and re-rendering markdown on every keypress was the
	// main source of j/k lag, so renders are cached and the renderer reused.
	previewCache  map[string]string
	previewCacheW int
	glam          *glamour.TermRenderer
}

func NewModel(plain, noAnim bool, cfg Config) Model {
	th := NewTheme(plain, cfg.Accent)
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter skills"
	ti.CharLimit = 80

	ci := textinput.New()
	ci.Prompt = ":"
	ci.Placeholder = "edit|delete|validate|reload|quit|help"
	ci.CharLimit = 80

	sp := spinner.New()
	sp.Spinner.FPS = 12

	vp := viewport.New(40, 10)

	return Model{
		theme: th, cfg: cfg, plain: plain, noAnim: noAnim,
		loading: true,
		preview: vp, filter: ti, cmdline: ci, spinner: sp,
		width: 80, height: 24,
		previewCache: map[string]string{},
		scroll:       map[string]int{},
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{loadSkillsCmd()}
	if !m.noAnim {
		cmds = append(cmds, m.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func (m *Model) selected() *Skill {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.cursor]
}

func (m *Model) applyFilter() {
	m.filtered = FilterSkills(m.skills, m.query)
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.applySelection()
	m.refreshPreview()
}

func (m *Model) refreshPreview() {
	m.refreshPreviewAt(true)
}

// refreshPreviewAt reloads the preview content. reset=true scrolls to
// top (the selection changed); reset=false keeps the current offset so
// a resize never throws away the reader's position.
func (m *Model) refreshPreviewAt(reset bool) {
	sel := m.selected()
	name := ""
	if sel != nil {
		name = sel.Name
	}
	// Remember where the outgoing skill was read to, so returning to it
	// later resumes in the same place.
	if m.previewName != "" && m.previewName != name {
		m.scroll[m.previewName] = m.preview.YOffset
	}
	w := m.previewWidth()
	if sel == nil {
		switch {
		case m.loading:
			m.preview.SetContent("scanning…")
		case len(m.skills) == 0:
			m.preview.SetContent("(no skills in " + skillsDir() + ") — r rescans")
		default:
			// A query is active and matched nothing (B7).
			m.preview.SetContent("(no match for /" + m.query + ") — esc clears the filter")
		}
		m.preview.GotoTop()
		m.previewName = ""
		return
	}
	m.preview.SetContent(m.cachedPreview(*sel, w))
	switch {
	case !reset && m.previewName == name:
		// Same skill, new size: keep the reader in place.
	case m.scroll[name] > 0:
		m.preview.SetYOffset(m.scroll[name])
	default:
		m.preview.GotoTop()
	}
	m.previewName = name
}

// invalidatePreview drops cached renders. Called on every rescan so edits,
// deletes, and undos never show stale content.
func (m *Model) invalidatePreview() {
	m.previewCache = map[string]string{}
}

// cachedPreview returns the cached render for a skill, rendering once on
// first visit. This keeps cursor movement at cache-lookup cost.
func (m *Model) cachedPreview(s Skill, w int) string {
	key := s.Name + "\x00" + strconv.Itoa(w)
	if text, ok := m.previewCache[key]; ok {
		return text
	}
	// Banner: identity, description, frontmatter metadata, then any
	// validation issues. Validation problems must be visible even when
	// the body renders cleanly.
	var head strings.Builder
	head.WriteString("# " + s.Name + "\n\n")
	if s.Desc != "" {
		head.WriteString(s.Desc + "\n\n")
	}
	if meta := s.metaLine(); meta != "" {
		head.WriteString(meta + "\n\n")
	}
	if len(s.Issues) > 0 {
		head.WriteString("## Issues\n\n")
		for _, is := range s.Issues {
			head.WriteString("- " + is + "\n")
		}
		head.WriteString("\n")
	}
	head.WriteString("---\n\n")
	text := head.String() + s.renderPreviewWith(m.sharedRenderer(w), m.plain)
	if m.previewCache == nil {
		m.previewCache = map[string]string{}
	}
	m.previewCache[key] = text
	return text
}

// sharedRenderer reuses one glamour renderer until the wrap width changes.
func (m *Model) sharedRenderer(w int) *glamour.TermRenderer {
	if m.plain {
		return nil
	}
	if m.glam == nil || m.previewCacheW != w {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(w),
		)
		if err != nil {
			return nil
		}
		m.glam = r
		m.previewCacheW = w
		m.previewCache = map[string]string{}
	}
	return m.glam
}

// paneW returns the list and preview pane widths for the current terminal
// width. Layout (View) and sizing (previewWidth) must agree: the glamour
// wrap width has to equal the box interior or the preview reflows as the
// selection changes. Duplicated layout constants caused audit B2.
func (m Model) paneW() (listW, prevW int) {
	if m.width < 70 {
		// Narrow: one pane at a time, full width.
		return m.width, m.width
	}
	// joinH puts a single space between the panes, so the panes plus the
	// gap must add up to exactly m.width for a flush frame.
	listW = (m.width - 1) / 2
	prevW = m.width - 1 - listW
	return listW, prevW
}

func (m *Model) previewWidth() int {
	_, prevW := m.paneW()
	// box() draws "| content |", so the interior is 2 cells narrower.
	w := prevW - 2
	if w < 20 {
		w = 20
	}
	return w
}

func (m *Model) setToast(text string, isErr bool) tea.Cmd {
	m.toast = text
	m.toastErr = isErr
	m.toastSeq++
	seq := m.toastSeq
	// Always schedule dismissal — including with animations off,
	// where the old early-return left success toasts stuck forever (B5).
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return toastClearMsg{seq: seq}
	})
}

// mainBoxH is the height of the list/preview box. Layout (View) and
// sizing (sizePanes) must share this one number — duplicated constants
// drifted before and produced the off-by-one in audit B2.
//
// The frame is header + box + status + footer, so the box takes the
// remaining rows exactly. The old height-4 left one row permanently
// unused (audit B16).
func (m Model) mainBoxH() int {
	h := m.height - 3
	if h < 5 {
		h = 5
	}
	return h
}

// contentH is how many body lines box() actually renders for a box of
// height h: title line + top + bottom borders are not content.
func contentH(h int) int {
	v := h - 3
	if v < 1 {
		v = 1
	}
	return v
}
