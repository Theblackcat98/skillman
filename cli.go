package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Non-interactive CLI: list, view, validate, delete.

func runList(jsonOut, plain bool, names bool) int {
	skills, err := ScanSkills(skillsDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error: "+err.Error())
		return 1
	}
	if names {
		// One name per line: what shell completion needs, with no
		// dependency on jq or a JSON parser.
		for _, s := range skills {
			fmt.Println(s.Name)
		}
		return 0
	}
	if jsonOut {
		type row struct {
			Name     string   `json:"name"`
			Desc     string   `json:"description"`
			Category string   `json:"category,omitempty"`
			Valid    bool     `json:"valid"`
			Issues   []string `json:"issues,omitempty"`
			Dir      string   `json:"dir"`
		}
		rows := make([]row, 0, len(skills))
		for _, s := range skills {
			rows = append(rows, row{s.Name, s.Desc, s.Category, s.Valid, s.Issues, s.Dir})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return encodeJSON(enc, rows)
	}
	if len(skills) == 0 {
		fmt.Println("No skills in " + skillsDir())
		return 0
	}
	for _, s := range skills {
		b, _ := s.Badge()
		// Fixed-width name and badge, truncated description: a 400-char
		// description used to wrap and break every line-oriented reader.
		fmt.Printf("%-28s [%s] %s\n", truncRunes(s.Name, 28), b, truncate(s.Desc, 68))
	}
	return 0
}

// encodeJSON writes v and reports a failed write. A truncated stream that
// still exits 0 is worse than an error.
func encodeJSON(enc *json.Encoder, v any) int {
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "write error: "+err.Error())
		return 1
	}
	return 0
}

func runView(name string, plain bool) int {
	skills, err := ScanSkills(skillsDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error: "+err.Error())
		return 1
	}
	for _, s := range skills {
		if s.Name == name {
			fmt.Printf("# %s\n\n%s\n\n", s.Name, s.Desc)
			if len(s.Issues) > 0 {
				fmt.Println("Issues:")
				for _, is := range s.Issues {
					fmt.Println("- " + is)
				}
				fmt.Println()
			}
			body := s.Body
			if !plain {
				body = s.RenderPreview(80, false)
			}
			fmt.Println(strings.TrimRight(body, "\n"))
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "skill not found: "+name)
	return 1
}

func runValidate(jsonOut bool) int {
	skills, err := ScanSkills(skillsDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error: "+err.Error())
		return 1
	}
	code := 0
	if jsonOut {
		type row struct {
			Name   string   `json:"name"`
			Valid  bool     `json:"valid"`
			Issues []string `json:"issues,omitempty"`
		}
		rows := make([]row, 0, len(skills))
		for _, s := range skills {
			rows = append(rows, row{s.Name, s.Valid, s.Issues})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		code = encodeJSON(enc, rows)
	} else {
		ok, warn, bad := summarize(skills)
		fmt.Printf("%d ok, %d warn, %d err (%d total)\n", ok, warn, bad, len(skills))
		for _, s := range skills {
			if !s.Valid {
				fmt.Printf("- %s: %s\n", s.Name, strings.Join(s.Issues, "; "))
			}
		}
	}
	// Map the worst issue over all skills to one exit code. Returning
	// from inside the loop made the code depend on alphabetical order
	// (audit A3): a warn sorting before an error reported the wrong one.
	worst := 0
	for _, s := range skills {
		for _, is := range s.Issues {
			if strings.HasPrefix(is, "missing SKILL.md") {
				worst = 2
			} else if worst < 1 {
				worst = 1
			}
		}
	}
	if worst > 0 {
		return worst
	}
	return code
}

func runDeleteCLI(name string, assumeYes bool) int {
	skills, err := ScanSkills(skillsDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error: "+err.Error())
		return 1
	}
	for _, s := range skills {
		if s.Name == name {
			if !assumeYes {
				fmt.Fprintf(os.Stderr, "refusing to delete %q without --yes\n", name)
				return 3
			}
			if _, err := deleteSkillToTrash(s); err != nil {
				fmt.Fprintln(os.Stderr, "delete error: "+err.Error())
				return 1
			}
			fmt.Printf("moved %s to the trash — restore with: skillman trash restore %s\n", name, name)
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "skill not found: "+name)
	return 1
}

// runTrash implements `skillman trash list|restore|purge`. The trash is
// the only way back from a delete, so it needs a real interface: undo
// works for 30s inside one TUI session and nowhere else.
func runTrash(args []string, jsonOut, all bool, age string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "trash needs list, restore <name> or purge")
		return 2
	}
	switch args[0] {
	case "list", "ls":
		entries, err := listTrash()
		if err != nil {
			fmt.Fprintln(os.Stderr, "trash error: "+err.Error())
			return 1
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if entries == nil {
				entries = []TrashEntry{}
			}
			return encodeJSON(enc, entries)
		}
		if len(entries) == 0 {
			fmt.Println("Trash is empty (" + trashDir() + ")")
			return 0
		}
		for _, e := range entries {
			fmt.Printf("%-28s %s  %s\n", truncRunes(e.Name, 28),
				e.Deleted.Format("2006-01-02 15:04"), e.Slug)
		}
		return 0

	case "restore":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "trash restore needs a skill name")
			return 2
		}
		name, err := restoreFromTrash(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "restore error: "+err.Error())
			return 1
		}
		fmt.Printf("restored %s to %s\n", name, filepath.Join(skillsDir(), name))
		return 0

	case "purge":
		maxAge := time.Duration(0)
		switch {
		case age != "":
			d, err := parseAge(age)
			if err != nil {
				fmt.Fprintln(os.Stderr, "trash purge: "+err.Error()+" (use 30d or 720h)")
				return 2
			}
			maxAge = d
		case !all:
			fmt.Fprintln(os.Stderr, "trash purge needs --older-than 30d, or --all")
			return 2
		}
		purged, err := purgeTrash(maxAge)
		if err != nil {
			fmt.Fprintln(os.Stderr, "purge error: "+err.Error())
			return 1
		}
		if len(purged) == 0 {
			fmt.Println("Nothing purged.")
			return 0
		}
		for _, slug := range purged {
			fmt.Println("purged " + slug)
		}
		return 0

	default:
		fmt.Fprintln(os.Stderr, "unknown trash command: "+args[0])
		return 2
	}
}

const bashCompletion = `# bash completion for skillman
_skillman() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  local flags="--plain --no-animations --json --yes --names --all --older-than --skills-dir --help"
  case "$prev" in
    --skills-dir|--older-than)
      COMPREPLY=(); return 0 ;;
    view|delete)
      COMPREPLY=( $(compgen -W "$(skillman list --names 2>/dev/null)" -- "$cur") ); return 0 ;;
    completion)
      COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") ); return 0 ;;
    trash)
      case "$cur" in
        -*) COMPREPLY=( $(compgen -W "$flags" -- "$cur") ) ;;
        *)  COMPREPLY=( $(compgen -W "list restore purge" -- "$cur") ) ;;
      esac
      return 0 ;;
  esac
  if [[ "$cur" == -* ]]; then
    COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
  else
    COMPREPLY=( $(compgen -W "list view validate delete trash completion" -- "$cur") )
  fi
}
complete -F _skillman skillman
`

const zshCompletion = `#compdef skillman
# zsh completion for skillman
_skillman() {
  local -a skills
  local curcontext="$curcontext" state line
  _arguments -C \
    '(-h --help)'{-h,--help}'[show help]' \
    '--plain[no colors or styling]' \
    '--no-animations[disable spinner and transitions]' \
    '--json[machine-readable output]' \
    '--yes[assume yes for destructive actions]' \
    '--names[print one skill name per line]' \
    '--skills-dir=[override the skills directory]:dir:_files -/' \
    '1:command:(list view validate delete trash completion)' \
    '*:file:_files'
  case "$words[2]" in
    view|delete)
      skills=(${(f)"$(skillman list --names 2>/dev/null)"})
      compadd -- $skills ;;
    completion)
      compadd bash zsh ;;
    trash)
      [[ "$words[3]" == "-"* ]] || compadd list restore purge ;;
  esac
}
compdef _skillman skillman
`

func runCompletion(shell string) int {
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	default:
		fmt.Fprintln(os.Stderr, "completion needs bash or zsh")
		return 2
	}
	return 0
}

// printHelp is generated from the same keymap and exit code table that
// the README documents, so the two cannot drift.
func printHelp(cfg Config) {
	help := []string{
		"skillman — manage opencode skills",
		"",
		"Usage:",
		"  skillman                        launch TUI (TTY)",
		"  skillman list [--json|--names]   list skills",
		"  skillman view <name> [--plain]  show skill preview",
		"  skillman validate [--json]      validate frontmatter",
		"  skillman delete <name> --yes    move skill to trash",
		"  skillman trash list [--json]    list trashed skills",
		"  skillman trash restore <name>   put a skill back",
		"  skillman trash purge --older-than 30d | --all",
		"  skillman completion bash|zsh    print a shell completion script",
		"",
		"Flags:",
		"  --plain            no colors/markdown styling (also NO_COLOR, TERM=dumb)",
		"  --no-animations    disable spinner/transitions (also NO_ANIMATIONS, REDUCED_MOTION, CI)",
		"  --skills-dir DIR   override skills directory (also SKILLMAN_SKILLS, config.yaml)",
		"  --older-than AGE   trash purge age: 30d or 720h",
		"  --all              trash purge: remove every entry",
		"  -h, --help         show this help",
		"",
		"Exit codes:",
	}
	help = append(help, exitCodeLines()...)
	help = append(help,
		"",
		"TUI keys:",
	)
	for _, line := range keyTableLines(cfg) {
		help = append(help, "  "+line)
	}
	help = append(help,
		"",
		"Environment variables:",
	)
	help = append(help, envVarLines()...)
	help = append(help,
		"",
		"",
		"Config: "+configPath(),
		"  skills_dir: where skills live",
		"  accent:     UI accent colour, e.g. \"#34D399\"",
		"  keys:       rebind an action, e.g. keys: {delete: X}",
		"              known actions: "+overridableNames(),
		"",
		"State: "+statePath()+" (last selection, preview scroll, pane focus)",
		"Trash: "+trashDir(),
		"",
		"Full documentation: README.md",
	)
	fmt.Println(strings.Join(help, "\n"))
}
