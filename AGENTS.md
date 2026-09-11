# AGENTS.md — SkillMan Workflow

Stack: Go + BubbleTea/Bubbles/LipGloss/Glamour on Termux (Android). Reference: `tui-philosophy.md`. Plan: `PLAN.md`.

## Loop (every phase, in order)

Repeat for each phase in `PLAN.md`, one phase at a time:

```
1. IMPLEMENT -> 2. CHECK -> 3. COMMIT -> 4. NEXT PHASE
```

### 1. IMPLEMENT
- Work only on the files listed for the current phase. Keep it VERY simple.
- Keyboard-first, resize-safe (80×24), tokens not hardcoded ANSI, async I/O as BubbleTea Cmds (cancellable with `Esc`).
- Respect `NO_COLOR`, `TERM=dumb`, `NO_ANIMATIONS`/`REDUCED_MOTION`/`CI`, non-TTY.
- Use clear, concise technical English. No emojis in code unless requested.

### 2. CHECK
- Run, in this order, with 60–120s timer cut-off:
  - `go vet ./...`
  - `go build -o tmp/skillman .`
  - Phase-specific check from `PLAN.md` (render, resize, `NO_COLOR=1`, filter, delete→undo, `list --json`, etc.).
- Fix failures before committing. Never commit broken builds.
- For TUI visuals: test at 80×24 and narrow width, plus `TERM=dumb` and piped output.

### 3. COMMIT
- Inspect before commit: `git status`, `git diff`, `git log --oneline -5`.
- Stage only intended files. Never commit secrets, trash, temp clones, or binaries.
- Conventional Commits: `<type>(<scope>): <subject>` (max 50 chars, imperative).
  - Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`.
  - Example: `feat(tui): add skill list with preview pane`
- One phase = one commit (squash noise with `git rebase -i` if needed, only before push).
- Do not amend a failed/pushed commit — fix forward with a new commit.
- Only commit, push, or open PRs when the user explicitly asks. Default: commit locally.

### 4. NEXT PHASE
- Mark phase done in `PLAN.md` (✅), move to next phase's IMPLEMENT step.
- If blocked: stop, explain blocker, do not skip CHECK or COMMIT.
- Branching: trunk-based, small commits on `main`. Use `feature/<desc>` branches only when asked; delete after merge.

## Termux notes
- Prefix long commands with `timeout 120`. Use `workdir` instead of `cd`.
- Go module cache lives under Termux prefix; `go build` output goes to project-local `./tmp/` (gitignored), never `/tmp` (not writable here) and never into the repo root.
- `.gitignore` covers: binaries (`skillman`, `tmp/`), `*.test`, coverage, `dist/`, IDE files.

## File map
- `tui-philosophy.md` — TUI baseline (do not weaken: `q/?//`, resize, NO_COLOR, clean exit, async, config, `--help`, non-interactive).
- `PLAN.md` — phases 0–6, each with goal/files/check.
- `*.go` — implementation per current phase only.
