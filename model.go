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
	// modeConfirmCreate gates creating a SKILL.md that does not exist
	// yet, so `e` never writes a file the user did not ask for.
	modeConfirmCreate
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

// editedMsg says the editor closed without error. The reload itself is
// scheduled by the model, so the edit path and every other reload share
// one code path.
type editedMsg struct{ name string }

// validateMsg carries the rescan from `v` back into the model, so the
// list badges and the summary come from one scan (review F9). It used to
// be a bare toast, which meant the badges on screen were from the
// previous scan.
type validateMsg struct {
	skills []Skill
	text   string
	isErr  bool
}

func loadSkillsCmd() tea.Cmd {
	return loadSkillsAt(skillsDir())
}

func loadSkillsAt(dir string) tea.Cmd {
	return func() tea.Msg {
		skills, err := ScanSkills(dir)
		return skillsLoadedMsg{skills: skills, err: err}
	}
}

// reloadCmd is the one path that re-reads the skills directory. Every
// operation that changes the disk underneath the model funnels through it:
// the initial scan, r, a post-edit reload, a delete and an undo.
//
// It used not to. Delete and undo called ScanSkills inline inside
// Update and threw the error away, so a transient read failure emptied
// the list while the status bar still claimed the skill was deleted
// (review B1, B2). More importantly the inline rescan ran synchronously
// on the update path, which freezes the UI on slow storage or with a
// thousand skills.
func (m *Model) reloadCmd(reason string) tea.Cmd {
	m.loading = true
	cmds := []tea.Cmd{loadSkillsCmd()}
	if !m.noAnim {
		// The tick chain stops when idle, so it has to be re-armed for
		// each new scan or the spinner freezes mid-turn.
		cmds = append(cmds, m.spinner.Tick)
	}
	if reason != "" {
		cmds = append(cmds, toastCmd(reason, false))
	}
	return tea.Batch(cmds...)
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
	// selName is the selection. It is a name, not an index into filtered,
	// because an index is a position in a list that a reload, a filter
	// change or a delete reshuffles underneath the user: the row stayed
	// the same and the skill under it silently changed (review F7).
	// Nothing owns an index; cursorIndex derives one for drawing.
	selName string
	query   string

	preview viewport.Model
	// helpVP scrolls the ? overlay. The keymap is taller than a short
	// terminal, so a fixed-height box silently cut it off with no way to
	// see the rest (review E2).
	helpVP  viewport.Model
	filter  textinput.Model
	cmdline textinput.Model
	spinner spinner.Model

	appMode       mode
	focusPreview  bool
	showPreviewOv bool // narrow screens: show preview instead of list

	toast    string
	toastErr bool
	toastSeq int // generation tokens so stale timers can't clear
	undoSeq  int // fresher state (audit B6)
	errMsg   string

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
		preview: vp, helpVP: vp, filter: ti, cmdline: ci, spinner: sp,
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

// selected is the skill under the cursor, or nil on an empty list.
func (m *Model) selected() *Skill {
	i := m.cursorIndex()
	if i < 0 || i >= len(m.filtered) {
		return nil
	}
	return &m.filtered[i]
}

// cursorIndex derives the selection's position in the filtered list. It is
// -1 when the selected name is not in the current view, which is the
// normal state while a filter hides it.
func (m Model) cursorIndex() int {
	if m.selName == "" {
		if len(m.filtered) > 0 {
			return 0
		}
		return -1
	}
	for i := range m.filtered {
		if m.filtered[i].Name == m.selName {
			return i
		}
	}
	// Not visible. Fall back to the first row so the list always has a
	// cursor, but do not change the stored name: a filter the user is
	// about to clear should not lose the selection.
	if len(m.filtered) > 0 {
		return 0
	}
	return -1
}

// selectAt moves the cursor to a position, clamped to the list.
func (m *Model) selectAt(i int) {
	if len(m.filtered) == 0 {
		m.selName = ""
		m.refreshPreview()
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(m.filtered) {
		i = len(m.filtered) - 1
	}
	m.selName = m.filtered[i].Name
	m.refreshPreview()
}

func (m *Model) selectLast() { m.selectAt(len(m.filtered) - 1) }

// selectName selects by name, and reports whether the name was in the
// list. Used to restore a saved session and an undone delete.
func (m *Model) selectName(name string) bool {
	for i := range m.filtered {
		if m.filtered[i].Name == name {
			m.selName = name
			m.refreshPreview()
			return true
		}
	}
	return false
}

func (m *Model) applyFilter() {
	// FilterSkills returns a copy, so a later mutation of m.filtered can
	// never scribble on m.skills (review F6).
	m.filtered = FilterSkills(m.skills, m.query)
	// Keep the selection if it survived the filter. A skill that is only
	// hidden is not the same as one that is gone: the user is one Esc
	// away from clearing the filter and must land back on the same skill.
	// So the name is kept while it still exists anywhere, and dropped only
	// when the skill is really gone.
	if m.selName != "" && !m.nameExists() {
		m.selName = ""
	}
	if m.selName == "" && len(m.filtered) > 0 {
		m.selName = m.filtered[0].Name
	}
	m.applySelection()
	m.refreshPreview()
}

// nameVisible reports whether the selection is in the current filter.
func (m *Model) nameVisible() bool {
	for i := range m.filtered {
		if m.filtered[i].Name == m.selName {
			return true
		}
	}
	return false
}

// nameExists reports whether the selection is still a skill on disk,
// whether or not the current filter shows it.
func (m *Model) nameExists() bool {
	if m.selName == "" {
		return false
	}
	for i := range m.skills {
		if m.skills[i].Name == m.selName {
			return true
		}
	}
	return false
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
	w := m.layout().previewWrap()
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

// previewCacheLimit caps how many rendered previews are kept. Each entry
// is a glamour render of one skill's whole body, so an unbounded cache
// is a second copy of every skill the user has ever looked at (review
// D2). The limit is generous: a reader who has visited far more than
// this many skills wants the list, not the history.
const previewCacheLimit = 24

// cachedPreview returns the cached render for a skill, rendering once on
// first visit. This keeps cursor movement at cache-lookup cost.
//
// When the cache is full the oldest entry is dropped. Go maps have no
// order, so the eviction is a plain clear rather than an LRU: a full
// rebuild costs one glamour pass over the visible skill, which is
// cheaper than tracking insertion order for a cache this small, and
// bounded is the property that matters.
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
			head.WriteString("- " + is.Msg + "\n")
		}
		head.WriteString("\n")
	}
	head.WriteString("---\n\n")
	text := head.String() + s.renderPreviewWith(m.sharedRenderer(w), m.plain)
	if m.previewCache == nil {
		m.previewCache = map[string]string{}
	}
	if len(m.previewCache) >= previewCacheLimit {
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
