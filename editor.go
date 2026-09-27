package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Open skill in $EDITOR/$VISUAL. The TUI suspends around it via
// tea.ExecProcess (see Model.doEdit), so the alt screen is restored.

// editorFor splits $VISUAL/$EDITOR into argv so flags like
// "code -w" work; falls back to vi.
func editorFor() []string {
	v := os.Getenv("VISUAL")
	if v == "" {
		v = os.Getenv("EDITOR")
	}
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return []string{"vi"}
	}
	return fields
}

func skillEditPath(s Skill) string {
	p := filepath.Join(s.Dir, "SKILL.md")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return s.Dir
}

// editorCmd builds the command ExecProcess will suspend the TUI for.
func editorCmd(path string) *exec.Cmd {
	ed := editorFor()
	return exec.Command(ed[0], append(ed[1:], path)...)
}
