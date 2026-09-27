package main

import (
	"os"
	"strings"
	"testing"
)

// The README is the single source of truth for the keymap, the exit
// codes and the environment variables. These tests fail when the tables
// in keymap.go and the tables in README.md drift apart.

func readme(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("README.md unreadable: %v", err)
	}
	return string(raw)
}

func TestDocsMatchKeymap(t *testing.T) {
	doc := readme(t)
	for _, k := range keyTable {
		if !strings.Contains(doc, "`"+k.Keys+"`") {
			t.Errorf("README key table is missing %q", k.Keys)
		}
		if !strings.Contains(doc, k.Desc) {
			t.Errorf("README key table is missing the description %q", k.Desc)
		}
		if k.Action == "" {
			continue
		}
		if !strings.Contains(doc, "`"+k.Action+"`") {
			t.Errorf("README key table is missing the rebind name %q", k.Action)
		}
	}
}

func TestDocsMatchExitCodes(t *testing.T) {
	doc := readme(t)
	for _, e := range exitCodeDoc {
		row := "| " + itoaTest(e.Code) + " | " + e.Meaning + " |"
		if !strings.Contains(doc, row) {
			t.Errorf("README exit code table missing %q", row)
		}
	}
}

func TestDocsMatchEnvVars(t *testing.T) {
	doc := readme(t)
	for _, e := range envVarDoc {
		row := "| `" + e.Name + "` | " + e.Effect + " |"
		if !strings.Contains(doc, row) {
			t.Errorf("README env var table missing %q", row)
		}
	}
}

// ItoaTest is a tiny local helper so the row format above reads clearly.
func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The help overlay, --help and the README must all come from one place.
func TestHelpSourcesMatchKeymap(t *testing.T) {
	help := strings.Join(keyTableLines(Config{Accent: defaultAccent}), "\n")
	if got := strings.Join(NewModel(true, true, Config{Accent: defaultAccent}).helpLines(), "\n"); !strings.Contains(got, help) {
		t.Error("the ? overlay does not render the keymap table")
	}
}

// No document may point at a file that no longer exists.
func TestNoStaleDocReferences(t *testing.T) {
	readmeDoc := readme(t)
	agents, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("AGENTS.md unreadable: %v", err)
	}
	for name, body := range map[string]string{"README.md": readmeDoc, "AGENTS.md": string(agents)} {
		for _, stale := range []string{"PLAN.md", "tui-philosophy.md"} {
			for _, line := range strings.Split(body, "\n") {
				// Links into docs/ are fine; a reference to a deleted
				// top-level file is not.
				if strings.Contains(line, stale) && !strings.Contains(line, "docs/") {
					t.Errorf("%s still references %s: %s", name, stale, strings.TrimSpace(line))
				}
			}
		}
	}
	for _, f := range []string{"PLAN.md", "tui-philosophy.md"} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s still exists; the README is the single source of truth", f)
		}
	}
}

// Every markdown link in the README must resolve.
func TestReadmeLinksResolve(t *testing.T) {
	doc := readme(t)
	for _, target := range extractLinks(doc) {
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		path := strings.SplitN(target, "#", 2)[0]
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("README link %q does not resolve: %v", target, err)
		}
	}
}

func extractLinks(doc string) []string {
	var out []string
	for i := 0; i < len(doc); i++ {
		if doc[i] != '[' {
			continue
		}
		close := strings.IndexByte(doc[i:], ']')
		if close < 0 || i+close+1 >= len(doc) || doc[i+close+1] != '(' {
			continue
		}
		end := strings.IndexByte(doc[i+close+1:], ')')
		if end < 0 {
			continue
		}
		out = append(out, doc[i+close+2:i+close+1+end])
		i += close + 1 + end
	}
	return out
}
