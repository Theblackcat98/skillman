package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// update.go is where the mode dispatch, the timer generations, the key
// routing and the destructive actions live, and it had no tests at all
// (review G1). These drive the model the way a user does: send a
// tea.KeyMsg, look at the resulting state.
//
// Nothing here asserts on a rendered frame except where the frame is
// the contract. golden_test.go owns that.

// typing sends each rune of s as a key press.
func typing(t *testing.T, m *Model, s string) {
	t.Helper()
	for _, r := range s {
		send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// enter, esc, tab and the other named keys.
func key(t *testing.T, m *Model, k tea.KeyType) {
	t.Helper()
	send(t, m, tea.KeyMsg{Type: k})
}

// quitRequested reports whether the command returned by the last Update
// is tea.Quit, without running the program.
func quitRequested(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// applyCmd runs a command's message back through Update, the way the
// Bubble Tea event loop does. Asserting on the model without doing this
// is how a test can pass while the user sees nothing: a toast is a
// message on its way, not a field that has been set yet.
func applyCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); isQuit {
		return
	}
	send(t, m, msg)
}

func TestFilterModeLifecycle(t *testing.T) {
	m := newTestModel(t, 80, 24, true)

	send(t, &m, keyPress('/'))
	if m.appMode != modeFilter {
		t.Fatalf("/ did not enter filter mode, got %d", m.appMode)
	}
	typing(t, &m, "skill-1")
	if !strings.Contains(m.query, "skill-1") {
		t.Errorf("query = %q, want it to contain skill-1", m.query)
	}
	if len(m.filtered) == 0 || len(m.filtered) >= len(m.skills) {
		t.Errorf("filter matched %d of %d skills", len(m.filtered), len(m.skills))
	}

	// Esc keeps the query: the filter is a way to search, not a modal
	// that discards what you typed.
	key(t, &m, tea.KeyEsc)
	if m.appMode != modeNormal {
		t.Errorf("esc did not leave filter mode, got %d", m.appMode)
	}
	if m.query == "" {
		t.Error("esc cleared the query; it should keep it")
	}
	// The second esc clears it.
	send(t, &m, keyPress('j'))
	key(t, &m, tea.KeyEsc)
	if m.query != "" {
		t.Errorf("second esc left query = %q", m.query)
	}
	if len(m.filtered) != len(m.skills) {
		t.Errorf("clearing the filter left %d of %d skills", len(m.filtered), len(m.skills))
	}
}

func TestFilterEnterKeepsQuery(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	send(t, &m, keyPress('/'))
	typing(t, &m, "skill-1")
	key(t, &m, tea.KeyEnter)
	if m.appMode != modeNormal {
		t.Errorf("enter did not leave filter mode")
	}
	if m.query == "" {
		t.Error("enter cleared the query")
	}
}

func TestCommandModeRunsAndCancels(t *testing.T) {
	m := newTestModel(t, 80, 24, true)

	// Unknown command: a toast, not a crash.
	send(t, &m, keyPress(':'))
	typing(t, &m, "bogus")
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(Model)
	applyCmd(t, &m, cmd)
	if m.appMode != modeNormal {
		t.Errorf("enter did not leave command mode")
	}
	if !strings.Contains(m.toast, "unknown command") {
		t.Errorf("toast = %q, want an unknown-command message", m.toast)
	}

	// Esc cancels and clears the line, so the next command starts fresh.
	send(t, &m, keyPress(':'))
	typing(t, &m, "clear")
	key(t, &m, tea.KeyEsc)
	if m.appMode != modeNormal {
		t.Errorf("esc did not leave command mode")
	}
	if m.cmdline.Value() != "" {
		t.Errorf("esc left the command line = %q", m.cmdline.Value())
	}
}

func TestCommandClearAndFilter(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.query = "skill-1"
	m.filter.SetValue("skill-1")
	m.applyFilter()
	send(t, &m, keyPress(':'))
	typing(t, &m, "clear")
	key(t, &m, tea.KeyEnter)
	if m.query != "" {
		t.Errorf(": clear left query = %q", m.query)
	}
}

func TestHelpOverlayClosesOnEveryKey(t *testing.T) {
	for _, closer := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyEsc},
		keyPress('?'),
		keyPress('q'),
		tea.KeyMsg{Type: tea.KeyEnter},
	} {
		m := newTestModel(t, 80, 24, true)
		send(t, &m, keyPress('?'))
		if m.appMode != modeHelp {
			t.Fatalf("? did not open help")
		}
		send(t, &m, closer)
		if m.appMode != modeNormal {
			t.Errorf("%v did not close the help overlay", closer)
		}
	}
}

func TestHelpOverlayDropsOtherKeys(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	send(t, &m, keyPress('?'))
	// d while help is open must not open the confirm modal.
	send(t, &m, keyPress('d'))
	if m.appMode != modeHelp {
		t.Errorf("d acted behind the help overlay, mode = %d", m.appMode)
	}
}

func TestCtrlCQuitsFromEveryLayer(t *testing.T) {
	for _, mode := range []mode{modeNormal, modeFilter, modeCommand, modeHelp, modeConfirm} {
		m := newTestModel(t, 80, 24, true)
		m.appMode = mode
		mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m = mm.(Model)
		if !quitRequested(cmd) {
			t.Errorf("ctrl+c in mode %d did not quit", mode)
		}
	}
}

func TestQuitFromNormalMode(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	mm, cmd := m.Update(keyPress('q'))
	m = mm.(Model)
	if !quitRequested(cmd) {
		t.Error("q did not quit from the base layer")
	}
	if m.appMode != modeNormal {
		t.Error("q changed the mode")
	}
}

// The confirm modal is the gate on a destructive action: y goes through,
// n and Esc back out, and nothing else does.
func TestConfirmModalGate(t *testing.T) {
	t.Run("n cancels", func(t *testing.T) {
		m := newTestModel(t, 80, 24, true)
		m.skills = fixtureSkills(t, 2)
		m.applyFilter()
		send(t, &m, keyPress('d'))
		if m.appMode != modeConfirm {
			t.Fatalf("d did not open the confirm modal")
		}
		send(t, &m, keyPress('n'))
		if m.appMode != modeNormal {
			t.Errorf("n left mode = %d", m.appMode)
		}
		if len(m.skills) != 2 {
			t.Errorf("n deleted something: %d skills left", len(m.skills))
		}
	})
	t.Run("esc cancels", func(t *testing.T) {
		m := newTestModel(t, 80, 24, true)
		m.skills = fixtureSkills(t, 2)
		m.applyFilter()
		send(t, &m, keyPress('d'))
		key(t, &m, tea.KeyEsc)
		if m.appMode != modeNormal || len(m.skills) != 2 {
			t.Errorf("esc did not cleanly cancel: mode %d, %d skills", m.appMode, len(m.skills))
		}
	})
	t.Run("y deletes", func(t *testing.T) {
		skills, _ := isolatedEnv(t)
		m := newTestModel(t, 80, 24, true)
		m.skills = fixtureSkills(t, 2)
		m.applyFilter()
		name := m.selected().Name
		dir := m.selected().Dir
		send(t, &m, keyPress('d'))
		applyCmd(t, &m, m.doDelete())
		if m.appMode != modeNormal {
			t.Errorf("y left mode = %d", m.appMode)
		}
		if !m.undo.Active {
			t.Error("y did not open an undo window")
		}
		if m.undo.Name != name {
			t.Errorf("undo is for %q, want %q", m.undo.Name, name)
		}
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			t.Errorf("%s is still in the skills dir after y", name)
		}
		// And it really is in the trash, under the temp data dir.
		if !strings.HasPrefix(m.undo.TrashPath, trashDir()) {
			t.Errorf("trash path %q is not under %q", m.undo.TrashPath, trashDir())
		}
		_ = skills
	})
}

// d and u are half-page scroll keys inside a viewport. With the preview
// focused they must scroll, never delete (review B3).
func TestPreviewFocusNeverDeletes(t *testing.T) {
	isolatedEnv(t)
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 3)
	m.applyFilter()
	m.focusPreview = true

	for _, k := range []rune{'d', 'u'} {
		before := len(m.skills)
		undoBefore := m.undo.Active
		send(t, &m, keyPress(k))
		if len(m.skills) != before {
			t.Errorf("%q changed the skill list while the preview was focused", k)
		}
		if m.undo.Active != undoBefore {
			t.Errorf("%q started an undo while the preview was focused", k)
		}
		if m.appMode == modeConfirm {
			t.Errorf("%q opened the confirm modal from the preview pane", k)
		}
	}
}

func TestPreviewFocusRoutesNavToViewport(t *testing.T) {
	m := longBodyModel(t, 80, 24)
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab})
	if !m.focusPreview {
		t.Fatal("tab did not focus the preview")
	}
	listCursor := m.cursorIndex()
	send(t, &m, keyPress('j'))
	send(t, &m, keyPress('j'))
	if m.preview.YOffset == 0 {
		t.Error("j did not scroll the preview")
	}
	if m.cursorIndex() != listCursor {
		t.Errorf("j moved the list cursor %d -> %d with the preview focused", listCursor, m.cursorIndex())
	}
	// g/G jump within the body, not within the list.
	send(t, &m, keyPress('G'))
	if !m.preview.AtBottom() {
		t.Error("G did not reach the end of the body")
	}
	if m.cursorIndex() != listCursor {
		t.Errorf("G moved the list cursor to %d", m.cursorIndex())
	}
}

func TestUndoWithoutPendingDelete(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	mm, cmd := m.Update(keyPress('u'))
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("u with nothing pending returned no command")
	}
	if msg, ok := cmd().(toastMsg); !ok || !msg.isErr || !strings.Contains(msg.text, "nothing to undo") {
		t.Errorf("u with nothing pending produced %v", cmd())
	}
}

func TestUndoRestoresTheDeletedSkill(t *testing.T) {
	isolatedEnv(t)
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 2)
	m.applyFilter()
	name := m.selected().Name

	send(t, &m, keyPress('d'))
	applyCmd(t, &m, m.doDelete())
	if m.undo.Name != name {
		t.Fatalf("undo is for %q, want %q", m.undo.Name, name)
	}
	mm, cmd := m.Update(keyPress('u'))
	m = mm.(Model)
	applyCmd(t, &m, cmd)
	if m.undo.Active {
		t.Error("u left the undo window open")
	}
	found := false
	for _, s := range m.skills {
		if s.Name == name {
			found = true
		}
	}
	if !found {
		t.Errorf("%s is not back in the list after undo; list is %v", name, skillNames(m.skills))
	}
	if m.selected() == nil || m.selected().Name != name {
		t.Errorf("the cursor did not return to the restored skill, it is on %v", m.selected())
	}
}

func skillNames(skills []Skill) []string {
	out := make([]string, 0, len(skills))
	for _, s := range skills {
		out = append(out, s.Name)
	}
	return out
}

// A timer from an older toast must not clear a newer one (review B6).
func TestToastTimerGenerations(t *testing.T) {
	m := newTestModel(t, 80, 24, true)

	first := m.setToast("first", false)
	seq1 := m.toastSeq
	_ = first
	m.setToast("second", false)
	seq2 := m.toastSeq
	if seq1 == seq2 {
		t.Fatal("the toast generation did not advance")
	}
	// The first toast's timer fires late.
	send(t, &m, toastClearMsg{seq: seq1})
	if m.toast != "second" {
		t.Errorf("a stale timer cleared the newer toast: %q", m.toast)
	}
	// The current toast's timer works.
	send(t, &m, toastClearMsg{seq: seq2})
	if m.toast != "" {
		t.Errorf("the current timer did not clear the toast: %q", m.toast)
	}
}

// A 30s undo timer from an earlier delete must not expire a newer one.
func TestUndoTimerGenerations(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.undo = pendingUndo{Name: "first", Active: true}
	m.undoSeq = 1
	m.undo = pendingUndo{Name: "second", Active: true}
	m.undoSeq = 2

	send(t, &m, undoExpireMsg{seq: 1})
	if !m.undo.Active {
		t.Error("a stale undo timer expired the newer window")
	}
	send(t, &m, undoExpireMsg{seq: 2})
	if m.undo.Active {
		t.Error("the current undo timer did not expire the window")
	}
}

func TestTimerCommandsAreScheduled(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	// A non-nil command is enough: the timers are three and thirty
	// seconds, and waiting for them in a test would be absurd.
	if cmd := m.setToast("hi", false); cmd == nil {
		t.Error("setToast scheduled no dismissal, so the toast would stick forever")
	}
	// This is the bug the command is there for: with animations off,
	// success toasts used to never be scheduled at all.
	if m.toast != "hi" {
		t.Error("setToast did not set the text")
	}
}

func TestNavigationKeys(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	n := len(m.filtered)

	send(t, &m, keyPress('G'))
	if m.cursorIndex() != n-1 {
		t.Errorf("G put the cursor at %d, want %d", m.cursorIndex(), n-1)
	}
	send(t, &m, keyPress('g'))
	if m.cursorIndex() != 0 {
		t.Errorf("g put the cursor at %d, want 0", m.cursorIndex())
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.cursorIndex() != n-1 {
		t.Errorf("end put the cursor at %d, want %d", m.cursorIndex(), n-1)
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyHome})
	if m.cursorIndex() != 0 {
		t.Errorf("home put the cursor at %d, want 0", m.cursorIndex())
	}
	send(t, &m, keyPress('j'))
	if m.cursorIndex() != 1 {
		t.Errorf("j put the cursor at %d, want 1", m.cursorIndex())
	}
	// Past the end: clamped, not wrapped, and the frame does not panic.
	for i := 0; i < n+30; i++ {
		send(t, &m, keyPress('j'))
	}
	if m.cursorIndex() != n-1 {
		t.Errorf("j past the end put the cursor at %d, want %d", m.cursorIndex(), n-1)
	}
	for i := 0; i < n+30; i++ {
		send(t, &m, keyPress('k'))
	}
	if m.cursorIndex() != 0 {
		t.Errorf("k past the start put the cursor at %d, want 0", m.cursorIndex())
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursorIndex() != 10 {
		t.Errorf("pgdown put the cursor at %d, want 10", m.cursorIndex())
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursorIndex() != 0 {
		t.Errorf("pgup put the cursor at %d, want 0", m.cursorIndex())
	}
}

func TestNavigationOnEmptyList(t *testing.T) {
	m := NewModel(true, true, Config{Accent: defaultAccent})
	send(t, &m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.ready = true
	m.loading = false
	m.skills = nil
	m.applyFilter()

	// None of these may panic or leave a negative cursor.
	for _, k := range []tea.Msg{keyPress('j'), keyPress('k'), keyPress('G'), keyPress('g'),
		tea.KeyMsg{Type: tea.KeyEnd}, tea.KeyMsg{Type: tea.KeyHome},
		tea.KeyMsg{Type: tea.KeyPgDown}, tea.KeyMsg{Type: tea.KeyPgUp}} {
		send(t, &m, k)
	}
	// -1 means "no row": the old index stayed 0, which pointed at
	// nothing and read as a real position to every caller.
	if m.cursorIndex() != -1 {
		t.Errorf("cursorIndex = %d on an empty list, want -1 (no row)", m.cursorIndex())
	}
	if m.selected() != nil {
		t.Error("selected() returned a skill on an empty list")
	}
	if !strings.Contains(m.View(), "no skills") {
		t.Error("the empty-list state does not say so")
	}
}

func TestDeleteWithNoSelectionIsNotSilent(t *testing.T) {
	m := NewModel(true, true, Config{Accent: defaultAccent})
	send(t, &m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.ready = true
	m.loading = false
	m.applyFilter()

	mm, cmd := m.Update(keyPress('d'))
	m = mm.(Model)
	if m.appMode == modeConfirm {
		t.Error("d with nothing selected opened the confirm modal")
	}
	if msg, ok := cmd().(toastMsg); !ok || !strings.Contains(msg.text, "nothing to delete") {
		t.Errorf("d with nothing selected produced %v, want a toast", cmd())
	}
}

func TestTabAndEnterToggleFocus(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab})
	if !m.focusPreview {
		t.Error("tab did not focus the preview")
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focusPreview {
		t.Error("tab did not unfocus the preview")
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.focusPreview {
		t.Error("enter did not focus the preview")
	}
}

// Below the split breakpoint tab switches panes; at or above it, tab
// moves focus inside the two-pane layout.
func TestNarrowTabSwitchesPane(t *testing.T) {
	m := newTestModel(t, 55, 24, true)
	if m.showPreviewOv {
		t.Fatal("narrow model started on the preview")
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab})
	if !m.showPreviewOv {
		t.Error("tab did not switch to the preview below 70 columns")
	}
	if m.focusPreview {
		t.Error("narrow tab set focus instead of switching panes")
	}
}

func TestResizeClampsAndRelaysOut(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	// Below the floor: clamped, not a division by zero or a panic.
	send(t, &m, tea.WindowSizeMsg{Width: 1, Height: 1})
	if m.width < 20 || m.height < 10 {
		t.Errorf("tiny terminal clamped to %dx%d, want at least 20x10", m.width, m.height)
	}
	frame := m.View()
	for i, ln := range strings.Split(frame, "\n") {
		if lipWidth(ln) > m.width {
			t.Errorf("after clamp to %dx%d, line %d is %d cells", m.width, m.height, i+1, lipWidth(ln))
		}
	}
}

func TestSkillsLoadedReplacesTheList(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.selectAt(5)
	// A reload with a different set must move the list, not append to it.
	send(t, &m, skillsLoadedMsg{skills: makeSkills(3)})
	if len(m.skills) != 3 {
		t.Errorf("reload left %d skills, want 3", len(m.skills))
	}
	if m.cursorIndex() >= len(m.skills) {
		t.Errorf("cursor %d is out of range after a shrinking reload", m.cursorIndex())
	}
}

func TestScanErrorIsSurfaced(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	mm, cmd := m.Update(skillsLoadedMsg{err: os.ErrPermission})
	m = mm.(Model)
	if m.loading {
		t.Error("the model is still loading after a scan error")
	}
	if m.errMsg == "" {
		t.Error("a scan error was not surfaced in the model")
	}
	if !strings.Contains(m.View(), "scan failed") && !strings.Contains(m.toast, "scan failed") {
		t.Error("a scan error produced no visible feedback")
	}
	_ = cmd
}

// v used to produce a toast only, so the badges on screen came from the
// previous scan while the numbers in the toast came from a fresh one
// (review F9). It now reloads the list it is reporting on.
func TestValidateRefreshesTheListItReportsOn(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "good", "---\nname: good\ndescription: d\n---\n\nbody\n")
	writeSkill(t, skills, "bad", "no frontmatter here\n")

	m := newTestModel(t, 80, 24, true)
	// Start from a stale list, as if the last scan predated bad/.
	m.skills = []Skill{{Name: "stale", Desc: "d", Dir: "/nonexistent/stale"}}
	m.applyFilter()

	mm, cmd := m.Update(keyPress('v'))
	m = mm.(Model)
	if cmd == nil {
		t.Fatal("v produced no command")
	}
	msg, ok := cmd().(validateMsg)
	if !ok {
		t.Fatalf("v produced %T, want a validateMsg that carries a scan", cmd())
	}
	if !strings.Contains(msg.text, "1 warn") {
		t.Errorf("summary = %q, want it to count the broken skill", msg.text)
	}
	if len(msg.skills) != 2 {
		t.Errorf("validate carried %d skills, want 2", len(msg.skills))
	}
	// And the model takes that list, so the badge on screen matches.
	send(t, &m, msg)
	if len(m.skills) != 2 {
		t.Errorf("after v the model shows %d skills, want 2", len(m.skills))
	}
	if !strings.Contains(m.View(), "1 warn") {
		t.Errorf("the frame does not carry the summary: %q", m.toast)
	}
}

func TestRescanSetsLoading(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	send(t, &m, keyPress('r'))
	if !m.loading {
		t.Error("r did not set the loading flag")
	}
	if !strings.Contains(m.View(), "scanning") {
		t.Error("the frame does not show that a scan is running")
	}
}

func TestEditWithNoSelectionToasts(t *testing.T) {
	m := NewModel(true, true, Config{Accent: defaultAccent})
	send(t, &m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.ready = true
	m.loading = false
	m.applyFilter()
	mm, cmd := m.Update(keyPress('e'))
	m = mm.(Model)
	if msg, ok := cmd().(toastMsg); !ok || !strings.Contains(msg.text, "nothing to edit") {
		t.Errorf("e with nothing selected produced %v", cmd())
	}
}

// Esc from the base layer unfocuses the preview before it does anything
// else, and does nothing at all when there is nothing to undo.
func TestEscOrder(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.focusPreview = true
	send(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.focusPreview {
		t.Error("esc did not unfocus the preview")
	}
	m.focusPreview = false
	m.query = "x"
	m.filter.SetValue("x")
	m.applyFilter()
	send(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.query != "" {
		t.Error("esc did not clear the query when nothing else was pending")
	}
}

// Moving the list cursor must not throw away where the reader was in the
// outgoing skill's body (review A10).
func TestCursorMovePreservesPerSkillScroll(t *testing.T) {
	m := longBodyModel(t, 80, 24)
	m.skills = append(m.skills, Skill{
		Name: "other", Desc: "another", Body: "short body", Dir: "/nonexistent/other",
	})
	m.applyFilter()

	send(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // focus preview
	send(t, &m, keyPress('G'))                // bottom of longbody
	deep := m.preview.YOffset
	if deep == 0 {
		t.Fatal("setup: G did not scroll")
	}
	send(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // back to the list
	send(t, &m, keyPress('j'))                // move to "other"
	if m.selected().Name != "other" {
		t.Fatalf("cursor is on %q, want other", m.selected().Name)
	}
	send(t, &m, keyPress('k')) // and back to longbody
	if m.selected().Name != "longbody" {
		t.Fatalf("cursor is on %q, want longbody", m.selected().Name)
	}
	if m.preview.YOffset == 0 {
		t.Errorf("returning to a skill lost the scroll position (was %d)", deep)
	}
}

// The preview cache is keyed on name and width only, so any reload has
// to drop it or an edit shows stale content (review A11). The assertion
// is on freshness, not on emptiness: after a reload the current
// selection is legitimately re-rendered once.
func TestPreviewCacheIsDroppedOnReload(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "live", "---\nname: live\ndescription: d\n---\n\nORIGINAL BODY\n")

	m := newTestModel(t, 80, 24, true)
	list, err := ScanSkills(skills)
	if err != nil {
		t.Fatal(err)
	}
	m.skills = list
	m.applyFilter()
	if !strings.Contains(m.preview.View(), "ORIGINAL BODY") {
		t.Fatalf("setup: preview does not show the body: %q", m.preview.View())
	}

	// Edit the file, then reload the way `r` or a post-edit reload does.
	writeSkill(t, skills, "live", "---\nname: live\ndescription: d\n---\n\nEDITED BODY\n")
	send(t, &m, skillsLoadedMsg{skills: mustScan(t, skills)})
	if strings.Contains(m.preview.View(), "ORIGINAL BODY") {
		t.Errorf("the preview still shows the pre-edit body:\n%s", m.preview.View())
	}
	if !strings.Contains(m.preview.View(), "EDITED BODY") {
		t.Errorf("the preview does not show the new body:\n%s", m.preview.View())
	}
}

// Delete and undo rewrite the list, so they must drop the cache: the
// skill is gone, and the cache key is name plus width only, so a render
// made before the delete can be served for a different skill that later
// takes the name (review A11). Both now go through reloadCmd, which is
// asynchronous, so the assertion is on the invalidation plus the reload
// being scheduled rather than on the list having changed yet.
func TestDeleteAndUndoDropTheCacheAndReload(t *testing.T) {
	isolatedEnv(t)
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 2)
	m.applyFilter()
	_ = m.cachedPreview(m.skills[0], 40)
	if len(m.previewCache) == 0 {
		t.Fatal("setup: nothing cached")
	}

	cmd := m.doDelete()
	if cmd == nil {
		t.Fatal("delete scheduled nothing")
	}
	if len(m.previewCache) != 0 {
		t.Errorf("delete left %d cached renders", len(m.previewCache))
	}
	if !m.loading {
		t.Error("delete did not set the loading flag; the reload is not scheduled")
	}
	if m.undo.Name == "" || !m.undo.Active {
		t.Error("delete did not open an undo window")
	}
	// The skill is gone from disk right away; the list catches up when
	// the reload lands.
	if _, err := os.Stat(m.undo.TrashPath); err != nil {
		t.Errorf("the skill is not in the trash: %v", err)
	}

	// Undo restores it, invalidates again, and remembers where to put
	// the cursor.
	_ = m.doUndo()
	if len(m.previewCache) != 0 {
		t.Errorf("undo left %d cached renders", len(m.previewCache))
	}
	if m.pendingSelection != m.undo.Name && m.undo.Name != "" {
		t.Error("undo did not record where the cursor should go")
	}
}

func mustScan(t *testing.T, dir string) []Skill {
	t.Helper()
	list, err := ScanSkills(dir)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// fixtureSkills builds real skill directories under the isolated skills
// dir, so a test that deletes really deletes something. It never touches
// the developer's own skills directory.
func fixtureSkills(t *testing.T, n int) []Skill {
	t.Helper()
	skills, _ := isolatedEnv(t)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("skill-%02d", i)
		md := "---\nname: " + name + "\ndescription: fixture\n---\n\nbody of " + name + "\n"
		writeSkill(t, skills, name, md)
	}
	list, err := ScanSkills(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("built %d fixture skills, want %d", len(list), n)
	}
	return list
}

// A session that runs for a while must not accumulate preview renders
// without bound (review D2).
func TestPreviewCacheIsBounded(t *testing.T) {
	m := longBodyModel(t, 80, 24)
	for i := 0; i < previewCacheLimit*3; i++ {
		s := Skill{
			Name: "gen-" + itoaTest(i), Desc: "d",
			Body: "line\nline\nline\n", Dir: "/nonexistent",
		}
		m.cachedPreview(s, 40)
	}
	if len(m.previewCache) > previewCacheLimit {
		t.Errorf("preview cache holds %d renders, limit is %d", len(m.previewCache), previewCacheLimit)
	}
}

func TestToastAndUndoDoNotRaceTheModel(t *testing.T) {
	// A cheap smoke test that the timers are tea.Tick commands and not
	// inline sleeps, so a fast session cannot block on them.
	m := newTestModel(t, 80, 24, true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			mm, cmd := m.Update(toastMsg{text: "t", isErr: false})
			m = mm.(Model)
			if cmd != nil {
				_ = cmd
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("toast handling blocked; a timer is being waited on inline")
	}
}

// e on a skill with no SKILL.md used to open a directory listing, so
// saving could not create the file and nothing said why (review E10).
func TestEditAsksBeforeCreatingSkillMD(t *testing.T) {
	isolatedEnv(t)
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 2)
	m.applyFilter()
	// Remove the file behind the selected skill.
	sel := m.selected()
	if err := os.Remove(filepath.Join(sel.Dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	// The model still thinks the file is fine; the filesystem is the
	// authority, and the check happens on the edit path.
	send(t, &m, keyPress('e'))
	if m.appMode != modeConfirmCreate {
		t.Fatalf("e on a skill with no SKILL.md put the model in mode %d, want the create confirmation", m.appMode)
	}
	view := m.View()
	if !strings.Contains(view, "no SKILL.md") || !strings.Contains(view, "Create one") {
		t.Errorf("the create modal does not say what it will do:\n%s", view)
	}

	// n creates nothing.
	send(t, &m, keyPress('n'))
	if m.appMode != modeNormal {
		t.Errorf("n left mode %d", m.appMode)
	}
	if _, err := os.Stat(filepath.Join(sel.Dir, "SKILL.md")); err == nil {
		t.Error("n created SKILL.md anyway")
	}
}

func TestEditOnExistingFileGoesStraightToTheEditor(t *testing.T) {
	isolatedEnv(t)
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 1)
	m.applyFilter()
	// A no-op editor, so ExecProcess returns immediately.
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "")
	mm, cmd := m.Update(keyPress('e'))
	m = mm.(Model)
	if m.appMode == modeConfirmCreate {
		t.Error("e on a skill that has SKILL.md asked to create one")
	}
	if cmd == nil {
		t.Fatal("e on an existing file produced no command")
	}
}

// The selection was an index into the filtered list, so anything that
// rebuilt the list moved the cursor onto a different skill while the
// highlighted row stayed put. Selection is a name now (review F7).
func TestSelectionSurvivesAReorder(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 6)
	m.applyFilter()
	m.selectName("skill-03")
	if got := m.selected().Name; got != "skill-03" {
		t.Fatalf("setup: selected %q", got)
	}

	// A reload that renames skill-01 to skill-99, deleting skill-05 and
	// appending a new skill. Every index below 3 shifts.
	next := []Skill{
		m.skills[1], m.skills[2], m.skills[3], m.skills[5],
		{Name: "skill-99", Desc: "renamed", Dir: "/tmp/skill-99"},
	}
	mm, _ := m.Update(skillsLoadedMsg{skills: next})
	m = mm.(Model)

	if got := m.selected().Name; got != "skill-03" {
		t.Errorf("after a reorder the selection is %q, want skill-03 (index %d)",
			got, m.cursorIndex())
	}
	// The highlighted row is the selected skill, not just row 3.
	frame := m.View()
	if !strings.Contains(frame, "> skill-03") {
		t.Errorf("the marker is not on the selected skill:\n%s", frame)
	}
}

// A filter that hides the selection must not lose it: the user is about
// to clear the filter and should land back where they were.
func TestFilterDoesNotLoseTheSelection(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 8)
	m.applyFilter()
	m.selectName("skill-06")

	m.query = "skill-0"
	m.applyFilter()
	if got := m.selected().Name; got != "skill-06" {
		t.Errorf("with a matching filter the selection became %q", got)
	}

	// A filter that matches nothing cannot keep it visible, but the
	// stored name must survive so clearing the filter restores it.
	m.query = "zzz-no-match"
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Fatalf("setup: the filter should match nothing, got %d", len(m.filtered))
	}
	if m.selected() != nil {
		t.Errorf("an empty result selected %q", m.selected().Name)
	}
	m.query = ""
	m.applyFilter()
	if got := m.selected().Name; got != "skill-06" {
		t.Errorf("clearing the filter selected %q, want the remembered skill-06", got)
	}
}

// FilterSkills must not hand back the caller's slice. The model stores the
// result directly, so returning the original let any later change write
// through to the full list (review F6).
func TestFilterSkillsNeverAliasesItsInput(t *testing.T) {
	in := []Skill{{Name: "a", Desc: "one"}, {Name: "b", Desc: "two"}}
	out := FilterSkills(in, "")
	if &out[0] == &in[0] {
		t.Fatal("FilterSkills returned the caller's own slice")
	}
	out[0].Name = "mutated"
	if in[0].Name != "a" {
		t.Errorf("writing to the result changed the input: %q", in[0].Name)
	}
	// Appending must not grow into the caller's array either.
	out = append(out, Skill{Name: "c"})
	if len(in) != 2 {
		t.Errorf("appending to the result changed the input length to %d", len(in))
	}
	// And a real filter result is independent too.
	out = FilterSkills(in, "a")
	out[0].Desc = "mutated"
	if in[0].Desc != "one" {
		t.Errorf("a filtered result aliases the input: %q", in[0].Desc)
	}
}

// An undone delete has to put the cursor back on the skill that came back,
// which now happens through the saved name rather than a hand-written
// walk over the list.
func TestUndoSelectsTheRestoredSkill(t *testing.T) {
	dir, _ := isolatedEnv(t)
	for i := 1; i <= 4; i++ {
		writeSkill(t, dir, "skill-0"+string(rune('0'+i)), validFM("skill-0"+string(rune('0'+i)), "d"))
	}
	m := newTestModel(t, 80, 24, true)
	mm, _ := m.Update(skillsLoadedMsg{skills: mustScan(t, dir)})
	m = mm.(Model)
	m.selectName("skill-03")
	if len(m.filtered) != 4 {
		t.Fatalf("setup: %d skills, want 4", len(m.filtered))
	}

	_ = m.doDelete()
	if m.undo.Name != "skill-03" {
		t.Fatalf("undo armed for %q", m.undo.Name)
	}
	// The reload that brings it back.
	mm, _ = m.Update(skillsLoadedMsg{skills: mustScan(t, dir)})
	m = mm.(Model)
	_ = m.doUndo()
	mm, _ = m.Update(skillsLoadedMsg{skills: mustScan(t, dir)})
	m = mm.(Model)

	if got := m.selected().Name; got != "skill-03" {
		t.Errorf("after undo the selection is %q, want the restored skill-03", got)
	}
	if len(m.filtered) != 4 {
		t.Errorf("after undo there are %d skills, want 4", len(m.filtered))
	}
}
