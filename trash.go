package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Trash-based delete with undo. Never rm -rf.

type pendingUndo struct {
	Name      string
	TrashPath string
	Active    bool
	ExpiresAt time.Time
}

func deleteSkillToTrash(s Skill) (string, error) {
	td := trashDir()
	if err := os.MkdirAll(td, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102-150405")
	dest := filepath.Join(td, fmt.Sprintf("%s-%s", s.Name, ts))
	if err := os.Rename(s.Dir, dest); err != nil {
		return "", err
	}
	logf("delete %s -> %s", s.Name, dest)
	return dest, nil
}

func undoDelete(name, trashPath string) error {
	dest := filepath.Join(skillsDir(), name)
	if _, err := os.Stat(dest); err == nil {
		dest = dest + "-restored"
	}
	if err := os.Rename(trashPath, dest); err != nil {
		return err
	}
	logf("undo %s <- %s", name, trashPath)
	return nil
}
