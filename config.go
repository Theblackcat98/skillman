package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Paths and environment gates. No emojis.

const defaultAccent = "#7C6CFF"

// overridable maps a config key name to the built-in key that performs
// that action. A config override binds a new key to an action name; the
// dispatch switch stays the single owner of what each action does.
var overridable = map[string]string{
	"quit": "q", "help": "?", "filter": "/", "command": ":",
	"edit": "e", "delete": "d", "undo": "u", "validate": "v",
	"rescan": "r", "pane": "tab", "top": "g", "bottom": "G",
	"down": "j", "up": "k",
}

func overridableNames() string {
	names := make([]string, 0, len(overridable))
	for n := range overridable {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

// Config is the optional config.yaml. Every field has a working default,
// so a missing or partial file is not an error.
type Config struct {
	SkillsDir string            `yaml:"skills_dir"`
	Accent    string            `yaml:"accent"`
	Keys      map[string]string `yaml:"keys"`
}

func configRoot() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "skillman")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "skillman")
	}
	return filepath.Join(home, ".config", "skillman")
}

func configPath() string { return filepath.Join(configRoot(), "config.yaml") }

// loadConfig reads config.yaml. A missing file is not an error. A broken
// file returns defaults plus the error, so the caller can warn and still
// run rather than refusing to start.
func loadConfig() (Config, error) {
	cfg := Config{Accent: defaultAccent}
	raw, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{Accent: defaultAccent}, fmt.Errorf("%s: %w", configPath(), err)
	}
	if cfg.Accent == "" {
		cfg.Accent = defaultAccent
	}
	for name, key := range cfg.Keys {
		if _, ok := overridable[name]; !ok {
			logf("config: unknown key name %q (ignored); known: %s", name, overridableNames())
			delete(cfg.Keys, name)
			continue
		}
		if key == "" {
			logf("config: key %q has an empty binding (ignored)", name)
			delete(cfg.Keys, name)
		}
	}
	return cfg, nil
}

// keyOverrides maps a pressed key back to the built-in key for the action
// it was rebound to, so overrides need no dispatch code of their own.
func (c Config) keyOverrides() map[string]string {
	if len(c.Keys) == 0 {
		return nil
	}
	out := make(map[string]string, len(c.Keys))
	for name, key := range c.Keys {
		if def, ok := overridable[name]; ok {
			out[key] = def
		}
	}
	return out
}

// resolveKey translates a rebound key into its built-in equivalent.
func (c Config) resolveKey(key string) string {
	if def, ok := c.keyOverrides()[key]; ok {
		return def
	}
	return key
}

// apply resolves the skills directory once, so precedence is explicit
// and in one place: --skills-dir flag, then SKILLMAN_SKILLS, then
// config.yaml, then the XDG default that skillsDir() computes.
func (c Config) apply(flagDir string) {
	switch {
	case flagDir != "":
		_ = os.Setenv("SKILLMAN_SKILLS", flagDir)
	case os.Getenv("SKILLMAN_SKILLS") == "" && c.SkillsDir != "":
		_ = os.Setenv("SKILLMAN_SKILLS", c.SkillsDir)
	}
}

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
