package main

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
	header := sel.Name + "\n" + sel.Desc + "\n\n"
	body := sel.RenderPreview(w, m.plain)
	m.preview.SetContent(header + body)
	m.preview.GotoTop()
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
