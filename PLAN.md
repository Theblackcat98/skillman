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

## Phase 1 — Skeleton TUI ✅ (MVP, commit `1efd7f3`)
- Built: header / list + preview panes / status + footer, quits cleanly via `q`/`Ctrl-C` with terminal restore.
- Files: `main.go`, `model.go`, `update.go`, `view.go`, `theme.go` (as planned).
- Done: 80×24 safe + narrow collapse (`Tab` toggles list/preview below 70 cols), `NO_COLOR` + `TERM=dumb` monochrome fallback, motion gate (`NO_ANIMATIONS`, `REDUCED_MOTION`, `CI`, non-TTY, `--no-animations`).
- Keys: `j/k,↑/↓,g/G,Home/End,PgUp/PgDn,Tab,Enter,q` (as planned).
- Check (passed): `go vet ./...`, `gofmt -l` clean, `go build -o tmp/skillman .`, `NO_COLOR=1` / `TERM=dumb` / piped output all plain.

## Phase 2 — Scan / parse / validate + preview ✅ (MVP, commit `1efd7f3`)
- Built: real skills from `~/.config/opencode/skills/` (or `SKILLMAN_SKILLS` / `--skills-dir`), Glamour markdown preview in viewport, `[ok]/[warn]/[err]` badges.
- Files: `skills.go` (readdir walk, YAML frontmatter parse, size/mtime stat) — as planned.
- Validate: missing SKILL.md, missing `name`/`description`, name≠dirname → issues listed in preview + `validate` command.
- Async: scan runs as BubbleTea Cmd with spinner + skeleton rows; `r` rescans. Note: local scan is ms-fast so no 200ms spinner delay and no mid-scan cancel — `Esc` clears filter / unfocuses preview instead.
- Check (passed): real skills dir shows 5 skills all `[ok]`; `view`, empty-dir empty-state; substring filter helper `FilterSkills` unit-covered via CLI.

## Phase 3 — Search / palette / help / toasts ✅ (MVP, commit `1efd7f3`)
- Built: `/` filter (query preserved on pane switch, `Esc` keeps query, second `Esc` clears), `:` command line (`edit delete validate reload clear quit help`, plus `filter <q>`), `?` help overlay, `Esc` pops exactly one layer.
- Files: folded into `update.go` / `view.go` / `model.go` — no separate `filter.go`, `palette.go`, `help.go`, `toast.go`, `status.go` (kept simple; split if they grow).
- Toasts: 3s auto-dismiss, errors sticky + log path, never steal focus. Note: no slide/fade animation — toasts render instantly in the status bar (simple + skippable by design); spinner (12fps) is the only animation.
- Check: filter narrows list (verified via `FilterSkills` + CLI); palette/help/toast paths compile and run — interactive overlay behavior still needs a live 80×24 keypress walkthrough.

## Phase 4 — Open in editor + delete with undo ✅ (MVP, commit `1efd7f3`)
- Built: `e` opens `SKILL.md` in `$EDITOR` (fallback `vi`); `d` confirm modal → move to `~/.local/share/skillman/trash/<name>-<ts>/` → toast + `u` restores within 30s. No `rm -rf`. Events logged to `~/.local/share/skillman/skillman.log`.
- Files: `editor.go`, `trash.go` (as planned).
- Known gap: editor runs synchronously without alt-screen suspend (`tea.ExecCommand` not wired) — editing from the TUI may render under the alt-screen on some terminals. Fix forward in a follow-up.
- Check (passed): CLI `delete` refuses without `--yes` (exit 3), moves to trash with `--yes`, manual restore verified in sandbox; TUI undo path compiles — live `d`→`u` walkthrough still to do.

## Phase 5 — Install from GitHub ⏸️ DEFERRED (post-MVP)
- Goal: `i` installs skills from a repo URL.
- Files: `install.go`.
- Flow: input modal (URL) → `git clone --depth 1` to safe temp (no shell injection, cancellable) → detect candidates: (a) root `SKILL.md`, (b) `skills/*/SKILL.md`, (c) any `*/SKILL.md` ≤2 deep → checklist modal → copy → rescan + toast. Errors inline + retry.
- Check: install from a test repo (e.g. one with `skills/*`) copies correctly; bad URL shows inline error, no freeze.

## Phase 6 — Hybrid CLI + config/state + polish ✅ (MVP, commit `1efd7f3`)
- Built: `skillman` launches TUI on TTY else plain `list`; `list [--json]`, `view <name> [--plain]`, `validate [--json]` (exit 0 ok / 1 issues / 2 missing SKILL.md), `delete <name> [--yes]`, `--help`, `--plain`, `--no-animations`, `--skills-dir` / `SKILLMAN_SKILLS`. `NO_COLOR`, `TERM=dumb` respected; motion gated by `NO_ANIMATIONS`/`REDUCED_MOTION`/`CI`.
- Files: `cli.go`, `config.go` (as planned).
- Not built (deferred): `config.yaml` file, `state.json` persistence (last selection/pane ratio), shell completion, `install <url>` (stub exits 3 pointing at Phase 5).
- Check (passed): `list --json`, `validate` (5 ok), `--help`, `NO_COLOR=1` / `TERM=dumb` / piped / non-TTY fallback all verified.

---

## Out of scope (v2)
- Phase 5 GitHub autoinstall (`install.go`, `i` key).
- Phase 6 leftovers: `config.yaml`, `state.json` persistence, shell completion.
- Live 80×24 interactive walkthrough (filter/palette/help/delete→undo keypresses) + editor alt-screen suspend fix.
- Enable/disable toggle, skill creation wizard, `skill-creator` integration, mouse support, OSC 52 clipboard, update-from-remote.
