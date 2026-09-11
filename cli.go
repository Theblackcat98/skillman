package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Non-interactive CLI: list, view, validate, delete.

func runList(jsonOut, plain bool) int {
	skills, err := ScanSkills(skillsDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error: "+err.Error())
		return 1
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
		_ = enc.Encode(rows)
		return 0
	}
	if len(skills) == 0 {
		fmt.Println("No skills in " + skillsDir())
		return 0
	}
	for _, s := range skills {
		b, _ := s.Badge()
		fmt.Printf("%-28s [%s] %s\n", s.Name, b, s.Desc)
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
		_ = enc.Encode(rows)
	} else {
		ok, warn, bad := summarize(skills)
		fmt.Printf("%d ok, %d warn, %d err (%d total)\n", ok, warn, bad, len(skills))
		for _, s := range skills {
			if !s.Valid {
				fmt.Printf("- %s: %s\n", s.Name, strings.Join(s.Issues, "; "))
			}
		}
	}
	for _, s := range skills {
		if !s.Valid {
			for _, is := range s.Issues {
				if strings.HasPrefix(is, "missing SKILL.md") {
					return 2
				}
			}
			return 1
		}
	}
	return 0
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
			dest, err := deleteSkillToTrash(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, "delete error: "+err.Error())
				return 1
			}
			fmt.Printf("moved %s to %s (restore manually to undo)\n", name, dest)
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, "skill not found: "+name)
	return 1
}

func printHelp() {
	fmt.Println(`skillman — manage opencode skills

Usage:
  skillman                        launch TUI (TTY)
  skillman list [--json]          list skills
  skillman view <name>            show skill preview
  skillman validate [--json]      validate frontmatter
  skillman delete <name> --yes    move skill to trash

Flags:
  --plain            no colors/markdown styling (also NO_COLOR, TERM=dumb)
  --no-animations    disable spinner/transitions (also NO_ANIMATIONS, REDUCED_MOTION, CI)
  --skills-dir DIR   override skills directory (also SKILLMAN_SKILLS)
  -h, --help         show this help

TUI keys:
  j/k, arrows move · Tab pane · / filter · : command · e edit
  d delete · u undo · v validate · r rescan · ? help · q quit`)
}
