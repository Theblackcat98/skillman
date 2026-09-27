package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// version is the release this build reports. It is the only place a
// version number lives, so `--version` and the man page agree.
const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var (
		plainFlag      = false
		noAnimFlag     = false
		jsonOut        = false
		namesOnly      = false
		assumeYes      = false
		purgeAll       = false
		skillsOverride = ""
		olderThan      = ""
		positional     []string
	)

	// A bare "help" is not a command: `skillman view help` must show the
	// skill named help, not this text. --help and -h still work.
	wantHelp := false
	wantVersion := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-h", "--help":
			wantHelp = true
		case "--version":
			wantVersion = true
		case "--plain":
			plainFlag = true
		case "--no-animations":
			noAnimFlag = true
		case "--json":
			jsonOut = true
		case "--names":
			namesOnly = true
		case "--yes", "-y":
			assumeYes = true
		case "--all":
			purgeAll = true
		case "--skills-dir", "--older-than":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s needs a value\n", a)
				return 2
			}
			i++
			if a == "--skills-dir" {
				if args[i] == "" {
					// An empty override silently fell back to the
					// default directory, which is never what was meant.
					fmt.Fprintln(os.Stderr, "--skills-dir needs a non-empty value")
					return 2
				}
				skillsOverride = args[i]
			} else {
				olderThan = args[i]
			}
		default:
			if v, ok := strings.CutPrefix(a, "--skills-dir="); ok {
				if v == "" {
					fmt.Fprintln(os.Stderr, "--skills-dir needs a non-empty value")
					return 2
				}
				skillsOverride = v
			} else if strings.HasPrefix(a, "-") {
				fmt.Fprintln(os.Stderr, "unknown flag: "+a)
				return 2
			} else {
				positional = append(positional, a)
			}
		}
	}
	// Config first: it decides the skills directory and the theme, and
	// the flag still wins over it.
	cfg, cfgErr := loadConfig()
	cfg.apply(skillsOverride)

	if wantVersion {
		fmt.Println("skillman " + version)
		return 0
	}
	if wantHelp {
		// The help text lists the keys actually in force, so it needs
		// the config; it does not need to complain about a bad one.
		printHelp(cfg)
		return 0
	}
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "config warning: "+cfgErr.Error()+" (using defaults)")
		logf("config error: %v", cfgErr)
	}

	plain := plainOutput(plainFlag)
	noAnim := !animationsEnabled(noAnimFlag)

	if len(positional) > 0 {
		switch positional[0] {
		case "list", "ls":
			return runList(jsonOut, plain, namesOnly)
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
		case "trash":
			return runTrash(positional[1:], jsonOut, purgeAll, olderThan)
		case "completion":
			if len(positional) < 2 {
				fmt.Fprintln(os.Stderr, "completion needs bash or zsh")
				return 2
			}
			return runCompletion(positional[1])
		case "install":
			// Exit 4, distinct from "destructive action refused", and
			// no pointer to a file that is on its way out.
			fmt.Fprintln(os.Stderr, "install from GitHub is not built yet")
			return 4
		case "help":
			// Only when it is the whole command line: `skillman view
			// help` must show the skill named help, not this text.
			if len(positional) == 1 {
				printHelp(cfg)
				return 0
			}
			fmt.Fprintln(os.Stderr, "unknown command: help")
			return 2
		default:
			fmt.Fprintln(os.Stderr, "unknown command: "+positional[0])
			printHelp(cfg)
			return 2
		}
	}

	// --json on its own means list --json; launching a TUI would just
	// silently ignore the flag (audit B8).
	if jsonOut || namesOnly {
		return runList(true, true, namesOnly)
	}

	// No TTY -> behave as list --plain.
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return runList(jsonOut, true, false)
	}

	m := NewModel(plain, noAnim, cfg)
	m.restoreState(loadState())
	// Cap redraws: full-screen rewrites on a slow terminal can queue up
	// behind spinner frames and delay keypresses.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithFPS(20))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tui error: "+err.Error())
		logf("tui error: %v", err)
		return 1
	}
	// Persist the session on every clean exit path, including Ctrl-C.
	if mm, ok := final.(Model); ok {
		if serr := saveState(mm.state()); serr != nil {
			logf("state save: %v", serr)
		}
	}
	return 0
}
