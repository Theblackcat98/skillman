package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// isolatedConfig points the config and state files at a temp dir.
func isolatedConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := isolatedConfig(t)
	if err := os.MkdirAll(filepath.Join(dir, "skillman"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skillman", "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDefaultsWhenMissing(t *testing.T) {
	isolatedConfig(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("missing config must not be an error: %v", err)
	}
	if cfg.Accent != defaultAccent {
		t.Errorf("accent = %q, want %q", cfg.Accent, defaultAccent)
	}
	if cfg.SkillsDir != "" || len(cfg.Keys) != 0 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestConfigRead(t *testing.T) {
	writeConfig(t, "skills_dir: /tmp/skills\naccent: \"#34D399\"\nkeys:\n  delete: X\n  quit: Q\n")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkillsDir != "/tmp/skills" {
		t.Errorf("skills_dir = %q", cfg.SkillsDir)
	}
	if cfg.Accent != "#34D399" {
		t.Errorf("accent = %q", cfg.Accent)
	}
	if got := cfg.resolveKey("X"); got != "d" {
		t.Errorf("resolveKey(X) = %q, want d", got)
	}
	if got := cfg.resolveKey("Q"); got != "q" {
		t.Errorf("resolveKey(Q) = %q, want q", got)
	}
	if got := cfg.resolveKey("j"); got != "j" {
		t.Errorf("unbound key changed: %q", got)
	}
}

func TestConfigBrokenYAMLFallsBackToDefaults(t *testing.T) {
	writeConfig(t, "accent: [unclosed\n")
	cfg, err := loadConfig()
	if err == nil {
		t.Fatal("want an error for broken config")
	}
	if cfg.Accent != defaultAccent {
		t.Errorf("broken config must fall back to the default accent, got %q", cfg.Accent)
	}
	if !strings.Contains(err.Error(), "config.yaml") {
		t.Errorf("error should name the file: %v", err)
	}
}

func TestConfigDropsBadKeyBindings(t *testing.T) {
	writeConfig(t, "keys:\n  nosuchaction: Z\n  delete: \"\"\n  undo: u\n")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Keys["nosuchaction"]; ok {
		t.Error("unknown key name must be dropped")
	}
	if _, ok := cfg.Keys["delete"]; ok {
		t.Error("empty binding must be dropped")
	}
	if _, ok := cfg.Keys["undo"]; !ok {
		t.Error("valid binding was dropped")
	}
}

// Precedence: flag beats env beats config beats the XDG default.
func TestConfigSkillsDirPrecedence(t *testing.T) {
	dir := isolatedConfig(t)
	if err := os.MkdirAll(filepath.Join(dir, "skillman"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "skills_dir: " + filepath.Join(dir, "from-config") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "skillman", "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("SKILLMAN_SKILLS", "")
	cfg.apply("")
	if want := filepath.Join(dir, "from-config"); skillsDir() != want {
		t.Errorf("config skills_dir ignored: got %q, want %q", skillsDir(), want)
	}

	envDir := filepath.Join(dir, "from-env")
	t.Setenv("SKILLMAN_SKILLS", envDir)
	cfg.apply("")
	if skillsDir() != envDir {
		t.Errorf("env must beat config: got %q, want %q", skillsDir(), envDir)
	}

	flagDir := filepath.Join(dir, "from-flag")
	cfg.apply(flagDir)
	if skillsDir() != flagDir {
		t.Errorf("flag must beat env: got %q, want %q", skillsDir(), flagDir)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := isolatedConfig(t)
	want := State{
		Schema:        stateSchema,
		LastSelection: "longbody",
		PreviewScroll: map[string]int{"longbody": 42},
		FocusPreview:  true,
		SplitWidth:    100,
	}
	if err := saveState(want); err != nil {
		t.Fatal(err)
	}
	got := loadState()
	if got.LastSelection != want.LastSelection || got.FocusPreview != want.FocusPreview || got.SplitWidth != want.SplitWidth {
		t.Errorf("state mismatch: %+v", got)
	}
	if got.PreviewScroll["longbody"] != 42 {
		t.Errorf("scroll lost: %+v", got.PreviewScroll)
	}
	// The atomic write must not leave temp files behind.
	entries, err := os.ReadDir(filepath.Join(dir, "skillman"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "state-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestStateUnusableFilesStartFresh(t *testing.T) {
	cases := map[string]string{
		"broken json":  "{not json",
		"wrong schema": `{"schema":99,"last_selection":"x"}`,
		"wrong shape":  `["a"]`,
	}
	for name, body := range cases {
		dir := isolatedConfig(t)
		if err := os.MkdirAll(filepath.Join(dir, "skillman"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(statePath(), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		got := loadState()
		if got.Schema != stateSchema || got.LastSelection != "" || got.PreviewScroll == nil {
			t.Errorf("%s: want a fresh usable state, got %+v", name, got)
		}
	}
}

func TestMissingStateIsFine(t *testing.T) {
	isolatedConfig(t)
	got := loadState()
	if got.Schema != stateSchema || got.PreviewScroll == nil {
		t.Errorf("missing state: %+v", got)
	}
}

// Per-skill scroll must survive a cursor move and come back intact.
func TestPreviewScrollIsPerSkill(t *testing.T) {
	m := longBodyModel(t, 80, 24)
	m.skills = append(m.skills, Skill{Name: "other", Body: "short body", Dir: "/nonexistent/other"})
	m.applyFilter()

	send(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // focus preview
	send(t, &m, keyPress('G'))                // to the end of longbody
	if !m.preview.AtBottom() {
		t.Fatalf("setup: not at the bottom")
	}
	first := m.preview.YOffset
	if first == 0 {
		t.Fatal("setup: expected a non-zero offset")
	}

	send(t, &m, tea.KeyMsg{Type: tea.KeyTab}) // back to the list
	send(t, &m, keyPress('j'))                // move the list cursor to "other"
	if m.previewName != "other" {
		t.Fatalf("selection did not move: %q", m.previewName)
	}
	if m.preview.YOffset != 0 {
		t.Errorf("new skill should start at the top, offset %d", m.preview.YOffset)
	}
	send(t, &m, keyPress('k')) // back to longbody
	if m.preview.YOffset != first {
		t.Errorf("scroll not remembered: %d, want %d", m.preview.YOffset, first)
	}
}

// state() must capture the live session, restoreState must replay it.
func TestModelStateRoundTrip(t *testing.T) {
	isolatedConfig(t)
	m := longBodyModel(t, 80, 24)
	m.focusPreview = true // G then goes to the preview, not the list
	send(t, &m, keyPress('G'))
	saved := m.state()
	if saved.LastSelection != "longbody" {
		t.Errorf("last selection = %q", saved.LastSelection)
	}
	if saved.PreviewScroll["longbody"] == 0 {
		t.Errorf("scroll not captured: %+v", saved)
	}
	if !saved.FocusPreview {
		t.Error("focus not captured")
	}

	// A fresh process restores the same session.
	next := NewModel(false, true, Config{Accent: defaultAccent})
	next.restoreState(saved)
	next.skills = longBodyModel(t, 80, 24).skills
	next.loading = false
	next.applyFilter()
	if sel := next.selected(); sel == nil || sel.Name != "longbody" {
		t.Errorf("selection not restored, got %v", sel)
	}
	if !next.focusPreview {
		t.Error("focus not restored")
	}
}

func TestRestoreStateIgnoresVanishedSkill(t *testing.T) {
	isolatedConfig(t)
	m := longBodyModel(t, 80, 24)
	saved := m.state()
	saved.LastSelection = "deleted-skill"
	next := NewModel(false, true, Config{Accent: defaultAccent})
	next.restoreState(saved)
	next.skills = longBodyModel(t, 80, 24).skills
	next.loading = false
	next.applyFilter() // must not panic or fail
	if next.pendingSelection != "" {
		t.Error("vanished selection should be dropped")
	}
}

// A rebound key must be visible in the ? overlay and must keep the key
// column aligned.
func TestHelpShowsEffectiveKeys(t *testing.T) {
	m := newTestModel(t, 80, 24, false)
	plain := m.helpLines()
	m.cfg = Config{Accent: defaultAccent, Keys: map[string]string{"help": "F1", "delete": "D"}}
	bound := m.helpLines()
	if len(bound) != len(plain) {
		t.Fatalf("line count changed: %d vs %d", len(bound), len(plain))
	}
	joined := strings.Join(bound, "\n")
	if !strings.Contains(joined, "F1") {
		t.Errorf("rebound help key missing from the key table:\n%s", joined)
	}
	if !strings.Contains(joined, "D") {
		t.Errorf("rebound delete key missing from the key table:\n%s", joined)
	}
	// Descriptions must start in the same column with and without
	// overrides, or the table reads as broken.
	for i, line := range bound {
		if i >= len(plain) {
			break
		}
		desc := strings.Index(line, strings.TrimSpace(descOf(plain[i])))
		origDesc := strings.Index(plain[i], strings.TrimSpace(descOf(plain[i])))
		if desc != origDesc {
			t.Errorf("line %d description moved from column %d to %d: %q",
				i+1, origDesc, desc, line)
		}
	}
}

// descOf is the description half of a help line: everything after the
// padded key column.
func descOf(line string) string {
	if len(line) <= 13 {
		return ""
	}
	return line[13:]
}

// A keybinding override must reach the key dispatcher.
func TestKeyOverrideDispatches(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newTestModel(t, 80, 24, false) // 20 skills: skill-01 .. skill-20
	m.cfg = Config{Accent: defaultAccent, Keys: map[string]string{"down": "n", "up": "p"}}
	if sel := m.selected(); sel == nil || sel.Name != "skill-01" {
		t.Fatalf("setup: %v", sel)
	}
	send(t, &m, keyPress('n')) // rebound move-down
	if sel := m.selected(); sel == nil || sel.Name != "skill-02" {
		t.Errorf("rebound down key did not move: %v", sel)
	}
	send(t, &m, keyPress('p')) // rebound move-up
	if sel := m.selected(); sel == nil || sel.Name != "skill-01" {
		t.Errorf("rebound up key did not move: %v", sel)
	}
	send(t, &m, keyPress('j')) // built-in still works
	if sel := m.selected(); sel == nil || sel.Name != "skill-02" {
		t.Errorf("built-in down key broke: %v", sel)
	}
	// A rebound quit key must quit, not fall through.
	m2 := newTestModel(t, 80, 24, false)
	m2.cfg = Config{Accent: defaultAccent, Keys: map[string]string{"quit": "Q"}}
	mm, cmd := m2.Update(keyPress('Q'))
	if mm.(Model).appMode != modeNormal {
		t.Error("rebound quit left the app in another mode")
	}
	if cmd == nil {
		t.Error("rebound quit did not produce a quit command")
	}
}

func TestParseAge(t *testing.T) {
	ok := map[string]time.Duration{
		"30d":   30 * 24 * time.Hour,
		"720h":  720 * time.Hour,
		"1h30m": 90 * time.Minute,
		"0d":    0,
	}
	for in, want := range ok {
		got, err := parseAge(in)
		if err != nil {
			t.Errorf("parseAge(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseAge(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "soon", "-5d", "5x"} {
		if _, err := parseAge(in); err == nil {
			t.Errorf("parseAge(%q) should fail", in)
		}
	}
}
