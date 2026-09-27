package main

// Install tests. Every one of them builds its own git repository in a
// temp directory and points SKILLMAN_SKILLS at a temp directory, so
// nothing here can touch the real skills directory: no network, no real
// clone, no real install. The same isolation is enforced for the whole
// suite by TestMain in hygiene_test.go.

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// collectMsgs flattens a batch into its messages, so a test can find the
// one it cares about among the spinner ticks.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, collectMsgs(sub)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// runCLI runs f with both streams captured and returns the output and the
// exit code. install reports its findings on stderr and its result on
// stdout, so a test that wants to read either needs both.
func runCLI(t *testing.T, f func() int) (string, int) {
	t.Helper()
	so, se := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	code := f()
	os.Stdout, os.Stderr = so, se
	_ = w.Close()
	all := <-done
	_ = r.Close()
	return all, code
}

// fixtureRepo makes a throwaway git repository with the given files and
// returns its path.
func fixtureRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "add", "-A"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		// A fixture must not pick up the user's git config, which on some
		// machines carries hooks or a template directory.
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

// repoURL is the file:// form of a fixture, which is the only source
// these tests use.
func repoURL(root string) string { return "file://" + root }

func skillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n\nBody of " + name + ".\n"
}

// --- URL validation ---

// ext:: is git's remote helper, which runs a shell command. A URL that
// reaches it is remote code execution, so it is refused by name before
// git ever sees it.
func TestInstallRefusesRemoteHelpers(t *testing.T) {
	for _, u := range []string{
		"ext::sh -c id",
		"EXT::sh -c id",
		"https://example.com/x --upload-pack=/bin/sh",
		"--upload-pack=/bin/sh",
		"-oProxyCommand=id",
		"ftp://example.com/x",
		"",
		"   ",
		"::/x",
	} {
		if _, err := validateGitURL(u); err == nil {
			t.Errorf("validateGitURL(%q) accepted a URL that must be refused", u)
		}
	}
}

func TestInstallAcceptsRealGitURLs(t *testing.T) {
	dir := t.TempDir()
	rel := t.TempDir()
	// A relative path is only accepted when it exists, so the error names
	// the path rather than surfacing a git failure later.
	for _, u := range []string{
		"https://github.com/owner/repo",
		"https://github.com/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"git://example.com/repo.git",
		"file:///tmp/repo",
		"git@github.com:owner/repo.git",
		dir,
		rel,
	} {
		if _, err := validateGitURL(u); err != nil {
			t.Errorf("validateGitURL(%q) = %v, want it accepted", u, err)
		}
	}
}

// A ref reaches git as an option value, so it must not be able to become
// an option.
func TestInstallRefValidation(t *testing.T) {
	if _, err := validateGitURL("./does-not-exist"); err == nil {
		t.Error("a relative path that does not exist was accepted")
	}
	for _, r := range []string{"main", "v1.2.3", "feature/x", "a1b2c3"} {
		if _, err := validateRef(r); err != nil {
			t.Errorf("validateRef(%q) = %v, want it accepted", r, err)
		}
	}
	for _, r := range []string{"--upload-pack=id", "-main", "a b", "a;id", "a$(id)", ""} {
		got, err := validateRef(r)
		if r == "" {
			if err != nil {
				t.Errorf("validateRef(\"\") = %v, want an empty ref to be fine", err)
			}
			if got != "" {
				t.Errorf("validateRef(\"\") = %q", got)
			}
			continue
		}
		if err == nil {
			t.Errorf("validateRef(%q) accepted a value that must be refused", r)
		}
	}
}

// --- candidate detection ---

func TestFindCandidatesCoversTheThreeLayouts(t *testing.T) {
	root := fixtureRepo(t, map[string]string{
		"SKILL.md":               skillMD("root-skill", "at the repository root"),
		"skills/alpha/SKILL.md":  skillMD("alpha", "under skills/"),
		"skills/nested/SKILL.md": skillMD("nested", "under skills/"),
		"extra/beta/SKILL.md":    skillMD("beta", "one level down"),
		"extra/deep/gamma/SKILL": "not a SKILL.md, must be ignored",
		"a/b/c/delta/SKILL.md":   skillMD("delta", "three levels down, too deep"),
		"README.md":              "not a skill",
		"docs/notes.md":          "not a skill",
	})
	src, err := cloneRepo(context.Background(), repoURL(root), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(src)

	cands, err := findCandidates(filepath.Join(src, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cands {
		got[c.Name] = c.Desc
		if !c.Selected {
			t.Errorf("%s is not selected by default", c.Name)
		}
	}
	for _, want := range []string{"alpha", "nested", "beta"} {
		if _, ok := got[want]; !ok {
			t.Errorf("did not find %q; found %v", want, got)
		}
	}
	// The walk is bounded at two levels, so a deep tree is skipped
	// rather than walked, and the root SKILL.md is a candidate too.
	if _, ok := got["delta"]; ok {
		t.Error("found a skill three levels down; the walk is bounded at two")
	}
}

// --- the whole CLI path, dry run only ---

// --dry-run has to exercise the clone, the detection and the report while
// writing nothing at all. That is what makes it safe to run against a
// repository the user is only considering.
func TestInstallDryRunWritesNothing(t *testing.T) {
	root := fixtureRepo(t, map[string]string{
		"skills/alpha/SKILL.md": skillMD("alpha", "the alpha skill"),
		"skills/beta/SKILL.md":  skillMD("beta", "the beta skill"),
	})
	skills, _ := isolatedEnv(t)
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, func() int { return runInstall(repoURL(root), "", true) })
	if code != 0 {
		t.Fatalf("runInstall --dry-run = %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "would install") {
		t.Errorf("dry run did not say what it would do:\n%s", out)
	}
	entries, err := os.ReadDir(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("a dry run wrote %v into the skills directory", names)
	}
}

// A real install into a temp skills directory, which is the only test that
// copies anything.
func TestInstallCopiesIntoTheSkillsDirectory(t *testing.T) {
	root := fixtureRepo(t, map[string]string{
		"skills/alpha/SKILL.md": skillMD("alpha", "the alpha skill"),
		"skills/beta/SKILL.md":  skillMD("beta", "the beta skill"),
		"skills/alpha/extra.md": "a supporting file",
	})
	skills, _ := isolatedEnv(t)
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, func() int { return runInstall(repoURL(root), "", false) })
	if code != 0 {
		t.Fatalf("runInstall = %d, want 0\n%s", code, out)
	}
	for _, name := range []string{"alpha", "beta"} {
		p := filepath.Join(skills, name, "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was not installed: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(skills, "alpha", "extra.md")); err != nil {
		t.Errorf("a supporting file was not copied: %v", err)
	}
	// And the installed skill scans clean.
	list, err := ScanSkills(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("scanned %d skills, want 2", len(list))
	}
	if got := ValidateExitCode(list); got != 0 {
		t.Errorf("the installed skills validate to %d, want 0", got)
	}
}

func TestInstallNeverClobbersAnExistingSkill(t *testing.T) {
	root := fixtureRepo(t, map[string]string{
		"skills/alpha/SKILL.md": skillMD("alpha", "from the repository"),
	})
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "alpha", skillMD("alpha", "already installed"))

	_, code := runCLI(t, func() int { return runInstall(repoURL(root), "", false) })
	if code != 1 {
		t.Errorf("installing over an existing skill = %d, want 1", code)
	}
	got, err := ScanSkills(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Desc != "already installed" {
		t.Errorf("the existing skill was replaced: %+v", got)
	}
}

func TestInstallReportsAnUnusableURL(t *testing.T) {
	for _, u := range []string{"ext::sh -c id", "", "ftp://x/y"} {
		_, code := runCLI(t, func() int { return runInstall(u, "", false) })
		if code != 2 {
			t.Errorf("runInstall(%q) = %d, want 2", u, code)
		}
	}
}

func TestInstallReportsARepositoryWithNoSkills(t *testing.T) {
	root := fixtureRepo(t, map[string]string{"README.md": "no skills here"})
	_, code := runCLI(t, func() int { return runInstall(repoURL(root), "", false) })
	if code != 1 {
		t.Errorf("install from a repository with no SKILL.md = %d, want 1", code)
	}
}

// The clone must not survive the command, whether it worked or not.
func TestInstallRemovesItsTempClone(t *testing.T) {
	root := fixtureRepo(t, map[string]string{
		"skills/alpha/SKILL.md": skillMD("alpha", "the alpha skill"),
	})
	before := countTempDirs(t, "skillman-clone-")
	runCLI(t, func() int { return runInstall(repoURL(root), "", true) })
	if after := countTempDirs(t, "skillman-clone-"); after != before {
		t.Errorf("temp clones went from %d to %d; a clone was left behind", before, after)
	}
	// And after a failed install too.
	runCLI(t, func() int { return runInstall("file:///nonexistent/repo", "", false) })
	if after := countTempDirs(t, "skillman-clone-"); after != before {
		t.Errorf("a failed install left a clone behind: %d -> %d", before, after)
	}
}

func countTempDirs(t *testing.T, prefix string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), prefix+"*"))
	if err != nil {
		t.Fatal(err)
	}
	return len(matches)
}

// --- copying is contained ---

// A repository can contain a symlink pointing anywhere. Following it would
// let a clone read, and then write, somewhere the user never named.
func TestCopySkillRefusesToFollowASymlink(t *testing.T) {
	skills, _ := isolatedEnv(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("do not copy me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte(skillMD("x", "d")), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file symlink and a directory symlink, both pointing outside.
	if err := os.Symlink(secret, filepath.Join(src, "leak.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "escape")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(skills, "x")
	if err := copySkill(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "leak.txt")); err == nil {
		t.Error("a symlinked file was copied as a file")
	}
	if _, err := os.Lstat(filepath.Join(dst, "escape")); err == nil {
		t.Error("a symlinked directory was copied")
	}
	// The real content did come across.
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md was not copied: %v", err)
	}
}

func TestCopySkillRefusesToWriteOutsideTheSkillsDirectory(t *testing.T) {
	skills, _ := isolatedEnv(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte(skillMD("x", "d")), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "escaped")
	if err := copySkill(src, outside); err == nil {
		t.Error("copySkill wrote outside the skills directory")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("the outside destination was created")
	}
	// The skills directory itself is not a valid destination either.
	if err := copySkill(src, skills); err == nil {
		t.Error("copySkill would have written over the skills directory")
	}
}

func TestWithinDoesNotAllowEscaping(t *testing.T) {
	root := "/a/b"
	for _, c := range []struct {
		path string
		want bool
	}{
		{"/a/b/c", true},
		{"/a/b", true},
		{"/a/bc", false},
		{"/a", false},
		{"/a/b/../c", false},      // cleans to /a/c
		{"/a/b/c/../d", true},     // cleans to /a/b/d
		{"/a/b/c/../../c", false}, // cleans to /a/c
	} {
		if got := within(root, filepath.Clean(c.path)); got != c.want {
			t.Errorf("within(%q, %q) = %v, want %v", root, c.path, got, c.want)
		}
	}
}

// --- the TUI flow ---

// The `i` key must not let a list key through. Delete, undo and the
// navigation keys all belong to the base layer.
func TestInstallLayersOwnTheirKeys(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.skills = fixtureSkills(t, 3)
	m.applyFilter()
	before := m.selected().Name

	mm, _ := m.Update(keyPress('i'))
	m = mm.(Model)
	if m.appMode != modeInstallInput {
		t.Fatalf("i did not open the install prompt, mode = %v", m.appMode)
	}
	for _, k := range []rune{'d', 'u', 'j', 'k', 'v', 'r', 'q'} {
		mm, _ = m.Update(keyPress(k))
		m = mm.(Model)
	}
	if m.appMode != modeInstallInput {
		t.Errorf("a base-layer key escaped from the install prompt, mode = %v", m.appMode)
	}
	if got := m.selected().Name; got != before {
		t.Errorf("the selection moved from %q to %q while in the install prompt", before, got)
	}
	if m.undo.Active {
		t.Error("d reached the delete handler from the install prompt")
	}
	// Esc leaves, and the base layer still works afterwards.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.appMode != modeNormal {
		t.Errorf("esc did not leave the install prompt, mode = %v", m.appMode)
	}
	mm, _ = m.Update(keyPress('j'))
	m = mm.(Model)
	if m.selected().Name == before {
		t.Error("j did not move the selection after leaving the install prompt")
	}
}

// The checklist toggles, and enter installs only what is ticked.
func TestInstallChecklistToggles(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	m.installCands = []installCandidate{
		{Name: "alpha", Selected: true, Valid: true},
		{Name: "beta", Selected: true, Valid: true},
		{Name: "gamma", Selected: true, Valid: false},
	}
	m.appMode = modeInstallPick
	if got := tickedCount(m.installCands); got != 3 {
		t.Fatalf("setup: %d ticked", got)
	}
	// Space unticks the row under the cursor.
	mm, _ := m.Update(keyPress(' '))
	m = mm.(Model)
	if got := tickedCount(m.installCands); got != 2 {
		t.Errorf("space unticked, %d left", got)
	}
	// a is a toggle, so with one unticked it ticks the rest.
	mm, _ = m.Update(keyPress('a'))
	m = mm.(Model)
	if got := tickedCount(m.installCands); got != 3 {
		t.Errorf("a did not tick the rest, %d ticked", got)
	}
	// And with everything ticked it clears the list.
	mm, _ = m.Update(keyPress('a'))
	m = mm.(Model)
	if got := tickedCount(m.installCands); got != 0 {
		t.Errorf("a did not clear the list, %d left", got)
	}
	mm, _ = m.Update(keyPress('a'))
	m = mm.(Model)
	if got := tickedCount(m.installCands); got != 3 {
		t.Errorf("a did not tick everything, %d ticked", got)
	}
	// An invalid skill is visible as such, not hidden.
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "gamma") || !strings.Contains(frame, "invalid") {
		t.Errorf("the checklist hides the invalid skill:\n%s", frame)
	}
	// Enter with nothing ticked must not report a successful install.
	m.installCands[0].Selected = false
	m.installCands[1].Selected = false
	m.installCands[2].Selected = false
	cmd := m.chooseInstall()
	if cmd == nil {
		t.Fatal("chooseInstall returned nothing")
	}
	var done *installDoneMsg
	for _, sub := range collectMsgs(cmd) {
		if d, ok := sub.(installDoneMsg); ok {
			done = &d
		}
	}
	if done == nil {
		t.Fatal("chooseInstall produced no installDoneMsg")
	}
	if len(done.res.Installed) != 0 {
		t.Errorf("installed %v with nothing ticked", done.res.Installed)
	}
}

// Cancelling must remove the clone. A temp directory left behind by an
// abandoned install is litter nobody asked for.
func TestInstallCancelRemovesTheClone(t *testing.T) {
	m := newTestModel(t, 80, 24, true)
	dir := t.TempDir()
	m.installSrc = dir
	m.installCands = []installCandidate{{Name: "alpha"}}
	m.appMode = modeInstallPick
	if err := os.MkdirAll(filepath.Join(dir, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}

	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(Model)
	if m.appMode != modeNormal {
		t.Errorf("esc did not leave the checklist")
	}
	if cmd == nil {
		t.Fatal("cancelling did not schedule a cleanup")
	}
	cmd()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the clone is still on disk at %s", dir)
	}
	if m.installSrc != "" {
		t.Error("the model still holds the clone path")
	}
}

// The completion scripts each carried the command list as a literal, so
// adding install silently left them offering a command that did not exist
// and not offering one that did. They are substituted from the table now.
func TestCompletionScriptsOfferEveryCommand(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"bash", withCommands(bashCompletion)},
		{"zsh", withCommands(zshCompletion)},
	} {
		for _, c := range commands {
			if c.name == "help" {
				continue // offered as --help
			}
			if !strings.Contains(tc.script, c.name) {
				t.Errorf("the %s completion does not offer %q", tc.name, c.name)
			}
		}
		// The placeholder must not survive into the output.
		if strings.Contains(tc.script, "$SKILLMAN_COMMANDS") {
			t.Errorf("the %s completion still has an unsubstituted placeholder", tc.name)
		}
	}
}
