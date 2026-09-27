package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	plainFlag := false
	noAnimFlag := false
	jsonOut := false
	skillsOverride := ""
	assumeYes := false
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help", "help":
			printHelp()
			return 0
		case "--plain":
			plainFlag = true
		case "--no-animations":
			noAnimFlag = true
		case "--json":
			jsonOut = true
		case "--yes", "-y":
			assumeYes = true
		case "--skills-dir":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--skills-dir needs a value")
				return 2
			}
			i++
			skillsOverride = args[i]
		default:
			if strings.HasPrefix(a, "--skills-dir=") {
				skillsOverride = strings.TrimPrefix(a, "--skills-dir=")
			} else if strings.HasPrefix(a, "-") {
				fmt.Fprintln(os.Stderr, "unknown flag: "+a)
				return 2
			} else {
				positional = append(positional, a)
			}
		}
	}
	if skillsOverride != "" {
		_ = os.Setenv("SKILLMAN_SKILLS", skillsOverride)
	}
	plain := plainOutput(plainFlag)
	noAnim := !animationsEnabled(noAnimFlag)
	if noAnimFlag {
		noAnim = true
	}

	// Subcommands.
	if len(positional) > 0 {
		switch positional[0] {
		case "list", "ls":
			return runList(jsonOut, plain)
		case "view", "show":
			if len(positional) < 2 {
				fmt.Fprintln(os.Stderr, "view needs a skill name")
				return 2
			}
			return runView(positional[1], plain)
		case "validate", "check":
			return runValidate(jsonOut)
		case "delete", "rm":
			if len(positional) < 2 {
				fmt.Fprintln(os.Stderr, "delete needs a skill name")
				return 2
			}
			return runDeleteCLI(positional[1], assumeYes)
		case "install":
			fmt.Fprintln(os.Stderr, "install from GitHub lands in a later phase (see PLAN.md Phase 5)")
			return 3
		default:
			fmt.Fprintln(os.Stderr, "unknown command: "+positional[0])
			printHelp()
			return 2
		}
	}

	// --json on its own means list --json; launching a TUI would just
	// silently ignore the flag (audit B8).
	if jsonOut {
		return runList(true, true)
	}

	// No TTY -> behave as list --plain.
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return runList(jsonOut, true)
	}

	m := NewModel(plain, noAnim)
	// Cap redraws: full-screen rewrites on a slow terminal can queue up
	// behind spinner frames and delay keypresses.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithFPS(20))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui error: "+err.Error())
		logf("tui error: %v", err)
		return 1
	}
	return 0
}
