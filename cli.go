package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Non-interactive CLI: list, view, validate, delete.

// loadSkills is the one place a CLI command reads the skills directory,
// so every command reports a scan error the same way and the precedence
// rules live in one function (review F8).
func loadSkills() ([]Skill, int) {
	skills, err := ScanSkills(skillsDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error: "+err.Error())
		return nil, 1
	}
	return skills, 0
}

func runList(jsonOut, plain, names, long bool) int {
	skills, code := loadSkills()
	if code != 0 {
		return code
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
			License  string   `json:"license,omitempty"`
			Compat   string   `json:"compatibility,omitempty"`
			Valid    bool     `json:"valid"`
			Severity string   `json:"severity"`
			Issues   []string `json:"issues,omitempty"`
			Codes    []string `json:"issue_codes,omitempty"`
			Size     int64    `json:"size_bytes"`
			Modified string   `json:"modified,omitempty"`
			Dir      string   `json:"dir"`
		}
		rows := make([]row, 0, len(skills))
		for _, s := range skills {
			r := row{
				Name: s.Name, Desc: s.Desc, Category: s.Category,
				License: s.License, Compat: s.Compat,
				Valid: s.Valid(), Issues: s.issueTexts(), Size: s.Size, Dir: s.Dir,
				Severity: s.Severity().String(),
			}
			if !s.ModTime.IsZero() {
				r.Modified = s.ModTime.Format(time.RFC3339)
			}
			rows = append(rows, r)
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
		b := s.Badge()
		// Fixed-width name and badge, truncated description: a 400-char
		// description used to wrap and break every line-oriented reader
		// (review E8). --long is the escape hatch.
		fmt.Printf("%-28s [%s] %s\n", truncRunes(s.Name, 28), b, truncate(s.Desc, 68))
		if !long {
			continue
		}
		// The scan walks every skill directory to compute Size and
		// ModTime. Those numbers used to be discarded; --long is where
		// they are worth something (review D1).
		var extra []string
		if s.Category != "" {
			extra = append(extra, "category="+s.Category)
		}
		if s.License != "" {
			extra = append(extra, "license="+s.License)
		}
		if s.Compat != "" {
			extra = append(extra, "compat="+s.Compat)
		}
		extra = append(extra, fmt.Sprintf("size=%s", humanSize(s.Size)))
		if !s.ModTime.IsZero() {
			extra = append(extra, "modified="+s.ModTime.Format("2006-01-02 15:04"))
		}
		if len(s.Issues) > 0 {
			extra = append(extra, "issues="+truncRunes(strings.Join(s.issueTexts(), "; "), 60))
		}
		fmt.Printf("%-28s         %s\n", "", strings.Join(extra, "  "))
	}
	return 0
}

// humanSize renders a byte count the way a person reads one.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
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
	skills, code := loadSkills()
	if code != 0 {
		return code
	}
	for _, s := range skills {
		if s.Name == name {
			fmt.Printf("# %s\n\n%s\n\n", s.Name, s.Desc)
			if len(s.Issues) > 0 {
				fmt.Println("Issues:")
				for _, is := range s.issueTexts() {
					fmt.Println("- " + is)
				}
				fmt.Println()
			}
			body := s.Body()
			if !plain {
				// Glamour picks "no style" when stdout is not a terminal,
				// so an explicit SKILLMAN_COLOR=always has to name a
				// style or the escape hatch does nothing.
				if colorForced() {
					body = s.renderPreviewStyled(80)
				} else {
					body = s.RenderPreview(80, false)
				}
			}
			fmt.Println(strings.TrimRight(body, "\n"))
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "skill not found: "+name)
	return 1
}

func runValidate(jsonOut bool) int {
	skills, exit := loadSkills()
	if exit != 0 {
		return exit
	}
	code := 0
	if jsonOut {
		type row struct {
			Name     string   `json:"name"`
			Valid    bool     `json:"valid"`
			Severity string   `json:"severity"`
			Issues   []string `json:"issues,omitempty"`
			Codes    []string `json:"issue_codes,omitempty"`
		}
		rows := make([]row, 0, len(skills))
		for _, s := range skills {
			codes := make([]string, 0, len(s.Issues))
			for _, is := range s.Issues {
				codes = append(codes, is.Code)
			}
			rows = append(rows, row{s.Name, s.Valid(), s.Severity().String(), s.issueTexts(), codes})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		code = encodeJSON(enc, rows)
	} else {
		ok, warn, bad := summarize(skills)
		fmt.Printf("%d ok, %d warn, %d err (%d total)\n", ok, warn, bad, len(skills))
		for _, s := range skills {
			if !s.Valid() {
				fmt.Printf("- %s [%s]: %s\n", s.Name, s.Severity(), strings.Join(s.issueTexts(), "; "))
			}
		}
	}
	// One owner for the exit-code contract: the worst severity over all
	// skills, mapped once. Returning from inside the loop made the code
	// depend on alphabetical order (review A3).
	if code != 0 {
		return code
	}
	return ValidateExitCode(skills)
}

func runDeleteCLI(name string, assumeYes bool) int {
	skills, code := loadSkills()
	if code != 0 {
		return code
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

// completionCommands is the command list the completion scripts offer.
// It was a literal string in each script — a fourth copy of the command
// list, which install made stale the moment it was added. The scripts
// carry a placeholder and the word list is substituted at print time, so
// the scripts and the table cannot disagree.
var completionCommands = func() string {
	names := make([]string, 0, len(commands))
	for _, c := range commands {
		if c.name == "help" {
			continue // the scripts offer --help themselves
		}
		names = append(names, c.name)
	}
	return strings.Join(names, " ")
}()

func withCommands(script string) string {
	return strings.ReplaceAll(script, "$SKILLMAN_COMMANDS", completionCommands)
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
    COMPREPLY=( $(compgen -W "$SKILLMAN_COMMANDS" -- "$cur") )
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
    '1:command:($SKILLMAN_COMMANDS)' \
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

// runInstall clones a repository, reports the skills it found, and copies
// the chosen ones. --dry-run stops after the report, so the whole path can
// be exercised without writing anything.
func runInstall(url, ref string, dryRun bool) int {
	cleanURL, err := validateGitURL(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "install: "+err.Error())
		return 2
	}
	cleanRef, err := validateRef(ref)
	if err != nil {
		fmt.Fprintln(os.Stderr, "install: "+err.Error())
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()

	dir, err := cloneRepo(ctx, cleanURL, cleanRef)
	if err != nil {
		fmt.Fprintln(os.Stderr, "install: "+err.Error())
		if errors.Is(err, context.DeadlineExceeded) {
			return 1
		}
		return 1
	}
	// The clone is a temporary artefact whatever happens next.
	defer os.RemoveAll(dir)

	cands, err := findCandidates(filepath.Join(dir, "repo"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "install: "+err.Error())
		return 1
	}
	if len(cands) == 0 {
		fmt.Fprintln(os.Stderr, "install: no SKILL.md found in "+cleanURL)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%d skill(s) in %s:\n", len(cands), cleanURL)
	for _, c := range cands {
		state := "ok"
		if !c.Valid {
			state = "invalid"
		}
		fmt.Fprintf(os.Stderr, "  %-24s %-8s %s\n", c.Name, state, c.Desc)
	}

	res, err := install(cands, filepath.Join(dir, "repo"), dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "install: "+err.Error())
		return 1
	}
	for _, s := range res.Skipped {
		fmt.Fprintln(os.Stderr, "skipped "+s)
	}
	if dryRun {
		fmt.Fprintf(os.Stderr, "dry run: would install %s\n",
			strings.Join(res.Installed, ", "))
		return 0
	}
	if len(res.Installed) == 0 {
		fmt.Fprintln(os.Stderr, "install: nothing was installed")
		return 1
	}
	fmt.Printf("installed %s\n", strings.Join(res.Installed, ", "))
	return 0
}

func runCompletion(shell string) int {
	switch shell {
	case "bash":
		fmt.Print(withCommands(bashCompletion))
	case "zsh":
		fmt.Print(withCommands(zshCompletion))
	default:
		fmt.Fprintln(os.Stderr, "completion needs bash or zsh")
		return 2
	}
	return 0
}

// printHelp is generated from the same keymap and exit code table that
// the README documents, so the two cannot drift.
// printHelp writes the help text. The usage block is generated from the
// command table, so a command cannot be added to the binary without
// appearing here (review D9).
func printHelp(cfg Config) {
	help := []string{
		"skillman — manage opencode skills",
		"",
		"Usage:",
		"  skillman                        launch TUI (TTY)",
	}
	for _, c := range commands {
		line := "  skillman " + c.name
		if c.arg != "" {
			line += " " + c.arg
		}
		for _, f := range c.flags {
			line += " [--" + strings.TrimPrefix(f, "--") + "]"
		}
		help = append(help, line)
	}
	help = append(help,
		"",
		"Flags:",
		"  --plain            no colors/markdown styling (also NO_COLOR, TERM=dumb)",
		"  --no-animations    disable spinner/transitions (also NO_ANIMATIONS, REDUCED_MOTION, CI)",
		"  --skills-dir DIR   override skills directory (also SKILLMAN_SKILLS, config.yaml)",
		"  --older-than AGE   trash purge age: 30d or 720h",
		"  --all              trash purge: remove every entry",
		"  --yes, -y          confirm a destructive action",
		"  -h, --help         show this help",
		"  --version          print the version and exit",
		"",
		"Exit codes:",
	)
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
		"Full documentation: README.md, man skillman",
	)
	fmt.Println(strings.Join(help, "\n"))
}
