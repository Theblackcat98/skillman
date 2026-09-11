package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Open skill in $EDITOR. Caller suspends the TUI around this.

func editorFor() string {
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "vi"
}

func skillEditPath(s Skill) string {
	p := filepath.Join(s.Dir, "SKILL.md")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return s.Dir
}

func openInEditor(path string) error {
	ed := editorFor()
	cmd := exec.Command(ed, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
