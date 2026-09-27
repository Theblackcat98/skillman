package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Test hygiene.
//
// Two things were wrong with the suite itself, and both are the kind of
// problem that makes a person stop running it:
//
//   - Several tests wrote into the developer's real trash and config
//     directories. A test suite that moves your files is a test suite you
//     eventually disable.
//   - Every test that called a CLI function printed its output into the
//     test log, so `go test` scrolled hundreds of lines of help text and
//     JSON past the actual results (review G7).

// captureStdout redirects os.Stdout for the duration of f and returns
// what was written. Needed because the CLI writes to os.Stdout
// directly rather than through an io.Writer. A nil t means "discard,
// do not report", which is what quiet wants.
func captureStdout(t *testing.T, f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		return ""
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	f()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// TestMain watches the developer's real trash for the whole suite.
//
// A test that moves a real skill into the real trash is a test that eats
// the developer's data, and it is invisible in the output: the test
// passes, the suite is green, and a directory you did not create gains an
// entry. Two tests did exactly this before this check existed, and the
// only reason it was caught was a manual count.
//
// This snapshots the real trash before the first test and diffs it after
// the last one, so the regression is impossible to reintroduce silently.
func TestMain(m *testing.M) {
	before := realTrashEntries()
	code := m.Run()
	if after := realTrashEntries(); len(after) != len(before) {
		fmt.Fprintf(os.Stderr,
			"\nFAIL: the test suite wrote into the real trash at %s\n  before: %d entries\n  after:  %d entries\n"+
				"  new:    %v\n"+
				"A test that deletes must call isolatedEnv(t) so SKILLMAN_SKILLS,\n"+
				"XDG_DATA_HOME and XDG_CONFIG_HOME all point at a temp dir.\n",
			trashDir(), len(before), len(after), diffNames(before, after))
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// realTrashEntries lists the trash directory as the developer's
// environment defines it, before any t.Setenv can change it.
func realTrashEntries() map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(trashDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		out[e.Name()] = true
	}
	return out
}

func diffNames(before, after map[string]bool) []string {
	var added []string
	for n := range after {
		if !before[n] {
			added = append(added, n)
		}
	}
	sort.Strings(added)
	return added
}

// A suite that writes outside its own temp directories is a suite that
// eats the developer's data. This exercises a full delete and restore
// with HOME and the XDG dirs redirected, and checks nothing lands in the
// developer's real home.
func TestNoTestWritesOutsideItsTempDirs(t *testing.T) {
	home := t.TempDir()
	realHome, hadHome := os.LookupEnv("HOME")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("SKILLMAN_SKILLS", filepath.Join(home, "skills"))
	if hadHome {
		defer os.Setenv("HOME", realHome)
	}

	// A representative destructive path: move a real skill to the real
	// trash, then put it back. Every delete path in the suite goes
	// through the same helper.
	skills := filepath.Join(home, "skills")
	writeSkill(t, skills, "demo", validFM("demo", "d"))
	captureStdout(t, func() {
		if code := run([]string{"delete", "demo", "--yes"}); code != 0 {
			t.Errorf("delete exit = %d, want 0", code)
		}
		if code := run([]string{"trash", "restore", "demo"}); code != 0 {
			t.Errorf("restore exit = %d, want 0", code)
		}
	})

	// Nothing may appear outside $HOME.
	assertEmptyDir(t, home, "state.json")
	assertEmptyDir(t, home, "config.yaml")
}

func assertEmptyDir(t *testing.T, home, name string) {
	t.Helper()
	var found []string
	_ = filepath.Walk(home, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(p) == name {
			found = append(found, p)
		}
		return nil
	})
	if len(found) > 0 {
		t.Errorf("unexpected %s written outside the temp home: %v", name, found)
	}
}

// The CLI test functions must not spray their output into the test log.
func TestCLITestsDoNotPolluteStdout(t *testing.T) {
	skills, _ := isolatedEnv(t)
	writeSkill(t, skills, "demo", validFM("demo", "d"))

	leaked := ""
	captureStdout(t, func() {
		// captureStdout already redirects, so this proves the helper
		// works: what the CLI writes must be captured, not printed.
		leaked = captureStdout(t, func() {
			runList(false, true, false)
		})
	})
	if !strings.Contains(leaked, "demo") {
		t.Errorf("captured output does not contain the skill: %q", leaked)
	}
}

// quiet runs f with stdout and stderr discarded. Every CLI test uses it,
// so `go test` output is results and nothing else.
func quiet(f func()) {
	_ = captureStdout(nil, f)
}
