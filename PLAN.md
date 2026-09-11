# SkillMan — Phased Plan (Bubble Tea, Go)

Simple but beautiful TUI to manage skills in `~/.config/opencode/skills/`.
Stack: Go + BubbleTea + Bubbles + LipGloss + Glamour. See `tui-philosophy.md` for baseline.

Skills on disk: `skills/<name>/SKILL.md` (frontmatter: `name, description, license, compatibility, metadata`) + optional `scripts/`, `references/`.

---

## Phase 0 — Repo bootstrap ✅
- Goal: git repo, docs skeleton, Go module init.
- Files: `.gitignore`, `PLAN.md`, `AGENTS.md`, `go.mod`.
- Check: `git status` clean, `go version` ok.
- Commit: `chore(repo): init skillman with plan and workflow`

## Phase 1 — Skeleton TUI ✅ (MVP)
- Goal: runs as `go run .`, shows header / list (dummy data) / preview pane / status+footer, quits cleanly.
- Files: `main.go`, `model.go`, `update.go`, `view.go`, `theme.go`.
- Must: 80×24 safe + narrow collapse (Tab toggles list/preview), `q/Ctrl-C` restores terminal, `NO_COLOR` + `TERM=dumb` monochrome fallback, motion gate (`NO_ANIMATIONS`, `REDUCED_MOTION`, `CI`, non-TTY).
- Keys: `j/k,↑/↓,gg/G,PgUp/PgDn,Tab,q`.
- Check: `go vet ./... && go build -o tmp/skillman . && ./tmp/skillman` renders, resizes without breakage, `NO_COLOR=1` has no color.

## Phase 2 — Scan / parse / validate + preview ✅ (MVP)
- Goal: real skills listed from `~/.config/opencode/skills/`, markdown preview, validation badges.
- Files: `skills.go` (walk, frontmatter parse, stat size/mtime), preview via Glamour viewport.
- Validate: missing SKILL.md, missing `name/description`, name≠dirname → `[ok]/[warn]/[err]` badge + details in preview.
- Async: scan as BubbleTea Cmd, skeleton rows + spinner only if >200ms, `r` rescan, `Esc` cancels.
- Check: `go build` + run against real skills dir shows 5 skills with correct badges; empty dir shows empty-state.

## Phase 3 — Search / palette / help / toasts ✅ (MVP)
- Goal: discoverable, never dead-ends.
- Files: `filter.go` or in `update.go`, `palette.go`, `help.go`, `toast.go`, `status.go`.
- Keys: `/` fuzzy filter (preserve query on pane switch), `:` palette (`install, edit, delete, validate, reload, quit`), `?` help overlay, `Esc` pops one layer only.
- Toasts: success auto-dismiss 3s, error sticky + log path, slide 200ms ease-out, skippable, never steal focus.
- Check: filter narrows list, palette runs commands, `?` opens/closes, toasts appear/dismiss.

## Phase 4 — Open in editor + delete with undo ✅ (MVP)
- Goal: `e` edits, `d` deletes safely.
- Files: `editor.go`, `trash.go`.
- Edit: `$EDITOR <skill>/SKILL.md` (fallback `vi`), suspend alt-screen, restore on exit, toast on return, rescan.
- Delete: confirm modal → move to `~/.local/share/skillman/trash/<name>-<ts>/` → toast `Deleted X — u to undo (5s)` → `u` restores. No `rm -rf`. Log to `~/.local/share/skillman/skillman.log`.
- Check: edit round-trips, delete→undo restores files, log written.

## Phase 5 — Install from GitHub ⏸️ DEFERRED (post-MVP)
- Goal: `i` installs skills from a repo URL.
- Files: `install.go`.
- Flow: input modal (URL) → `git clone --depth 1` to safe temp (no shell injection, cancellable) → detect candidates: (a) root `SKILL.md`, (b) `skills/*/SKILL.md`, (c) any `*/SKILL.md` ≤2 deep → checklist modal → copy → rescan + toast. Errors inline + retry.
- Check: install from a test repo (e.g. one with `skills/*`) copies correctly; bad URL shows inline error, no freeze.

## Phase 6 — Hybrid CLI + config/state + polish ✅ (MVP)
- Goal: well-behaved CLI per philosophy §1.
- Files: `cli.go`, `config.go`.
- CLI: `skillman` (TUI if TTY else `list --plain`), `list [--json]`, `view <name> [--plain]`, `validate [--json]`, `install <url>`, `delete <name> [--yes]`, `--help`, shell completion, stable exit codes.
- Config: `$XDG_CONFIG_HOME/skillman/config.yaml` + env + flags (`--no-animations`, `--plain`); state `state.json` (last selection, pane ratio); `NO_COLOR` respected everywhere.
- Polish: `--help` + README keymap, 80×24 + `TERM=dumb` + piped tests, `go vet`, no flicker, CPU sane.
- Check: `skillman list --json | jq`, `skillman validate`, `skillman --help`, `NO_COLOR=1 skillman list`, `CI=true` disables animation.

---

## Out of scope (v2)
- Enable/disable toggle, skill creation wizard, `skill-creator` integration, mouse support, OSC 52 clipboard, update-from-remote.
