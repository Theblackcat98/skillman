package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width < 20 {
			m.width = 20
		}
		if m.height < 10 {
			m.height = 10
		}
		m.ready = true
		m.sizePanes()
		if m.appMode == modeHelp {
			m.helpVP.SetContent(strings.Join(m.helpLines(), "\n"))
		}
		// Keep the reader's scroll position across resize (B16 polish).
		m.refreshPreviewAt(false)
		return m, nil

	case editedMsg:
		// The editor closed cleanly; reload and say so.
		m.invalidatePreview()
		return m, m.reloadCmd("edited " + msg.name)

	case skillsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			logf("scan error: %v", msg.err)
			cmd := m.setToast("scan failed — see log", true)
			return m, cmd
		}
		m.errMsg = ""
		m.skills = msg.skills
		m.invalidatePreview()
		m.applyFilter()
		m.sizePanes()
		return m, nil

	case validateMsg:
		// The list the user is looking at must match the numbers in the
		// toast they just read.
		m.loading = false
		m.skills = msg.skills
		m.invalidatePreview()
		m.applyFilter()
		m.sizePanes()
		cmd := m.setToast(msg.text, msg.isErr)
		return m, cmd

	case toastMsg:
		cmd := m.setToast(msg.text, msg.isErr)
		return m, cmd

	case toastClearMsg:
		// Ignore clears scheduled by an older toast (B6).
		if msg.seq == m.toastSeq {
			m.toast = ""
			m.toastErr = false
		}
		return m, nil

	case undoExpireMsg:
		// Ignore expiry from a previous delete: a newer undo window
		// must keep its full 30s (B6).
		if msg.seq == m.undoSeq {
			m.undo = pendingUndo{}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Route spinner/viewport/input ticks by mode.
	var cmds []tea.Cmd
	switch m.appMode {
	case modeFilter:
		var c tea.Cmd
		m.filter, c = m.filter.Update(msg)
		if c != nil {
			cmds = append(cmds, c)
		}
		m.query = m.filter.Value()
		m.applyFilter()
	case modeCommand:
		var c tea.Cmd
		m.cmdline, c = m.cmdline.Update(msg)
		if c != nil {
			cmds = append(cmds, c)
		}
	}
	// Spinner ticks only while loading. A never-ending tick loop would
	// redraw the full screen 12x/sec even when idle, queueing up behind
	// slow terminal redraws and delaying keypresses.
	if m.loading && !m.noAnim {
		var c tea.Cmd
		m.spinner, c = m.spinner.Update(msg)
		if c != nil {
			cmds = append(cmds, c)
		}
	}
	// Viewport scroll (when preview focused or always with pgup/pgdn handled in keys).
	if m.focusPreview || m.appMode == modeNormal {
		var c tea.Cmd
		m.preview, c = m.preview.Update(msg)
		if c != nil {
			cmds = append(cmds, c)
		}
	}
	return m, tea.Batch(cmds...)
}

// sizePanes pushes the layout into the viewports. The values come from
// the one layout owner, so a pane can never be sized differently from the
// way it is drawn (review F3).
func (m *Model) sizePanes() {
	l := m.layout()
	m.preview.Width = l.prevInner
	// Match box() content lines exactly, otherwise the last preview
	// lines can never scroll into view.
	m.preview.Height = l.bodyH
	m.helpVP.Width = l.helpInner
	m.helpVP.Height = l.helpBodyH
}

// openHelp shows the keymap overlay. Content and size are set here, so
// opening help on a resized terminal shows a full box.
func (m *Model) openHelp() tea.Cmd {
	m.appMode = modeHelp
	m.sizePanes()
	m.helpVP.SetContent(strings.Join(m.helpLines(), "\n"))
	m.helpVP.GotoTop()
	return nil
}

// The command is always bound to a local before the model is returned.
// `return m, m.doDelete()` works on gc because a call is evaluated
// before the surrounding operands are copied, but the Go spec does not
// order non-call operands against calls, so it is a latent trap rather
// than correct code (review F2).
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Ctrl-C must quit from every layer — filter, command, help, and
	// confirm otherwise swallow it and the app becomes unkillable (B4).
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Help overlay: Esc/?/q/Enter closes, everything else scrolls. The
	// keymap is taller than a short terminal, so j/k/pgup/pgdn/g/G are
	// the only way to read the lower half (review E2).
	if m.appMode == modeHelp {
		switch key {
		case "esc", "?", "q", "enter":
			m.appMode = modeNormal
		default:
			var c tea.Cmd
			m.helpVP, c = m.helpVP.Update(msg)
			_ = c
		}
		return m, nil
	}

	// Confirm modals: y confirms, n/Esc cancels. Each one gates a
	// different action, so they stay separate modes rather than sharing
	// a flag.
	switch m.appMode {
	case modeConfirm:
		switch key {
		case "y", "Y", "enter":
			cmd := m.doDelete()
			return m, cmd
		case "n", "N", "esc":
			m.appMode = modeNormal
			return m, nil
		}
		return m, nil
	case modeConfirmCreate:
		switch key {
		case "y", "Y", "enter":
			cmd := m.doCreateAndEdit()
			return m, cmd
		case "n", "N", "esc":
			m.appMode = modeNormal
			return m, nil
		}
		return m, nil
	}

	// Filter input mode.
	if m.appMode == modeFilter {
		switch key {
		case "esc":
			m.appMode = modeNormal
			m.filter.Blur()
			return m, nil
		case "enter":
			m.appMode = modeNormal
			m.filter.Blur()
			return m, nil
		}
		var c tea.Cmd
		m.filter, c = m.filter.Update(msg)
		m.query = m.filter.Value()
		m.applyFilter()
		return m, c
	}

	// Command palette mode.
	if m.appMode == modeCommand {
		switch key {
		case "esc":
			m.appMode = modeNormal
			m.cmdline.Blur()
			m.cmdline.SetValue("")
			return m, nil
		case "enter":
			typed := strings.TrimSpace(m.cmdline.Value())
			m.cmdline.SetValue("")
			m.cmdline.Blur()
			m.appMode = modeNormal
			cmd := m.runCommand(typed)
			return m, cmd
		}
		var c tea.Cmd
		m.cmdline, c = m.cmdline.Update(msg)
		return m, c
	}

	// Preview focus: navigation keys scroll the preview, and d/u —
	// half-page scroll keys in a viewport — must never reach the list's
	// delete/undo handlers from here (audit B3).
	if m.focusPreview {
		switch key {
		case "g", "home":
			m.preview.GotoTop()
			return m, nil
		case "G", "end":
			m.preview.GotoBottom()
			return m, nil
		case "j", "k", "up", "down", "pgup", "pgdown", "ctrl+f", "ctrl+b",
			"d", "u", "f", "b", " ", "ctrl+d", "ctrl+u":
			var c tea.Cmd
			m.preview, c = m.preview.Update(msg)
			return m, c
		}
	}

	// Normal mode. A config keybinding override is translated to the
	// built-in key for that action first, so the switch below stays the
	// only place that says what an action does.
	switch m.cfg.resolveKey(key) {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?", "H":
		cmd := m.openHelp()
		return m, cmd
	case "/":
		m.appMode = modeFilter
		m.filter.Focus()
		return m, nil
	case ":":
		m.appMode = modeCommand
		m.cmdline.Focus()
		return m, nil
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g":
		m.cursor = 0
		m.refreshPreview()
	case "G", "end":
		m.cursor = len(m.filtered) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.refreshPreview()
	case "home":
		m.cursor = 0
		m.refreshPreview()
	case "pgdown", "ctrl+f":
		m.move(10)
	case "pgup", "ctrl+b":
		m.move(-10)
	case "tab":
		if m.width < narrowBreak {
			m.showPreviewOv = !m.showPreviewOv
		} else {
			m.focusPreview = !m.focusPreview
		}
		return m, nil
	case "enter":
		m.focusPreview = !m.focusPreview
		return m, nil
	case "e":
		cmd := m.doEdit()
		return m, cmd
	case "d", "delete":
		if m.selected() == nil {
			return m, toastCmd("nothing to delete", true)
		}
		m.appMode = modeConfirm
		return m, nil
	case "u":
		cmd := m.doUndo()
		return m, cmd
	case "v":
		cmd := m.doValidate()
		return m, cmd
	case "r":
		cmd := m.reloadCmd("")
		return m, cmd
	case "esc":
		if m.query != "" {
			m.query = ""
			m.filter.SetValue("")
			m.applyFilter()
		} else if m.focusPreview {
			m.focusPreview = false
		}
		return m, nil
	}

	// Preview scrolling passthrough when focused.
	if m.focusPreview {
		var c tea.Cmd
		m.preview, c = m.preview.Update(msg)
		if c != nil {
			return m, c
		}
	}
	return m, nil
}

func (m *Model) move(d int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.refreshPreview()
}

func (m *Model) doEdit() tea.Cmd {
	sel := m.selected()
	if sel == nil {
		return toastCmd("nothing to edit", true)
	}
	// A skill with no SKILL.md is a real state (a half-unzipped archive,
	// a directory the user made by hand), and `e` on it used to open a
	// file listing. Ask before creating a file the user did not ask for.
	path, exists := skillEditPath(*sel)
	if !exists {
		m.appMode = modeConfirmCreate
		return nil
	}
	name := sel.Name
	// ExecProcess suspends the alt screen around the editor and
	// restores it afterwards (audit B10); $EDITOR flags are split
	// into argv by editorCmd.
	return tea.ExecProcess(editorCmd(path), func(err error) tea.Msg {
		if err != nil {
			logf("editor error: %v", err)
			return toastMsg{text: "editor failed: " + err.Error(), isErr: true}
		}
		// Edit is done: reload through the one path, so the preview can
		// never show stale content.
		return editedMsg{name: name}
	})
}

// doCreateAndEdit writes the starter SKILL.md, then opens the editor on
// it. Split from doEdit so the confirmation modal stays a pure gate.
func (m *Model) doCreateAndEdit() tea.Cmd {
	sel := m.selected()
	if sel == nil {
		m.appMode = modeNormal
		return nil
	}
	name := sel.Name
	path, created, err := ensureSkillMD(*sel)
	if err != nil {
		m.appMode = modeNormal
		logf("create SKILL.md: %v", err)
		return toastCmd("could not create SKILL.md: "+err.Error(), true)
	}
	if !created {
		m.appMode = modeNormal
		return m.doEdit()
	}
	m.appMode = modeNormal
	sk := *sel
	sk.Body = fmt.Sprintf(skillTemplate, name, name)
	sk.Desc = "one line, say what this skill is for"
	sk.Issues = nil
	cmds := []tea.Cmd{
		toastCmd("created "+name+"/SKILL.md", false),
		tea.ExecProcess(editorCmd(path), func(eerr error) tea.Msg {
			if eerr != nil {
				logf("editor error: %v", eerr)
				return toastMsg{text: "editor failed: " + eerr.Error(), isErr: true}
			}
			return editedMsg{name: name}
		}),
	}
	return tea.Batch(cmds...)
}

// doDelete moves the selected skill to the trash, then reloads through
// the one path. The file operation is the only synchronous I/O left here,
// because it has to report its own error before the reload is scheduled.
func (m *Model) doDelete() tea.Cmd {
	sel := m.selected()
	if sel == nil {
		m.appMode = modeNormal
		return nil
	}
	name := sel.Name
	dest, err := deleteSkillToTrash(*sel)
	m.appMode = modeNormal
	if err != nil {
		logf("delete error: %v", err)
		return toastCmd("delete failed: "+err.Error(), true)
	}
	m.undo = pendingUndo{Name: name, TrashPath: dest, Active: true}
	m.undoSeq++
	seq := m.undoSeq
	// Forget the cached render now rather than waiting for the reload:
	// the skill is gone, and the key it is cached under may name a
	// different skill by the time the list is rebuilt (review A11).
	m.invalidatePreview()
	return tea.Batch(
		m.reloadCmd("deleted "+name+" — u to undo"),
		tea.Tick(30*time.Second, func(time.Time) tea.Msg { return undoExpireMsg{seq: seq} }),
	)
}

func (m *Model) doUndo() tea.Cmd {
	if !m.undo.Active {
		return toastCmd("nothing to undo", true)
	}
	name := m.undo.Name
	if err := undoDelete(m.undo.Name, m.undo.TrashPath); err != nil {
		logf("undo error: %v", err)
		return toastCmd("undo failed: "+err.Error(), true)
	}
	m.undo = pendingUndo{}
	m.pendingSelection = name
	m.invalidatePreview()
	return m.reloadCmd("restored " + name)
}

// doValidate reloads and reports, so the list badges and the summary come
// from the same scan instead of two (review F9).
func (m *Model) doValidate() tea.Cmd {
	return func() tea.Msg {
		skills, err := ScanSkills(skillsDir())
		if err != nil {
			return toastMsg{text: "validate failed: " + err.Error(), isErr: true}
		}
		ok, warn, bad := summarize(skills)
		return validateMsg{
			skills: skills,
			text:   fmt.Sprintf("%d ok · %d warn · %d err", ok, warn, bad),
			isErr:  bad > 0,
		}
	}
}

func (m *Model) runCommand(cmd string) tea.Cmd {
	switch strings.ToLower(cmd) {
	case "", "q", "quit":
		// empty : just closes; q quits via command too
		if cmd == "" {
			return nil
		}
		return tea.Quit
	case "reload", "r":
		return m.reloadCmd("")
	case "validate", "v":
		return m.doValidate()
	case "edit", "e":
		return m.doEdit()
	case "delete", "d":
		if m.selected() == nil {
			return toastCmd("nothing to delete", true)
		}
		m.appMode = modeConfirm
		return nil
	case "help", "h", "?":
		return m.openHelp()
	case "clear":
		m.query = ""
		m.filter.SetValue("")
		m.applyFilter()
		return nil
	default:
		if strings.HasPrefix(strings.ToLower(cmd), "filter ") {
			q := strings.TrimSpace(cmd[7:])
			m.query = q
			m.filter.SetValue(q)
			m.applyFilter()
			return nil
		}
		return toastCmd("unknown command: "+cmd, true)
	}
}
