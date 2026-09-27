# AGENTS.md — SkillMan working rules

The project is one Go module, one package, one binary. Stack: Go +
BubbleTea + Bubbles + LipGloss + Glamour, running on Termux (Android).

**`README.md` is the single source of truth.** Do not duplicate facts
from it into another file. If you need one of these, read it there:

| What | Where in `README.md` |
|---|---|
| Canonical TUI key table | [Usage → TUI keys](README.md#tui-keys) |
| Design rules, baseline, layer model | [Design rules](README.md#design-rules) |
| Current roadmap, and the phase in progress | [Roadmap](README.md#roadmap) |
| What each file owns | [Architecture](README.md#architecture) |
| Commands, flags, exit codes, env vars, config, state | [Usage](README.md#usage) |
| Checks to run before committing | [Development](README.md#development) |

`docs/review-2026-09-27.md` and `docs/plan-archive.md` are history. They
are not maintained and are not a source of truth.

## Loop

Repeat for each phase in the README roadmap, one phase at a time:

```
1. IMPLEMENT -> 2. CHECK -> 3. COMMIT -> 4. NEXT PHASE
```

### 1. IMPLEMENT
- Work only on the files the phase needs. Keep it simple.
- Keyboard-first, resize-safe at 80×24, no hardcoded ANSI, no I/O on the
  update path. Every side effect is a `tea.Cmd` returning a typed message.
- Respect `NO_COLOR`, `TERM=dumb`, `NO_ANIMATIONS`, `REDUCED_MOTION`, `CI`
  and non-TTY.
- A new key, exit code or environment variable goes into `keymap.go`
  first; the `?` overlay, `--help` and the README follow from it.
- Measure width in terminal cells with `go-runewidth`, never runes.
- Use clear, concise technical English. No emojis in code unless asked.

### 2. CHECK
- Run, in this order, with a timer cut-off:
  - `go vet ./...`
  - `gofmt -l .` (must be empty)
  - `go build -o tmp/skillman .`
  - `go test ./...`
  - the phase-specific check from the README roadmap
- For TUI changes, also look at real frames at 80×24, 50×20 and 20×10,
  with `NO_COLOR=1`, with `TERM=dumb`, and with a CJK skill name in the
  list. `scripts/smoke-tmux.sh` drives one tmux session.
- Fix failures before committing. Never commit a broken build.

### 3. COMMIT
- Inspect first: `git status`, `git diff`, `git log --oneline -5`.
- Stage only the files the phase touched. Never commit secrets, trash,
  temp clones or binaries.
- Conventional Commits: `<type>(<scope>): <subject>`, max 50 chars,
  imperative. Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`,
  `perf`, `ci`. Example: `feat(tui): add skill list with preview pane`.
- One phase is one commit. Small commits on `main`; `feature/<desc>`
  branches only when asked, deleted after merge.
- Do not amend a pushed commit. Fix forward.
- Only commit, push or open a PR when explicitly asked. Default: commit
  locally.

### 4. NEXT PHASE
- Mark the phase done in the **README roadmap table**, then start the
  next phase's IMPLEMENT step.
- If blocked: stop, explain the blocker, do not skip CHECK or COMMIT.

## Termux notes
- Prefix long commands with `timeout`. Use the `workdir` parameter
  instead of `cd`.
- The Go module cache lives under the Termux prefix. Build output goes to
  the project-local `./tmp/` (gitignored), never `/tmp` (not writable
  here) and never into the repo root.
- `.gitignore` covers binaries (`skillman`, `tmp/`), `*.test`, coverage,
  `dist/`, `vendor/`, `node_modules` and IDE files.
