package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"gopkg.in/yaml.v3"
)

// Skill is one directory under skills/ containing SKILL.md.

type Skill struct {
	Name     string
	Dir      string
	Desc     string
	License  string
	Compat   string
	Category string
	Size     int64
	ModTime  time.Time
	Valid    bool
	Issues   []string
	Body     string
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
		s := Skill{Name: name, Dir: skillPath}
		mdPath := filepath.Join(skillPath, "SKILL.md")
		raw, err := os.ReadFile(mdPath)
		if err != nil {
			s.Valid = false
			s.Issues = []string{"missing SKILL.md"}
			s.Desc = "(no SKILL.md)"
			if st, serr := os.Stat(skillPath); serr == nil {
				s.ModTime = st.ModTime()
			}
			out = append(out, s)
			continue
		}
		fm, body := parseFrontmatter(string(raw))
		s.Body = body
		if fm.Name != "" {
			// keep dir name as identity, but record mismatch
			if fm.Name != name {
				s.Issues = append(s.Issues, fmt.Sprintf("frontmatter name %q != dirname %q", fm.Name, name))
			}
		}
		s.Desc = firstLine(fm.Description)
		if s.Desc == "" {
			s.Desc = "(no description)"
			s.Issues = append(s.Issues, "missing description")
		}
		if fm.Name == "" {
			s.Issues = append(s.Issues, "missing name")
		}
		s.License = fm.License
		s.Compat = fm.Compatibility
		if fm.Metadata != nil {
			if c, ok := fm.Metadata["category"]; ok {
				s.Category = fmt.Sprint(c)
			}
		}
		var total int64
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
		s.Valid = len(s.Issues) == 0
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func parseFrontmatter(raw string) (skillFM, string) {
	var fm skillFM
	if !strings.HasPrefix(raw, "---") {
		return fm, raw
	}
	// Find closing --- on its own line.
	lines := strings.Split(raw, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return fm, raw
	}
	head := strings.Join(lines[1:end], "\n")
	_ = yaml.Unmarshal([]byte(head), &fm)
	return fm, strings.Join(lines[end+1:], "\n")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// FilterSkills matches query against name+desc+category (case-insensitive).
func FilterSkills(skills []Skill, query string) []Skill {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return skills
	}
	var out []Skill
	for _, s := range skills {
		hay := strings.ToLower(s.Name + " " + s.Desc + " " + s.Category)
		if strings.Contains(hay, q) {
			out = append(out, s)
		}
	}
	return out
}

// Badge returns short status text and level.
func (s Skill) Badge() (string, string) {
	if len(s.Issues) == 0 {
		return "ok", "ok"
	}
	for _, is := range s.Issues {
		if strings.HasPrefix(is, "missing SKILL.md") {
			return "err", "err"
		}
	}
	return "warn", "warn"
}

// RenderPreview returns glamour-rendered markdown, or raw fallback.
func (s Skill) RenderPreview(width int, plain bool) string {
	if width < 20 {
		width = 20
	}
	if s.Body == "" {
		if len(s.Issues) > 0 {
			return "Issues:\n- " + strings.Join(s.Issues, "\n- ") + "\n"
		}
		return "(empty SKILL.md)"
	}
	if plain {
		return s.Body
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return s.Body
	}
	out, err := r.Render(s.Body)
	if err != nil {
		return s.Body
	}
	return out
}

func summarize(skills []Skill) (ok, warn, bad int) {
	for _, s := range skills {
		b, _ := s.Badge()
		switch b {
		case "ok":
			ok++
		case "warn":
			warn++
		default:
			bad++
		}
	}
	return ok, warn, bad
}
