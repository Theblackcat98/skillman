package main

import (
	"strconv"
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

type toastClearMsg struct{}

type undoExpireMsg struct{}

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
	toastAt    time.Time
	errMsg     string
	confirmIdx int

	undo pendingUndo

	// Preview render cache: key is skill name + wrap width. Rebuilding a
	// glamour renderer and re-rendering markdown on every keypress was the
	// main source of j/k lag, so renders are cached and the renderer reused.
	previewCache  map[string]string
	previewCacheW int
	glam          *glamour.TermRenderer
}

func NewModel(plain, noAnim bool) Model {
	th := NewTheme(plain)
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
		theme: th, plain: plain, noAnim: noAnim,
		loading: true,
		preview: vp, filter: ti, cmdline: ci, spinner: sp,
		width: 80, height: 24,
		previewCache: map[string]string{},
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
	m.refreshPreview()
}

func (m *Model) refreshPreview() {
	sel := m.selected()
	w := m.previewWidth()
	if sel == nil {
		m.preview.SetContent("(no skills match — press / to filter, r to rescan)")
		return
	}
	m.preview.SetContent(m.cachedPreview(*sel, w))
	m.preview.GotoTop()
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
	header := s.Name + "\n" + s.Desc + "\n\n"
	text := header + s.renderPreviewWith(m.sharedRenderer(w), m.plain)
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

func (m *Model) previewWidth() int {
	w := m.width/2 - 6
	if m.width < 70 {
		w = m.width - 6
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (m *Model) setToast(text string, isErr bool) tea.Cmd {
	m.toast = text
	m.toastErr = isErr
	m.toastAt = time.Now()
	if m.noAnim && !isErr {
		return nil
	}
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return toastClearMsg{}
	})
}
