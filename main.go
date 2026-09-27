package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
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

// globalFlags apply to every command and to the bare TUI launch.
var globalFlags = map[string]bool{
	"--help": true, "-h": true, "--version": true,
	"--skills-dir": true, "--no-animations": true, "--plain": true,
}

// commandFlags is what each command actually honours. A flag that is
// accepted and then ignored is worse than one that is rejected: the
// script asks for JSON, gets rendered text, and fails later somewhere
// unrelated. So anything not listed here is an error (review A9, E15).
var commandFlags = map[string]map[string]bool{
	"list":       {"--json": true, "--names": true, "--long": true},
	"view":       {},
	"validate":   {"--json": true},
	"delete":     {"--yes": true, "-y": true},
	"trash":      {"--json": true, "--all": true, "--older-than": true},
	"completion": {},
	"install":    {"--ref": true},
}

// aliases are the alternate spellings, mapped to the canonical command so
// the flag table has one entry per command rather than one per spelling.
var commandAliases = map[string]string{
	"ls": "list", "show": "view", "check": "validate", "rm": "delete",
}

// unsupportedFlags returns the flags that were passed but do nothing for
// cmd, in a stable order, so the error names them.
func unsupportedFlags(cmd string, passed map[string]bool) []string {
	canonical, ok := commandAliases[cmd]
	if !ok {
		canonical = cmd
	}
	allowed, known := commandFlags[canonical]
	var bad []string
	for _, f := range sortedKeys(passed) {
		if globalFlags[f] {
			continue
		}
		if !known || !allowed[f] {
			bad = append(bad, f)
		}
	}
	// --names and --json are the same output, so asking for both is a
	// contradiction rather than a preference.
	if canonical == "list" && passed["--names"] && passed["--json"] {
		bad = append(bad, "--names --json")
	}
	return bad
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func run(args []string) int {
	var (
		plainFlag      = false
		noAnimFlag     = false
		jsonOut        = false
		namesOnly      = false
		longOut        = false
		assumeYes      = false
		purgeAll       = false
		skillsOverride = ""
		olderThan      = ""
		positional     []string
		// rest holds the arguments after a bare "--", so a skill named
		// like a flag is reachable.
		rest          []string
		afterDashDash bool
		// passed records the flags the user actually wrote, so the
		// command/flag contract can be checked against them rather than
		// guessed from the command name.
		passed = map[string]bool{}
	)

	// A bare "help" is not a command: `skillman view help` must show the
	// skill named help, not this text. --help and -h still work.
	wantHelp := false
	wantVersion := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		// Everything after -- is a name, never a flag, so a skill
		// called --names or -h is still reachable.
		if afterDashDash {
			rest = append(rest, a)
			continue
		}
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
			passed["--json"] = true
		case "--names":
			namesOnly = true
			passed["--names"] = true
		case "--long":
			longOut = true
			passed["--long"] = true
		case "--yes", "-y":
			assumeYes = true
			passed[a] = true
		case "--all":
			purgeAll = true
			passed["--all"] = true
		case "--":
			afterDashDash = true
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
			} else if strings.HasPrefix(a, "-") && a != "-" {
				fmt.Fprintln(os.Stderr, "unknown flag: "+a)
				return 2
			} else {
				positional = append(positional, a)
			}
		}
	}
	// A name given after -- wins over a bare positional, so
	// `skillman view -- --weird-name` works.
	if len(rest) > 0 {
		positional = append(positional, rest...)
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
		cmd := positional[0]
		// A flag that is accepted and ignored is worse than one that is
		// rejected, so check the whole command line against one table.
		if known := cmd == "list" || cmd == "ls" || cmd == "view" || cmd == "show" ||
			cmd == "validate" || cmd == "check" || cmd == "delete" || cmd == "rm" ||
			cmd == "trash" || cmd == "completion" || cmd == "install" || cmd == "help"; known {
			if bad := unsupportedFlags(cmd, passed); len(bad) > 0 {
				fmt.Fprintf(os.Stderr, "%s does not take %s\n", cmd, strings.Join(bad, " or "))
				fmt.Fprintln(os.Stderr, "run: skillman --help")
				return 2
			}
		}
		switch cmd {
		case "list", "ls":
			return runList(jsonOut, plain, namesOnly, longOut)
		case "view", "show":
			if len(positional) < 2 {
				fmt.Fprintln(os.Stderr, "view needs a skill name")
				return 2
			}
			if len(positional) > 2 {
				// `view a b c` used to drop b and c silently.
				fmt.Fprintln(os.Stderr, "view takes one skill name; got "+strconv.Itoa(len(positional)-1))
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
			if len(positional) > 2 {
				fmt.Fprintln(os.Stderr, "delete takes one skill name; got "+strconv.Itoa(len(positional)-1))
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
			// Exit 4, distinct from "destructive action refused".
			fmt.Fprintln(os.Stderr, "install from a git repository is not built yet")
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
			fmt.Fprintln(os.Stderr, "unknown command: "+cmd)
			printHelp(cfg)
			return 2
		}
	}

	// --json on its own means list --json; launching a TUI would just
	// silently ignore the flag (audit B8).
	if jsonOut || namesOnly {
		return runList(true, true, namesOnly, longOut)
	}

	// No TTY -> behave as list --plain.
	if !term.IsTerminal(int(os.Stdout.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return runList(jsonOut, true, false, longOut)
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
