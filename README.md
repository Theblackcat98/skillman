# SkillMan

A keyboard-first TUI and CLI for managing `opencode` skills in
`~/.config/opencode/skills/`.

Browse, preview, validate, edit and trash-delete skills. `opencode`
itself never edits or removes a skill by accident, so neither does
SkillMan: a delete moves the directory to a trash you can restore from.

```
┌ skills ────────────────┐┌ preview · longbody ───────────────────┐
│› longbody          [ok] ││ longbody                             │
│  has a very long body…  ││ has a very long body for scroll test │
│  analysis         [warn]││ analysis · license: MIT              │
│…                       ││                                     │
│                        ││ ## Issues                            │
│                        ││ - missing description                │
└────────────────────────┘│ ────────────────────────────────     │
 18 ok · 1 warn · 0 err            normal    5–21/40           │
 / filter · : cmd · e edit · d del · u undo · ? help · q quit    │
```

## Quickstart

```sh
go build -o tmp/skillman .      # from a checkout
go install                     # or put the binary on your PATH

skillman                       # TUI on a terminal
skillman list                  # plain list when piped, or on a dumb terminal
skillman validate              # check every skill's frontmatter
skillman view <name>           # print one skill
skillman delete <name> --yes   # move it to the trash (never rm -rf)
skillman trash list            # see what is in the trash
skillman trash restore <name>  # put it back
```

Enable shell completion:

```sh
skillman completion bash >> ~/.bashrc
skillman completion zsh  >> ~/.zshrc
```

## Usage

### Commands

| Command | What it does |
|---|---|
| *(none)* | Launch the TUI. On a non-TTY, or with stdout/stderr not a terminal, falls back to `list --plain`. |
| `list [--json] [--names] [--long]` | List skills. `--names` prints one name per line, for completion. `--long` adds category, licence, compatibility, size and mtime. |
| `view <name> [--plain]` | Print one skill: description, issues, body. |
| `validate [--json]` | Check every skill's frontmatter. Exit code follows the worst issue: 2 for a broken frontmatter or a missing `SKILL.md`, 1 for a warning, 0 clean. `--json` adds `severity` and `issue_codes`. |
| `delete <name> --yes` | Move a skill to the trash. Refuses without `--yes`. |
| `trash list [--json]` | List trashed skills, newest first. |
| `trash restore <name>` | Put a trashed skill back under its original name. |
| `trash purge --older-than 30d \| --all` | Remove trashed skills. Needs an explicit selector. |
| `completion bash\|zsh` | Print a completion script. |
| `install <url>` | Not built in this version. Exits 4. |

Aliases: `ls` for `list`, `show` for `view`, `check` for `validate`, `rm` for `delete`.

### Flags

| Flag | Effect |
|---|---|
| `--plain` | No colour, no markdown styling. |
| `--no-animations` | No spinner, no transitions. |
| `--skills-dir DIR` | Override the skills directory. Wins over `SKILLMAN_SKILLS` and `config.yaml`. |
| `--json` | Machine-readable output. `list`, `validate`, `trash list` only. |
| `--names` | One skill name per line (`list` only). |
| `--long` | `list` only: add category, licence, size and mtime. |
| `--yes`, `-y` | Assume yes for a destructive action. |
| `--older-than AGE` | `trash purge` age: `30d` or `720h`. |
| `--all` | `trash purge`: remove every entry. |
| `--` | End of flags. Everything after it is a name, so a skill called `-h` is reachable. |
| `-h`, `--help` | Help, generated from the same tables as this file. |
| `--version` | Print the version and exit. |

A flag a command does not support is an error, not a no-op: `skillman
view x --json` exits 2 rather than printing text a script would try to
parse as data.

### TUI keys

This is the canonical key table. `keyTable` in `keymap.go` is the only
definition; the `?` overlay, `--help` and this table are all generated
from it, and `TestDocsMatchKeymap` fails if this table drifts.

| Key | Action | Rebind as |
|---|---|---|
| `j / down` | move selection down | `down` |
| `k / up` | move selection up | `up` |
| `g / home` | first skill | `top` |
| `G / end` | last skill | `bottom` |
| `PgUp / PgDn` | page (Ctrl-B / Ctrl-F) | |
| `Tab / Enter` | switch pane · focus preview | `pane` |
| `H / ?` | this help | `help` |
| `/` | filter · esc keeps, esc esc clears | `filter` |
| `:` | command: edit delete validate reload clear quit | `command` |
| `e` | open SKILL.md in $EDITOR | `edit` |
| `d` | delete to trash | `delete` |
| `u` | undo a delete, 30s window | `undo` |
| `v` | validate all skills | `validate` |
| `r` | rescan the skills directory | `rescan` |
| `q` | quit from the base layer | `quit` |
| `Esc` | back one layer | |

The footer is a width-tiered reminder, not a second keymap: it shows the
longest hint set that fits the terminal, so `? help · q quit` survives
as long as there is room.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | ok |
| 1 | validation issues, or skill not found |
| 2 | bad usage, or a skill has a broken frontmatter or no SKILL.md |
| 3 | destructive action refused without --yes |
| 4 | command not built in this version |

### Environment variables

| Variable | Effect |
|---|---|
| `SKILLMAN_SKILLS` | skills directory override |
| `XDG_CONFIG_HOME` | config.yaml and state.json live here |
| `XDG_DATA_HOME` | trash and skillman.log live here |
| `VISUAL` | editor to open SKILL.md with, before EDITOR |
| `EDITOR` | editor to open SKILL.md with |
| `NO_COLOR` | set: no colour, anywhere |
| `SKILLMAN_COLOR` | always: colour even when piped |
| `TERM` | dumb: no colour and no animation |
| `NO_ANIMATIONS` | set: no spinner, no transitions |
| `REDUCED_MOTION` | set: no spinner, no transitions |
| `CI` | set: no spinner, no transitions |

Skills directory precedence: `--skills-dir`, then `SKILLMAN_SKILLS`,
then `config.yaml`'s `skills_dir`, then
`$XDG_CONFIG_HOME/opencode/skills`.

### Config

`$XDG_CONFIG_HOME/skillman/config.yaml`. Every field has a working
default, so the file is optional. A missing file is not an error; a
broken file prints a warning and runs on defaults rather than refusing
to start.

```yaml
skills_dir: ~/.config/opencode/skills   # where skills live
accent: "#34D399"                      # UI accent colour
keys:                                  # rebind an action
  delete: D
  help: F1
```

`accent` takes any hex colour and drives the whole palette. `keys` binds
a new key to an action name from the *Rebind as* column above; unknown
names and empty bindings are ignored and logged. A rebound key is
translated to its built-in equivalent before dispatch, so the key
dispatcher stays the only place that says what an action does. The `?`
overlay prints the keys actually in force.

### State

`$XDG_CONFIG_HOME/skillman/state.json`, written atomically on every
clean exit. It holds the last selection, per-skill preview scroll, pane
focus and terminal width, so a restart resumes where you stopped.
Missing, corrupt or stale-schema state starts fresh instead of failing.

### Delete, undo and the trash

A delete moves `skills/<name>` to
`$XDG_DATA_HOME/skillman/trash/<name>-<timestamp>`. Nothing is ever
`rm -rf`ed. Moves fall back to copy-then-remove when the skills
directory and the data directory are on different filesystems, which is
normal on Termux.

- In the TUI, `u` restores within 30 seconds of the delete.
- After that, or from another process: `skillman trash restore <name>`.
- `trash restore` refuses if the name is taken again, and says where the
  copy is, rather than overwriting the new skill.
- The trash grows until you purge it: `trash purge --older-than 30d`, or
  `--all`. A bare `trash purge` does nothing and exits 2.

## Architecture

Single Go module, single package, one binary. Deliberately flat: at
~3k lines the cost of a package tree would exceed the benefit.

| File | Owns |
|---|---|
| `main.go` | Flag parsing, command dispatch, exit codes, config and state wiring. |
| `cli.go` | Non-interactive commands, completion scripts, `--help`. |
| `keymap.go` | **The** keymap and exit-code tables. Everything user-facing is generated from here. |
| `config.go` | Paths, environment gates, `config.yaml`, key overrides. |
| `state.go` | `state.json` load/save, model → state and back. |
| `skills.go` | `Skill`, frontmatter parsing, validation, badges, scan. |
| `trash.go` | Trash delete, undo, restore, purge, cross-device move. |
| `model.go` | Bubble Tea model: state, selection, preview cache, scroll. |
| `update.go` | Key dispatch and message handling. |
| `view.go` | Rendering, layout arithmetic, width handling. |
| `theme.go` | Semantic colour tokens. Never hardcoded ANSI elsewhere. |
| `editor.go` | `$EDITOR` argv splitting. |
| `scripts/smoke-tmux.sh` | One tmux session, end to end. |

Layer rules, in one line each:

- `skills.go` and `trash.go` know nothing about Bubble Tea or LipGloss.
- `view.go` never performs I/O and never mutates state.
- Every side effect is a `tea.Cmd` returning a typed message; no method
  both mutates and does I/O.
- Widths are counted in terminal cells with `go-runewidth`, never runes.
  A CJK or emoji name occupies two cells; counting runes pushes the box
  borders off screen.
- Layout geometry has exactly one owner: `Model.paneW`. The renderer and
  the viewport sizing both read it, so the glamour wrap width cannot
  drift from the box interior.
- Untrusted content is skill file content. It reaches a terminal through
  `truncate`, which never splits an escape sequence.

## Design rules

The rules that govern this project, distilled from the TUI reference it
was built against. That reference has been absorbed here and removed.

**Baseline — all of it is required, not aspirational.**

- Keyboard-only operation. Everything is reachable without a mouse.
- Discoverable help: `?` overlay, context footer, `--help`, README.
- Incremental filter (`/`), a command line (`:`), a real help overlay.
- Visible state: loading, empty, no-match, error, success, mode.
- Resize-safe at 80×24, degrading gracefully below it. No fixed
  dimensions, no overflow, no broken borders.
- Clean terminal lifecycle: cursor, screen and raw mode restored on
  normal exit, `Ctrl-C` and signals. No alt-screen leak.
- Configurable: config file, env vars, flags, keybinding overrides,
  theme, persisted state.
- Terminal respect: `NO_COLOR`, `TERM=dumb`, 256/truecolor, and never
  colour alone to convey meaning — every severity carries a text label
  (`[ok]`, `[warn]`, `[err]`), or a mark (`!`, `x`) where a word will not
  fit.
- Async: no I/O on the update path. Scans and the editor run as commands.
- Efficient rendering: a preview render cache, a spinner that only ticks
  while loading, and a 20fps cap.
- Meaningful exit codes, confirmation plus undo for destructive actions.
- Non-interactive mode: flags, JSON, stable exit codes.

**Severity is data, not prose.** Every problem is an `Issue` with a
stable `Code` and a `Severity`. The badge, the row colour, the
`validate` summary and the exit code all read one value, so rewording a
message can never change an exit code. A malformed frontmatter is one
error, not three warnings about a file nothing can read.

**One layout owner.** `layout.go` turns a terminal size into the whole
frame: pane widths, box heights, content cells and the per-row cell
budget. Drawing and viewport sizing both read it, so they cannot
disagree. Pane geometry used to be recomputed in two places, and that
duplication is what produced an off-by-one in the first review.

At 70 columns and below the frame shows one pane and `Tab` switches. When
a row is too narrow for a severity word, it spends one cell on a mark
instead (`!` warning, `x` error, blank when clean) so the name keeps the
cells that identify the skill. `?` help scrolls: the keymap is taller
than a short terminal, and the overlay title carries the visible range.

**Layers.** Base view, then overlays (filter, command, help), then
modals (confirm), then notifications (toast), then the status line.
`Esc` pops exactly one layer. `q` quits only from the base layer.
`Ctrl-C` quits from every layer, because an app you cannot kill is
broken. A modal dims the background instead of hiding it, and keeps its
own state when reopened. Toasts never steal focus.

**UX.** State is always visible. Every action gets feedback under
~100ms; anything slower gets a spinner. Latency is managed, not hidden.
Modes are explicit in the status bar and the mode indicator is never
truncated — you need it most when the frame is tight. Focus is never
ambiguous: the active pane has a highlighted border, the inactive one is
dimmed. Errors are inline, plain, actionable, and never destroy the
frame. Undo beats confirmation: the TUI deletes with a 30s undo window,
and `trash restore` covers everything after that. Empty states name the
next action: `(no skills — r to rescan)` and
`(no match for /x) — esc clears the filter` are different states on
purpose.

**Animation.** Only when it communicates: a spinner for unknown latency,
a dimmed background for a modal, a highlighted pane for focus. Never
block input, always skippable, short (100–300ms), 10–20fps. Disabled by
non-TTY, `TERM=dumb`, `CI`, `NO_ANIMATIONS`, `REDUCED_MOTION` and
`--no-animations`. No spinners for instant operations, no typewriter
effects, no full-screen wipes. Transitions never reset scroll, selection
or the filter query.

**Accessibility.** No meaning carried by colour alone. Wide characters
and combining marks are measured in cells. Focus is always visible.
Animation is always skippable.

**Documented deviations.** Three places where this build knowingly
differs from the baseline above, each with a reason:

- `Ctrl-C` quits from every layer instead of cancelling the current
  operation first. An app that cannot be killed is worse than one that
  quits when the user expected a cancel.
- `?` in filter mode types a literal `?`; the help overlay is opened from
  normal mode only, so filtering for a question mark is possible.
- `TERM=dumb` drops colour and animation but still uses the alt screen on
  a real TTY. A dumb terminal can run a full-screen program; it just
  cannot style one.

`Shift-Tab` and `Ctrl-P` are unbound. The key table is in
[keymap.go](keymap.go), and one row added there reaches the `?` overlay,
`--help`, the man page and this file at once.

**Plain by default off-screen.** Piped or redirected output is plain:
`skillman view x > out.md` used to write glamour's padding as roughly
forty escape sequences per filler cell. `SKILLMAN_COLOR=always` asks for
colour anyway, and `--plain` always wins.

**Untrusted content.** A `SKILL.md` is third-party input, and a
directory name is not YAML at all: it is whatever the user unzipped. Any
control byte in either one — `ESC[2J`, `ESC[1;1H`, `OSC 0`, `OSC 52`, a
tab, a C1 code, a bidi override — is escaped to a visible `\xNN` before
the TUI, the CLI or the JSON encoder sees it. The original bytes are
still legible, because a user debugging a broken skill needs to see what
is actually in the file.

## Development

```sh
go vet ./...            # required
gofmt -l .              # must be empty
go build -o tmp/skillman .
go test ./...           # required
scripts/smoke-tmux.sh   # one real TUI session (needs the build above)
```

CI also runs `go test -race`, `go mod tidy -diff`, `staticcheck`,
`shellcheck`, `govulncheck`, a cross-build for windows, darwin and
linux, and the tmux smoke script in its own job, so the TUI has
automated interaction coverage at all.

TUI changes are checked at 80×24, 50×20 and 20×10, with `NO_COLOR=1`,
with `TERM=dumb`, and with a CJK skill name in the list. A new key, exit
code or environment variable is added to `keymap.go` first, and the
`?` overlay, `--help` and the man page follow from it automatically.

`AGENTS.md` holds the working loop and the Termux notes.

### Tests

| File | What it covers |
|---|---|
| `view_test.go` | Rendered-frame invariants: width, row count, badges, the selected row, the footer, the empty states. |
| `golden_test.go` | Byte-exact frames in `testdata/` across a size table, both colour modes, both overlays and the empty states. |
| `update_test.go` | Key-driven model tests: every mode, the timer generations, focus routing, the confirm gate, resize. |
| `skills_test.go` | Frontmatter parsing, filtering, scanning. |
| `sanitize_test.go` | The untrusted-content boundary, field by field. |
| `cli_test.go` | Exit-code contract, flag handling, trash CLI, completion. |
| `trash_test.go` | Delete, undo, restore, purge, permissions. |
| `config_test.go` | Config precedence, key overrides, state round-trip. |
| `editor_test.go` | `$EDITOR` splitting, edit-path resolution. |
| `fuzz_test.go` | Fuzz targets for the frontmatter parser, the sanitizer and the scanner. |
| `stress_test.go` | The same three, with a seeded PRNG, for platforms where `go test -fuzz` is unavailable. |
| `hygiene_test.go` | `TestMain` fails the suite if any test writes into the real trash, plus stdout capture. |
| `docs_test.go`, `man_test.go` | This file and the man page against the tables in `keymap.go`. |

Regenerate the golden frames after an intentional layout change:

```sh
go test -run TestGoldenFrames -update
```

Fuzz on a supported platform (`-fuzz` needs linux or darwin, not
android/arm64):

```sh
go test -run '^$' -fuzz FuzzParseFrontmatter -fuzztime 60s
```

Any test that deletes, restores, writes state or logs must call
`isolatedEnv(t)`, which points the skills directory, the trash and the
config at temp directories. `TestMain` diffs the real trash around the
whole suite and fails if it grew, so this cannot regress silently.

## Status

Every phase in both roadmaps below is either `done` or `planned`; there
is no in-flight work. See [Roadmap](#roadmap).

Fixed along the way, each with a regression test: the badge disappearing
at 80 columns, the selected row vanishing at the list bottom, `Ctrl-C`
being swallowed in a modal, a success toast that never dismissed under
`NO_ANIMATIONS`, an older timer clearing a newer toast, the editor
rendering inside the alt screen, invalid YAML reported as "missing
name", cross-filesystem deletes, undo clobbering a re-created skill,
overlays blanking the background, three wasted rows and a header that
wrapped below 37 columns, CJK names breaking the frame, `validate`'s
exit code depending on alphabetical order, `help` shadowing a skill
named `help`, and an empty `--skills-dir=` being silently ignored.

The full 2026-09-27 review, with the redesign sketch and a longer
roadmap, is in [docs/review-2026-09-27.md](docs/review-2026-09-27.md).

## Roadmap

The phased plan this project was built from:

| Phase | Status | What it delivered |
|---|---|---|
| 0 · Repo bootstrap | done | `.gitignore`, `AGENTS.md`, Go module. |
| 1 · Skeleton TUI | done | Header, list, preview, status, footer; clean exit. |
| 2 · Scan and validate | done | Real skills, YAML frontmatter, Glamour preview, badges. |
| 3 · Filter, palette, help, toasts | done | `/`, `:`, `?`, toasts, layer stack. |
| 4 · Edit and delete with undo | done | `$EDITOR`, trash, 30s undo, event log. |
| 5 · Install from GitHub | planned | `i` key, URL prompt, `git clone --depth 1`, candidate checklist. |
| 6 · Hybrid CLI | done | `list`, `view`, `validate`, `delete`, `--help`, non-TTY fallback. |
| 7 · Core correctness | done | Badges at 80 columns, cursor row, focus keys, `Ctrl-C`, distinct empty states, footer. |
| 8 · Feedback and lifecycle | done | Timer generations, editor suspend, YAML errors, safe trash lifecycle. |
| 9 · Test suite and CI | done | Unit and render tests, tmux smoke script, CI. |
| 10 · UI/UX polish | done | Dimmed overlays, no wasted rows, scroll position, preview banner, cell-accurate width, per-skill scroll. |
| 11 · Config, state, trash CLI, completion | done | `config.yaml`, `state.json`, `trash list/restore/purge`, `completion bash\|zsh`. |
| 12 · Documentation consolidation | done | This file, as the single source of truth. |
| 13 · Man page and doc parity | planned | `docs/skillman.1` plus a parity check against this file. |

The 2026-09-27 review added a second roadmap, ordered so unblockers and
the safety net come first. The audit labels (`A1`, `C1`, `G3`, …) refer
to findings in [docs/review-2026-09-27.md](docs/review-2026-09-27.md).

| Review phase | Status | What it delivered |
|---|---|---|
| 0 · Stop the bleeding | done | `-race` and a Go version matrix in CI, staticcheck, shellcheck, govulncheck, cross-builds, tmux smoke in CI, `LICENSE`, `--version`, a documented reason for the lipgloss pin. |
| 1 · Security | done | One sanitize boundary for every string that came from a skill file. |
| 2 · Safety net | done | Golden frames over a size table, cell-width and flush-frame assertions, key-driven model tests for `update.go`, frontmatter and sanitize fuzzers plus seeded stress runs, a bounded preview cache, an editor test set, and a trash-writing guard in `TestMain`. |
| 3 · Cheap correctness | done | Dead code deleted, `--long` surfacing size and mtime, plain-by-default output with a colour opt-in, a one-table flag contract that rejects unsupported flags, `--` for flag-shaped skill names, and a create-confirmation before `e` writes a missing `SKILL.md`. |
| 4 · Domain model | done | Typed `Issue`/`Severity` with stable codes, one reload path used by every operation, malformed frontmatter reported as one error instead of three warnings. |
| 5 · Layout | done | `layout.go` owns all frame geometry, so drawing and sizing read the same numbers. The `?` overlay scrolls and reports its range. Rows keep the name legible at 20 columns by dropping the severity word for a one-cell mark. |
| 6 · Selection state | planned | Selection by name, preview cache invalidated on every reload. |
| 7 · CLI surface | planned | `list --long`, `--json` accepted or rejected everywhere, one load helper. |
| 8 · Config, state, keymap | done | Landed as plan phases 11 and 12. |
| 9 · Docs | planned | README (done), man page, parity test, stale-reference cleanup. |
| 10 · Memory and scale | in progress | Preview cache bounded at 24 renders (landed with the safety net). Bodies load lazily, next. |
| 11 · Install from GitHub | planned | Landed as plan phase 5, once the reload path it depends on exists. |

## Out of scope

GitHub autoinstall, mouse support, OSC 52 clipboard, multi-select and
sort UI, an enable/disable toggle, a skill creation wizard,
`skill-creator` integration, and update-from-remote.

## License

MIT. See [LICENSE](LICENSE).
