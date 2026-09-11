package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Paths and environment gates. No emojis.

func skillsDir() string {
	if v := os.Getenv("SKILLMAN_SKILLS"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode", "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "opencode", "skills")
	}
	return filepath.Join(home, ".config", "opencode", "skills")
}

func dataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "skillman")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".local", "share", "skillman")
	}
	return filepath.Join(home, ".local", "share", "skillman")
}

func trashDir() string { return filepath.Join(dataDir(), "trash") }
func logPath() string  { return filepath.Join(dataDir(), "skillman.log") }

func plainOutput(plainFlag bool) bool {
	if plainFlag {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return true
	}
	return false
}

func animationsEnabled(noAnimFlag bool) bool {
	if noAnimFlag {
		return false
	}
	if os.Getenv("NO_ANIMATIONS") != "" {
		return false
	}
	if os.Getenv("REDUCED_MOTION") != "" {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func logf(format string, args ...any) {
	dir := dataDir()
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	msg = strings.TrimRight(msg, "\n") + "\n"
	_, _ = fmt.Fprintf(f, "%s %s", ts, msg)
}
