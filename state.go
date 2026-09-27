package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Session state, persisted across restarts. Everything here is a
// convenience: a missing, stale or broken file must never stop skillman
// from starting, so load always returns something usable.

const stateSchema = 1

type State struct {
	Schema        int            `json:"schema"`
	LastSelection string         `json:"last_selection,omitempty"`
	PreviewScroll map[string]int `json:"preview_scroll,omitempty"`
	FocusPreview  bool           `json:"focus_preview"`
	SplitWidth    int            `json:"split_width,omitempty"`
}

func statePath() string { return filepath.Join(configRoot(), "state.json") }

func loadState() State {
	fresh := State{Schema: stateSchema, PreviewScroll: map[string]int{}}
	raw, err := os.ReadFile(statePath())
	if err != nil {
		return fresh
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		logf("state: unreadable, starting fresh: %v", err)
		return fresh
	}
	if s.Schema != stateSchema {
		// Written by another version. Start fresh rather than guess at
		// a layout this build does not know.
		logf("state: schema %d != %d, starting fresh", s.Schema, stateSchema)
		return fresh
	}
	if s.PreviewScroll == nil {
		s.PreviewScroll = map[string]int{}
	}
	return s
}

// saveState writes through a temp file and renames, so an interrupted
// write cannot leave a truncated file that fails to parse next time.
func saveState(s State) error {
	s.Schema = stateSchema
	if err := os.MkdirAll(configRoot(), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(configRoot(), "state-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// No-op once the rename below succeeds.
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, statePath())
}

// state snapshots the model for the next run.
func (m Model) state() State {
	s := State{
		Schema:        stateSchema,
		PreviewScroll: map[string]int{},
		FocusPreview:  m.focusPreview,
		SplitWidth:    m.width,
	}
	if sel := m.selected(); sel != nil {
		s.LastSelection = sel.Name
	}
	for name, off := range m.scroll {
		if off > 0 {
			s.PreviewScroll[name] = off
		}
	}
	if m.previewName != "" && m.preview.YOffset > 0 {
		s.PreviewScroll[m.previewName] = m.preview.YOffset
	}
	return s
}

// restoreState puts a saved session back. Selection is applied after the
// first scan, when the skill list exists.
func (m *Model) restoreState(s State) {
	m.scroll = map[string]int{}
	for name, off := range s.PreviewScroll {
		if off > 0 {
			m.scroll[name] = off
		}
	}
	m.focusPreview = s.FocusPreview
	m.pendingSelection = s.LastSelection
}

// applySelection moves the cursor to the skill named in the saved state or
// in an undo. A name that no longer exists is ignored: state is a
// convenience, never a reason to fail.
func (m *Model) applySelection() {
	if m.pendingSelection == "" {
		return
	}
	m.selectName(m.pendingSelection)
	m.pendingSelection = ""
}
