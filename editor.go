package main

import (
	"fmt"
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

// skillEditPath returns the file `e` should open, and whether it exists.
//
// The old version returned the skill directory when SKILL.md was absent,
// so `e` on a skill with no SKILL.md opened a file listing in $EDITOR:
// the user saw a directory, saving could not create the missing file, and
// nothing said why (review E10). Now the caller is told, and the fix is
// offered rather than guessed at.
func skillEditPath(s Skill) (string, bool) {
	p := filepath.Join(s.Dir, "SKILL.md")
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return p, false
}

// skillTemplate is what `e` offers to create for a skill that has no
// SKILL.md yet. The frontmatter keys are the ones ScanSkills reads, so
// the file is valid the moment it is saved.
const skillTemplate = `---
name: %s
description: one line, say what this skill is for
license: 
compatibility: 
metadata:
  category: 
---

# %s

What this skill does.

## When to use it

- 
`

// ensureSkillMD creates a starter SKILL.md when one is missing and
// reports whether it did. It is deliberately not automatic: a file the
// user did not ask for is its own kind of surprise, so the TUI asks
// first and the CLI says what to do instead.
func ensureSkillMD(s Skill) (string, bool, error) {
	path, exists := skillEditPath(s)
	if exists {
		return path, false, nil
	}
	name := s.Name
	content := fmt.Sprintf(skillTemplate, name, name)
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return path, false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return path, false, err
	}
	return path, true, nil
}

// editorCmd builds the command ExecProcess will suspend the TUI for.
func editorCmd(path string) *exec.Cmd {
	ed := editorFor()
	return exec.Command(ed[0], append(ed[1:], path)...)
}
