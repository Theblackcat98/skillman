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
		m.refreshPreview()
		return m, nil

	case skillsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			logf("scan error: %v", msg.err)
			return m, m.setToast("scan failed — see log", true)
		}
		m.errMsg = ""
		m.skills = msg.skills
		m.invalidatePreview()
		m.applyFilter()
		m.sizePanes()
		return m, nil

	case toastMsg:
		return m, m.setToast(msg.text, msg.isErr)

	case toastClearMsg:
		m.toast = ""
		m.toastErr = false
		return m, nil

	case undoExpireMsg:
		m.undo = pendingUndo{}
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

func (m *Model) sizePanes() {
	h := m.mainBoxH()
	m.preview.Width = m.previewWidth()
	// Match box() content lines exactly, otherwise the last preview
	// lines can never scroll into view.
	m.preview.Height = contentH(h)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Ctrl-C must quit from every layer — filter, command, help, and
	// confirm otherwise swallow it and the app becomes unkillable (B4).
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Help overlay: any Esc/?/q/Enter closes.
	if m.appMode == modeHelp {
		if key == "esc" || key == "?" || key == "q" || key == "enter" {
			m.appMode = modeNormal
		}
		return m, nil
	}

	// Confirm delete modal: y confirms, n/Esc cancels.
	if m.appMode == modeConfirm {
		switch key {
		case "y", "Y", "enter":
			return m, m.doDelete()
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
			cmd := strings.TrimSpace(m.cmdline.Value())
			m.cmdline.SetValue("")
			m.cmdline.Blur()
			m.appMode = modeNormal
			return m, m.runCommand(cmd)
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

	// Normal mode.
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?", "H":
		m.appMode = modeHelp
		return m, nil
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
		if m.width < 70 {
			m.showPreviewOv = !m.showPreviewOv
		} else {
			m.focusPreview = !m.focusPreview
		}
		return m, nil
	case "enter":
		m.focusPreview = !m.focusPreview
		return m, nil
	case "e":
		return m, m.doEdit()
	case "d", "delete":
		if m.selected() == nil {
			return m, toastCmd("nothing to delete", true)
		}
		m.appMode = modeConfirm
		return m, nil
	case "u":
		return m, m.doUndo()
	case "v":
		return m, m.doValidate()
	case "r":
		return m, m.rescanCmd()
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
	path := skillEditPath(*sel)
	c := func() tea.Msg {
		if err := openInEditor(path); err != nil {
			logf("editor error: %v", err)
			return toastMsg{text: "editor failed: " + err.Error(), isErr: true}
		}
		skills, err := ScanSkills(skillsDir())
		if err != nil {
			return toastMsg{text: "rescan failed", isErr: true}
		}
		_ = skills
		return toastMsg{text: "edited " + sel.Name + " — press r to rescan", isErr: false}
	}
	// NOTE: bubbletea suspends via ExecCommand; here we run editor
	// synchronously — main.go wraps TUI with alt-screen restore so the
	// terminal is usable. Rescan happens on next keypress or r.
	return c
}

func (m *Model) doDelete() tea.Cmd {
	sel := m.selected()
	if sel == nil {
		m.appMode = modeNormal
		return nil
	}
	name := sel.Name
	dest, err := deleteSkillToTrash(*sel)
	if err != nil {
		m.appMode = modeNormal
		logf("delete error: %v", err)
		return toastCmd("delete failed: "+err.Error(), true)
	}
	m.appMode = modeNormal
	m.undo = pendingUndo{Name: name, TrashPath: dest, Active: true, ExpiresAt: time.Now().Add(30 * time.Second)}
	// Rescan inline.
	skills, _ := ScanSkills(skillsDir())
	m.skills = skills
	m.applyFilter()
	m.refreshPreview()
	cmds := []tea.Cmd{
		m.setToast(fmt.Sprintf("deleted %s — u to undo", name), false),
		tea.Tick(30*time.Second, func(time.Time) tea.Msg { return undoExpireMsg{} }),
	}
	return tea.Batch(cmds...)
}

func (m *Model) doUndo() tea.Cmd {
	if !m.undo.Active {
		return toastCmd("nothing to undo", true)
	}
	if err := undoDelete(m.undo.Name, m.undo.TrashPath); err != nil {
		logf("undo error: %v", err)
		return toastCmd("undo failed: "+err.Error(), true)
	}
	name := m.undo.Name
	m.undo = pendingUndo{}
	skills, _ := ScanSkills(skillsDir())
	m.skills = skills
	m.applyFilter()
	// Restore cursor to the revived skill.
	for i, s := range m.filtered {
		if s.Name == name {
			m.cursor = i
			break
		}
	}
	m.refreshPreview()
	return toastCmd("restored "+name, false)
}

func (m *Model) doValidate() tea.Cmd {
	return func() tea.Msg {
		skills, err := ScanSkills(skillsDir())
		if err != nil {
			return toastMsg{text: "validate failed", isErr: true}
		}
		ok, warn, bad := summarize(skills)
		return toastMsg{
			text:  fmt.Sprintf("%d ok · %d warn · %d err", ok, warn, bad),
			isErr: bad > 0,
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
		return m.rescanCmd()
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
		m.appMode = modeHelp
		return nil
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
