package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	case installClonedMsg:
		m.installBusy = false
		if msg.err != nil {
			// Stay in the flow with the error visible, so the URL can be
			// corrected and retried without starting over.
			m.appMode = modeInstallInput
			m.installURL.Focus()
			m.installErr = msg.err.Error()
			return m, m.setToast("install failed: "+msg.err.Error(), true)
		}
		m.installErr = ""
		m.installSrc = msg.src
		m.installCands = msg.cands
		m.installIdx = 0
		if len(msg.cands) == 0 {
			cmd := m.closeInstall()
			return m, tea.Batch(cmd, m.setToast("no SKILL.md in that repository", true))
		}
		m.appMode = modeInstallPick
		return m, nil

	case installDoneMsg:
		m.installBusy = false
		cleanup := removeTempTree(msg.src)
		m.installSrc = ""
		m.installCands = nil
		m.appMode = modeNormal
		if msg.err != nil {
			logf("install: %v", msg.err)
			return m, tea.Batch(cleanup, m.setToast("install failed: "+msg.err.Error(), true))
		}
		if len(msg.res.Installed) > 0 {
			// Put the cursor on something new so the install is visible
			// rather than just announced.
			m.pendingSelection = msg.res.Installed[0]
		}
		text := "installed " + strings.Join(msg.res.Installed, ", ")
		if len(msg.res.Skipped) > 0 {
			text += fmt.Sprintf(" · skipped %d", len(msg.res.Skipped))
		}
		// One reload, through the one path, and only when something landed.
		if len(msg.res.Installed) == 0 {
			return m, tea.Batch(cleanup, m.setToast("nothing installed", true))
		}
		return m, tea.Batch(cleanup, m.reloadCmd(text))

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
	//
	// Quitting with a clone in flight also removes it: a temp directory
	// left behind by an install the user abandoned is litter nobody
	// asked for.
	if key == "ctrl+c" {
		return m, tea.Batch(removeTempTree(m.installSrc), tea.Quit)
	}

	// Install flow. Both modes own their keys completely, so no list
	// navigation or delete key can reach through them.
	switch m.appMode {
	case modeInstallInput:
		switch key {
		case "esc":
			cmd := m.closeInstall()
			return m, cmd
		case "enter":
			cmd := m.startInstall()
			return m, cmd
		}
		var c tea.Cmd
		m.installURL, c = m.installURL.Update(msg)
		return m, c
	case modeInstallPick:
		switch key {
		case "esc", "q":
			cmd := m.closeInstall()
			return m, cmd
		case "enter":
			cmd := m.chooseInstall()
			return m, cmd
		case "j", "down":
			m.moveInstall(1)
			return m, nil
		case "k", "up":
			m.moveInstall(-1)
			return m, nil
		case " ", "x":
			m.toggleInstall()
			return m, nil
		case "a":
			// Toggle all, which is the common case for a repo of skills
			// the user wants wholesale.
			all := true
			for _, c := range m.installCands {
				if !c.Selected {
					all = false
				}
			}
			for i := range m.installCands {
				m.installCands[i].Selected = !all
			}
			return m, nil
		case "g", "home":
			m.installIdx = 0
			return m, nil
		case "G", "end":
			m.installIdx = len(m.installCands) - 1
			return m, nil
		}
		return m, nil
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
		m.selectAt(0)
	case "G", "end":
		m.selectLast()
	case "home":
		m.selectAt(0)
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
	case "i":
		cmd := m.openInstallInput()
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
	m.selectAt(m.cursorIndex() + d)
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
	// The old code built a Skill here with the template body, a
	// description and no issues, and then never used it. The reload that
	// follows the editor re-reads the file, so the real values win.
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
	// The restored skill may not be back yet; applySelection picks it up
	// once the reload lands, which is why this no longer walks the list
	// by hand looking for a name that is not there (review F7).
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

// --- install ---

// openInstallInput is the `i` key: a URL prompt, nothing else. The clone
// does not start until the URL is submitted, so a half-typed URL costs
// nothing.
func (m *Model) openInstallInput() tea.Cmd {
	m.appMode = modeInstallInput
	m.installURL.SetValue("")
	m.installURL.Focus()
	return nil
}

// closeInstall leaves the install flow and removes the temp clone. The
// clone is a real directory on disk, so cancelling has to clean it up
// rather than leave it for the next boot.
func (m *Model) closeInstall() tea.Cmd {
	cmd := removeTempTree(m.installSrc)
	m.installSrc = ""
	m.installCands = nil
	m.installIdx = 0
	m.installBusy = false
	m.installURL.Blur()
	m.installURL.SetValue("")
	m.appMode = modeNormal
	return cmd
}

// startInstall validates the URL and clones. The clone runs in a command
// with a timeout, so a repository that never answers cannot wedge the UI,
// and Ctrl-C still quits because it is handled before any mode.
func (m *Model) startInstall() tea.Cmd {
	raw := strings.TrimSpace(m.installURL.Value())
	url, err := validateGitURL(raw)
	if err != nil {
		m.installURL.SetValue("")
		return toastCmd("install: "+err.Error(), true)
	}
	ref, err := validateRef(m.installRef)
	if err != nil {
		m.installURL.SetValue("")
		return toastCmd("install: "+err.Error(), true)
	}
	m.installBusy = true
	m.installURL.Blur()
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	return tea.Batch(
		func() tea.Msg {
			defer cancel()
			src, cerr := cloneRepo(ctx, url, ref)
			if cerr != nil {
				return installClonedMsg{err: cerr}
			}
			cands, ferr := findCandidates(filepath.Join(src, "repo"))
			if ferr != nil {
				os.RemoveAll(src)
				return installClonedMsg{err: ferr}
			}
			return installClonedMsg{src: src, cands: cands}
		},
		m.spinner.Tick,
	)
}

// chooseInstall copies the ticked candidates and reloads. The clone is
// removed either way: it is a staging area, not a cache.
func (m *Model) chooseInstall() tea.Cmd {
	if len(m.installCands) == 0 {
		return m.closeInstall()
	}
	src, cands := m.installSrc, append([]installCandidate(nil), m.installCands...)
	from := ""
	if sel := m.selected(); sel != nil {
		from = sel.Name
	}
	m.installBusy = true
	return tea.Batch(
		func() tea.Msg {
			res, err := install(cands, filepath.Join(src, "repo"), false)
			return installDoneMsg{res: res, src: src, err: err, from: from}
		},
		m.spinner.Tick,
	)
}

func (m *Model) toggleInstall() {
	if m.installIdx < 0 || m.installIdx >= len(m.installCands) {
		return
	}
	c := &m.installCands[m.installIdx]
	c.Selected = !c.Selected
}

func (m *Model) moveInstall(d int) {
	if len(m.installCands) == 0 {
		return
	}
	m.installIdx += d
	if m.installIdx < 0 {
		m.installIdx = 0
	}
	if m.installIdx >= len(m.installCands) {
		m.installIdx = len(m.installCands) - 1
	}
}

// removeTempTree deletes a temp clone. It runs as a command so the
// directory removal is not on the update path.
func removeTempTree(dir string) tea.Cmd {
	if dir == "" {
		return nil
	}
	return func() tea.Msg {
		if err := os.RemoveAll(dir); err != nil {
			logf("install cleanup: %v", err)
			return installClonedMsg{err: fmt.Errorf("could not clean up %s: %w", dir, err)}
		}
		return nil
	}
}
