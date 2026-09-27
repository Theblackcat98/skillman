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

// The man page is generated from the command tables, and the checked-in
// file has to match. A hand-written page is a fifth copy of the command
// list and the copy nobody remembers to update (review D9, D10).
func TestManPageIsUpToDate(t *testing.T) {
	want := manPage()
	got, err := os.ReadFile(manPagePath)
	if err != nil {
		if os.IsNotExist(err) && *update {
			if err := os.WriteFile(manPagePath, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
		t.Fatalf("%s unreadable: %v\nrun: go test -run TestManPage -update", manPagePath, err)
	}
	if string(got) == want {
		return
	}
	if *update {
		if err := os.WriteFile(manPagePath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Errorf("%s is stale; run: go test -run TestManPage -update", manPagePath)
}

// The man page must carry the same facts as the README and --help, or it
// is worse than no page: a reader trusts it.
func TestManPageCoversTheTables(t *testing.T) {
	page := manPage()
	for _, c := range commands {
		if !strings.Contains(page, ".B "+c.name) {
			t.Errorf("man page is missing the command %q", c.name)
		}
		if c.usage != "" && !strings.Contains(page, c.usage) {
			t.Errorf("man page is missing the usage line %q for %q", c.usage, c.name)
		}
	}
	for _, k := range keyTable {
		if !strings.Contains(page, roffEscape(k.Keys)) {
			t.Errorf("man page is missing the key %q", k.Keys)
		}
		if !strings.Contains(page, roffEscape(k.Desc)) {
			t.Errorf("man page is missing the description %q", roffEscape(k.Desc))
		}
	}
	for _, e := range exitCodeDoc {
		if !strings.Contains(page, ".B "+itoaTest(e.Code)) {
			t.Errorf("man page is missing exit code %d", e.Code)
		}
		if !strings.Contains(page, roffEscape(e.Meaning)) {
			t.Errorf("man page is missing the meaning %q", roffEscape(e.Meaning))
		}
	}
	for _, e := range envVarDoc {
		if !strings.Contains(page, ".B "+e.Name) {
			t.Errorf("man page is missing the env var %s", e.Name)
		}
		if !strings.Contains(page, roffEscape(e.Effect)) {
			t.Errorf("man page is missing the effect %q", roffEscape(e.Effect))
		}
	}
	// A command that accepts a flag must document it, or a reader cannot
	// discover it. This is the check that would have caught a flag added
	// to the table and forgotten in every doc.
	for _, c := range commands {
		for _, f := range c.flags {
			if !strings.Contains(page, strings.ReplaceAll(f, "-", `\-`)) {
				t.Errorf("man page does not document the %s flag of %s", f, c.name)
			}
		}
	}
	// Every alias has to be discoverable somewhere.
	for _, c := range commands {
		for _, a := range c.aliases {
			if !strings.Contains(page, a) {
				t.Errorf("man page does not mention the alias %q for %s", a, c.name)
			}
		}
	}
}

// Every command in the table must be dispatchable, and every dispatch case
// must be in the table. The chain of || that used to decide "is this a
// known command" is gone; this is what replaces it as a guard.
func TestEveryCommandIsDispatchable(t *testing.T) {
	known := map[string]bool{"list": true, "view": true, "validate": true,
		"delete": true, "trash": true, "completion": true, "install": true, "help": true}
	for _, c := range commands {
		if !known[c.name] {
			t.Errorf("command %q is in the table but not in the dispatcher", c.name)
		}
		// And the flag gate has to agree with what the command declares.
		gate, ok := commandFlags[c.name]
		if !ok {
			t.Errorf("command %q has no entry in the flag gate", c.name)
			continue
		}
		for _, f := range c.flags {
			if !gate[f] {
				t.Errorf("%s declares %s but the flag gate rejects it", c.name, f)
			}
		}
		for f := range gate {
			declared := f == "-y" // the one hidden flag
			for _, d := range c.flags {
				if d == f {
					declared = true
				}
			}
			if !declared {
				t.Errorf("the flag gate accepts %s for %s but the command does not declare it", f, c.name)
			}
		}
		// Aliases must resolve back to the command.
		for _, a := range c.aliases {
			got, ok := canonicalCommand(a)
			if !ok || got.name != c.name {
				t.Errorf("alias %q does not resolve to %q", a, c.name)
			}
		}
	}
	if len(known) != len(commands) {
		t.Errorf("the dispatcher knows %d commands, the table has %d", len(known), len(commands))
	}
}

// A roff file that starts a line with a dot is a request, not text, so
// every string that reaches the page has to be escaped.
func TestRoffEscape(t *testing.T) {
	cases := map[string]string{
		"plain":      "plain",
		".hidden":    `\.hidden`,
		"'quote":     `\'quote`,
		`back\slash`: `back\eslash`,
		"mid.dot":    "mid.dot",
		"a - b":      `a \- b`,
		"--yes":      `\-\-yes`,
	}
	for in, want := range cases {
		if got := roffEscape(in); got != want {
			t.Errorf("roffEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

// roff reads a bare "--" as an en-dash and a bare "-" as a hyphen, so a
// flag name that reaches the page unescaped renders as the wrong
// character. Two instances of this shipped into the first draft of the
// page, so it is checked rather than trusted.
func TestManPageHasNoBareDashes(t *testing.T) {
	for i, line := range strings.Split(manPage(), "\n") {
		// Strip the escapes, then look for a dash that was not one.
		stripped := strings.ReplaceAll(line, `\-`, "")
		if strings.Contains(stripped, "--") {
			t.Errorf("line %d has an unescaped --: %q", i+1, line)
		}
	}
}

// A dot at the start of a line is a roff request. The page may only use
// the requests it means to, so any other leading dot is a string that
// escaped roffEscape and would now be interpreted rather than printed.
var allowedRequests = map[string]bool{
	".\\\"": true, // comment
	".TH":   true, ".SH": true, ".B": true, ".TP": true,
	".br": true, ".PP": true,
}

func TestManPageUsesOnlyIntendedRequests(t *testing.T) {
	for i, line := range strings.Split(manPage(), "\n") {
		if line == "" || line[0] != '.' {
			continue
		}
		req := strings.Fields(line)[0]
		if !allowedRequests[req] {
			t.Errorf("line %d uses the roff request %q, which is not one this page intends: %q",
				i+1, req, line)
		}
	}
}
