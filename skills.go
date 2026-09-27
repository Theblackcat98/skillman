package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"gopkg.in/yaml.v3"
)

// Severity is how bad a problem is. It is data, not prose: the badge,
// the row colour, the validate summary and the exit code all read this
// one value, so none of them can disagree with the others (review A4).
//
// The old code stored issue *messages* and decided severity by asking
// whether a message started with "missing SKILL.md", so rewording that
// string silently changed exit codes.
type Severity int

const (
	SevOK Severity = iota
	SevWarn
	SevErr
)

// String is the badge text and the word in `validate` output.
func (s Severity) String() string {
	switch s {
	case SevErr:
		return "err"
	case SevWarn:
		return "warn"
	default:
		return "ok"
	}
}

// Issue codes. They are stable identifiers so a test or a script can key
// on a problem without matching English text.
const (
	CodeFileUnreadable = "file.unreadable"
	CodeFileMissing    = "file.missing"
	CodeFMMalformed    = "frontmatter.malformed"
	CodeNameMissing    = "name.missing"
	CodeDescMissing    = "description.missing"
	CodeNameMismatch   = "name.mismatch"
)

// Issue is one problem with one skill. Msg is human text and is never
// parsed back; Code is the stable part.
type Issue struct {
	Code string
	Msg  string
	Sev  Severity
}

// Skill is one directory under skills/ containing SKILL.md.
//
// Size and ModTime cost a walk of the whole skill directory on every
// scan. They used to be computed and never read, which halved the useful
// work of a rescan for nothing (review D1). They are now surfaced:
// Size and ModTime are columns in `list --long` and fields in
// `list --json`, and the trash lifecycle reports the same numbers.
type Skill struct {
	Name     string
	Dir      string
	Desc     string
	License  string
	Compat   string
	Category string
	Size     int64
	ModTime  time.Time
	Issues   []Issue

	// path is the SKILL.md this skill was scanned from. The markdown body
	// is deliberately not a field: a scan reads the frontmatter and stops,
	// and Body() reads the rest on demand. Holding every body meant the
	// list carried a copy of every skill's markdown whether or not the
	// user had looked at one (review D2).
	path string
}

type skillFM struct {
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	License       string         `yaml:"license"`
	Compatibility string         `yaml:"compatibility"`
	Metadata      map[string]any `yaml:"metadata"`
}

// ScanSkills reads dir/*/SKILL.md. Never fails hard on one bad skill.
func ScanSkills(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		skillPath := filepath.Join(dir, name)
		// The directory name is attacker-controlled and is not YAML, so
		// it needs the boundary treatment on its own.
		s := Skill{Name: safeName(name), Dir: skillPath}
		mdPath := filepath.Join(skillPath, "SKILL.md")
		s.path = mdPath
		fm, ferr := readFrontmatter(mdPath)
		// Three different failures, and the scan must not merge them: a
		// file that is not there, a file it cannot read, and a file whose
		// YAML will not parse. The first two existed; the third is
		// reported as its own code.
		var parseErr frontmatterError
		switch {
		case ferr == nil:
		case errors.Is(ferr, os.ErrNotExist):
			s.Desc = "(no SKILL.md)"
			s.Issues = []Issue{{
				Code: CodeFileMissing,
				Msg:  "missing SKILL.md",
				Sev:  SevErr,
			}}
			out = append(out, s)
			continue
		case errors.As(ferr, &parseErr):
			// A parse error is the whole story. The old code appended
			// "missing name" and "missing description" on top of it,
			// because the zero-valued frontmatter had empty fields, and
			// so reported three problems for one broken file (review A6).
			s.Issues = append(s.Issues, Issue{
				Code: CodeFMMalformed,
				Msg:  "invalid frontmatter: " + safeText(parseErr.Error()),
				Sev:  SevErr,
			})
			s.Desc = "(unreadable frontmatter)"
			out = append(out, s)
			continue
		default:
			s.Desc = "(unreadable SKILL.md)"
			s.Issues = []Issue{{
				Code: CodeFileUnreadable,
				Msg:  "unreadable SKILL.md: " + safeText(ferr.Error()),
				Sev:  SevErr,
			}}
			out = append(out, s)
			continue
		}
		if fm.Name == "" {
			s.Issues = append(s.Issues, Issue{Code: CodeNameMissing, Msg: "missing name", Sev: SevWarn})
		} else if fm.Name != name {
			s.Issues = append(s.Issues, Issue{
				Code: CodeNameMismatch,
				Msg:  fmt.Sprintf("frontmatter name %q != dirname %q", safeText(fm.Name), s.Name),
				Sev:  SevWarn,
			})
		}
		s.Desc = firstLine(safeBody(fm.Description))
		if s.Desc == "" {
			s.Desc = "(no description)"
			s.Issues = append(s.Issues, Issue{Code: CodeDescMissing, Msg: "missing description", Sev: SevWarn})
		}
		s.License = safeText(fm.License)
		s.Compat = safeText(fm.Compatibility)
		if fm.Metadata != nil {
			if c, ok := fm.Metadata["category"]; ok {
				s.Category = safeText(fmt.Sprint(c))
			}
		}
		var total int64
		// The walk is what produces Size and ModTime. It is a real
		// cost, so it is paid once here and the result is reported
		// rather than discarded.
		_ = filepath.Walk(skillPath, func(_ string, info os.FileInfo, _ error) error {
			if info != nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		s.Size = total
		if st, serr := os.Stat(mdPath); serr == nil {
			s.ModTime = st.ModTime()
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// splitFrontmatter separates the YAML head from the body. closed is false
// when the opening --- has no partner, which on a truncated prefix read
// means the frontmatter is longer than what was read.
func splitFrontmatter(raw string) (head, body string, closed, hasFM bool) {
	if !strings.HasPrefix(raw, "---") {
		return "", raw, false, false
	}
	lines := strings.Split(raw, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", raw, false, true
	}
	return strings.Join(lines[1:end], "\n"), strings.Join(lines[end+1:], "\n"), true, true
}

func parseFrontmatter(raw string) (skillFM, string, error) {
	var fm skillFM
	head, body, closed, hasFM := splitFrontmatter(raw)
	if !hasFM {
		return fm, raw, nil
	}
	if !closed {
		return fm, raw, errors.New("unterminated frontmatter (missing closing ---)")
	}
	if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
		return fm, body, err
	}
	return fm, body, nil
}

// frontmatterError marks a YAML failure as opposed to an I/O failure, so
// the scan can report a broken file differently from an unreadable one.
type frontmatterError struct{ err error }

func (e frontmatterError) Error() string { return e.err.Error() }
func (e frontmatterError) Unwrap() error { return e.err }

// frontmatterLimit bounds how much of a SKILL.md a scan reads. A
// frontmatter is a handful of lines; past this limit the bytes are body.
const frontmatterLimit = 32 << 10

// readFrontmatter reads and parses just the frontmatter of a SKILL.md.
//
// The scan used to read every byte of every file and keep the body in the
// model. That cost memory the list never needed — about 20 MB for 200
// skills of 100 KB, before the render cache holds its own copy — and it
// delayed the first frame behind every body in the directory.
func readFrontmatter(path string) (skillFM, error) {
	raw, truncated, err := readPrefix(path, frontmatterLimit)
	if err != nil {
		return skillFM{}, err
	}
	head, _, closed, hasFM := splitFrontmatter(raw)
	if hasFM && closed {
		var fm skillFM
		if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
			return fm, frontmatterError{err}
		}
		return fm, nil
	}
	if !truncated {
		// The whole file was read and it has no usable frontmatter.
		return skillFM{}, nil
	}
	// The frontmatter runs past the limit. That is pathological, so pay
	// for the full read and decide honestly rather than reporting a file
	// as broken when it is merely unusual.
	full, err := os.ReadFile(path)
	if err != nil {
		return skillFM{}, err
	}
	fm, _, err := parseFrontmatter(string(full))
	if err != nil {
		return fm, frontmatterError{err}
	}
	return fm, nil
}

// readPrefix reads at most limit bytes, and reports whether there was
// more to read.
func readPrefix(path string, limit int) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	// One byte past the limit, so a file that ends exactly at the limit
	// is not mistaken for a truncated one.
	buf := make([]byte, limit+1)
	n, rerr := io.ReadFull(f, buf)
	if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
		return "", false, rerr
	}
	if n > limit {
		return string(buf[:limit]), true, nil
	}
	return string(buf[:n]), false, nil
}

// Body returns the skill's markdown, read and sanitized on demand. It is
// the only way to get the body and a scan never calls it.
func (s Skill) Body() string {
	if s.path == "" {
		return ""
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return ""
	}
	_, body, perr := parseFrontmatter(string(raw))
	if perr != nil {
		// The scan already reported this as an issue, and the preview
		// falls back to the issue list.
		return ""
	}
	// The banner already ends in a blank line, and a real SKILL.md starts
	// with one after its frontmatter, so an untrimmed body put a double
	// blank line between the description and the first heading.
	return safeBody(strings.TrimLeft(body, "\n"))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// metaLine renders frontmatter metadata (category · license ·
// compatibility) for the preview header; empty fields are skipped.
func (s Skill) metaLine() string {
	parts := make([]string, 0, 3)
	if s.Category != "" {
		parts = append(parts, s.Category)
	}
	if s.License != "" {
		parts = append(parts, "license: "+s.License)
	}
	if s.Compat != "" {
		parts = append(parts, "compat: "+s.Compat)
	}
	return strings.Join(parts, " · ")
}

// FilterSkills matches query against name+desc+category (case-insensitive).
func FilterSkills(skills []Skill, query string) []Skill {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		// A copy, not the caller's slice. The model hands this straight
		// to its own field, so returning the original let a later change
		// to one write through to the full skill list (review F6).
		return slices.Clone(skills)
	}
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		hay := strings.ToLower(s.Name + " " + s.Desc + " " + s.Category)
		if strings.Contains(hay, q) {
			out = append(out, s)
		}
	}
	return out
}

// Severity is the worst issue on this skill, or SevOK.
func (s Skill) Severity() Severity {
	worst := SevOK
	for _, is := range s.Issues {
		if is.Sev > worst {
			worst = is.Sev
		}
	}
	return worst
}

// Badge is the short status text. It is derived from Severity, so the
// badge, the row colour, the validate summary and the exit code cannot
// disagree (review A4).
func (s Skill) Badge() string { return s.Severity().String() }

// Valid reports whether the skill has no issues. It reads Severity rather
// than carrying its own flag, so it cannot go stale.
func (s Skill) Valid() bool { return s.Severity() == SevOK }

// issueTexts is the issue messages on their own, for the places that
// render a list of them.
func (s Skill) issueTexts() []string {
	if len(s.Issues) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.Issues))
	for _, is := range s.Issues {
		out = append(out, is.Msg)
	}
	return out
}

// WorstSeverity is the worst severity across a list, and the only place
// the exit-code contract is decided. One pass over every issue, then one
// map: the old code returned from inside a loop on the first invalid
// skill, so the code depended on alphabetical order (review A3).
func WorstSeverity(skills []Skill) Severity {
	worst := SevOK
	for _, s := range skills {
		if s.Severity() > worst {
			worst = s.Severity()
		}
	}
	return worst
}

// ValidateExitCode maps the worst severity to the documented exit code:
// 0 clean, 1 warnings, 2 a skill has no SKILL.md.
func ValidateExitCode(skills []Skill) int {
	switch WorstSeverity(skills) {
	case SevErr:
		return 2
	case SevWarn:
		return 1
	default:
		return 0
	}
}

// RenderPreview returns glamour-rendered markdown, or raw fallback.
// One-shot helper for CLI use; the TUI uses renderPreviewWith + a cache.
func (s Skill) RenderPreview(width int, plain bool) string {
	if width < 20 {
		width = 20
	}
	if plain {
		return s.renderPreviewWith(nil, true)
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return s.renderPreviewWith(nil, true)
	}
	return s.renderPreviewWith(r, false)
}

// renderPreviewStyled is RenderPreview for the CLI when colour was asked
// for explicitly on a non-terminal. WithAutoStyle inspects stdout and
// picks "no style" when it is not a terminal, so SKILLMAN_COLOR=always
// would be silently ignored without an explicit style here.
func (s Skill) renderPreviewStyled(width int) string {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return s.renderPreviewWith(nil, true)
	}
	return s.renderPreviewWith(r, false)
}

// renderPreviewWith renders with a caller-supplied shared glamour renderer
// (fast path for the TUI). Falls back to raw text when plain or r is nil.
func (s Skill) renderPreviewWith(r *glamour.TermRenderer, plain bool) string {
	body := s.Body()
	if body == "" {
		if len(s.Issues) > 0 {
			return "Issues:\n- " + strings.Join(s.issueTexts(), "\n- ") + "\n"
		}
		return "(empty SKILL.md)"
	}
	if plain || r == nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return out
}

// summarize counts skills by severity for the one-line summary. It reads
// Severity, so it cannot drift from the badge or the exit code.
func summarize(skills []Skill) (ok, warn, bad int) {
	for _, s := range skills {
		switch s.Severity() {
		case SevErr:
			bad++
		case SevWarn:
			warn++
		default:
			ok++
		}
	}
	return ok, warn, bad
}
