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

## Audit — 2026-09-27 (verified: `go vet`/`build`/`gofmt` clean, CLI runs, tmux 80×24 walkthrough)

Working as intended (live-verified):
- CLI: `list [--json]`, `view`, `validate` (exit 0/1/2), `delete` (`--yes` gate → exit 3), `--help`, non-TTY and `TERM=dumb` → plain `list` fallback.
- TUI: render at 80×24, j/k/g/G/PgUp/PgDn, `/` filter, `:` commands, `?` help, Esc layer stacking, live resize 80↔50, narrow `Tab` toggle, `NO_COLOR` monochrome, clean quit with alt-screen restore, delete→undo (4/4 → 3/3 → 4/4 with cursor restored), async scan + spinner, preview cache (no j/k lag).

Bugs found (ID · severity · evidence):
- B1 · high · At 70–97 cols (incl. 80×24) the `[ok]/[warn]/[err]` badge is truncated out of every row: `renderRow` needs 44 cols but gets `width/2-5` (35 at 80). Status is then conveyed by color only — violates "no color alone". Badge is visible again only <70 or ≥98 cols (verified frames).
- B2 · high · Off-by-one: `renderListBox` emits `h-2` rows but `box` renders `h-3` → selected row invisible at the bottom edge (verified: `G` with 20 skills → status/preview show `skill-20` selected, list shows no `>` row).
- B3 · high · Preview focus is inert: j/k/↑↓/PgUp/PgDn are intercepted by the list before the viewport passthrough, so Tab+focus never scrolls the preview; `d`/`u` still fire delete/undo while preview is "focused" (verified: Tab + j j moved the list cursor).
- B4 · high · Ctrl-C swallowed in filter/command/help/confirm modes — only quits from normal mode (verified live in filter + confirm). Violates the clean-exit baseline.
- B5 · med · `setToast` skips the clear-timer when `noAnim && !isErr` → success toasts never dismiss under `NO_ANIMATIONS`/`CI`/`TERM=dumb` (verified: stuck at t+4.5s; animated control clears at 3s).
- B6 · med · `toastClearMsg`/`undoExpireMsg` carry no generation token — an older timer clears a newer toast, or a fresh 30s undo window, early (code path).
- B7 · med · Active query with 0 matches shows "(no skills — r to rescan)" — wrong advice; empty-dir and no-match states must differ, with `Esc` hint (verified).
- B8 · med · Bare `skillman --json` on a TTY launches the TUI, silently ignoring `--json` (verified).
- B9 · med · Footer truncates at 80 cols before `? help · q quit` — the key hints are invisible at the reference size (verified).
- B10 · high · Editor: no alt-screen suspend (runs inside a goroutine Cmd; output interleaves with the alt-screen), `$EDITOR` with args (`code -w`) fails because the whole string is exec'd, post-edit rescan result is discarded (`_ = skills`) (Phase 4 known gap).
- B11 · low · Overlays blank the background entirely (`overlay` ignores `under`) instead of dimming it; the confirm modal always appends `--- [Esc] close ---` beneath `y confirm · n cancel` (verified frames).
- B12 · low · Invalid YAML frontmatter is swallowed (`_ = yaml.Unmarshal`) → misreported as "missing name/description" instead of a parse error (code path).
- B13 · med · Delete/undo use `os.Rename` only → fails across filesystems (skills dir on different FS than data dir, plausible on Termux/storage) and collides when two deletes share the same second (timestamp-only destination) (code path).
- B14 · low · Undo collision renames the skill to `<name>-restored` → dirname≠frontmatter warning on next scan, and cursor restore misses (code path).
- B15 · low · Status shows "ready" while a query is active; filter line double-prompts (`filter: /warn`); `d` with no selection is a silent no-op while `e` toasts (verified).
- B16 · low · Styled header branch is not width-truncated (`gap=1` overflow wraps at tiny widths); view renders `height-3` lines — 3 rows wasted while B1/B9 clip content (code path).

Hygiene gaps:
- Zero tests (no `*_test.go`; Phase 2's "unit-covered via CLI" claim is inaccurate), no README, no man page, no CI.
- Dead/vestigial code: `sizePanes` computes `w` and discards it; `toastAt` written but never read; `var _ = viewport.Model{}` import hack; layout constants duplicated between `sizePanes`/`renderMain` (drift caused B2).
- lipgloss pinned to a pre-release pseudo-version; magic layout numbers (22/12 col fields, `h-6`, `w-4`) scattered across files.
- Trash grows unbounded (no `trash` CLI); `config.yaml`, `state.json`, shell completion still missing (baseline items from `tui-philosophy.md`).

---

## Phase 7 — Core correctness fixes (B1–B4, B7–B9, B15)
- Goal: every key works in every mode; all info visible at 80×24.
- Files: `view.go`, `update.go`, `model.go`, `main.go`.
- Fixes: priority row layout (name + badge always fit; category drops when narrow — kills B1); align list rows with `box` content lines (B2); route nav keys to the viewport while preview is focused and stop firing list-only keys there (B3); global Ctrl-C check before mode dispatch (B4); distinct empty-dir vs no-match states with `Esc` hint (B7); bare `--json` ⇒ `list --json` on TTY (B8); compact context-aware footer so `? help · q quit` fits 80 cols (B9); status/query copy fixes (B15).
- Check: tmux 80×24 — badge visible on every row; `G` shows `>` on the last row; Tab+j scrolls preview only; Ctrl-C quits from filter, command, help, and confirm; `/nomatch` shows match-hint state; `--json` prints JSON on a TTY; footer ends with `q quit`.

## Phase 8 — Feedback + lifecycle hardening (B5, B6, B10, B12–B14)
- Goal: timers are reliable; editor, YAML, and trash flows can't corrupt or lie.
- Files: `model.go`, `update.go`, `editor.go`, `trash.go`, `skills.go`.
- Fixes: always schedule toast dismissal (drop the `noAnim` early-return) and add generation tokens to toast/undo timers (B5/B6); editor via `tea.ExecProcess` with alt-screen suspend + `$EDITOR` arg splitting + rescan after edit using the result (B10); surface YAML parse errors as an issue (B12); trash: unique dest name incl. pid, copy+remove fallback for cross-device moves (B13); undo restores to original name when free instead of `<name>-restored` (B14).
- Check: `NO_ANIMATIONS=1` delete toast clears at 3s; rapid double-delete both land in trash; `EDITOR='true'` edit returns to a intact TUI frame; corrupted frontmatter reports `invalid YAML`; unit tests from Phase 9 cover parse/trash naming.

## Phase 9 — Test suite + CI
- Goal: lock the fixes in; make regressions like B1/B2 impossible.
- Files: `skills_test.go`, `view_test.go`, `trash_test.go`, `cli_test.go`, optional `scripts/smoke-tmux.sh`.
- Tests: frontmatter parse (valid/invalid/missing/CRLF), `FilterSkills`, badge/validate exit codes (0/1/2), trash naming + undo collision, golden render assertions at 80×24 (row width ≤ pane, badge present, cursor row visible at bottom, footer ≤ 80 cols, no ESC when plain).
- Check: `go test ./...` green; `go vet` clean; smoke script drives one tmux session (open, filter, delete, undo, quit).

## Phase 10 — UI/UX polish (B11, B16 + preview/status depth)
- Goal: overlays and previews feel finished.
- Files: `view.go`, `model.go`.
- Changes: overlay composites over a dimmed background and drops the redundant `--- [Esc] close ---` on confirm (B11); use the spare 3 rows (or 2-line footer) instead of clipping (B16); list title shows scroll position (`5–19/40`); preview gains an issues banner + frontmatter metadata line (license/compat/category); keep preview scroll offset across resize; document `H` in help; filter prompt de-duplication.
- Check: tmux frames at 80×24, 50×20, `NO_COLOR=1`, `TERM=dumb`; long SKILL.md scrolls to its last line; resize preserves scroll.

## Phase 11 — Config + state + trash CLI + completion
- Goal: close the `tui-philosophy.md` baseline gaps.
- Files: `config.go`, `state.go`, `cli.go`, `main.go`.
- Built: `config.yaml` in `$XDG_CONFIG_HOME/skillman/` (skills dir, theme/accents, keybinding overrides); `state.json` (last selection, scroll, pane focus, width breakpoint); `skillman trash list|restore <name>|purge` (+ `--json`); bash/zsh completion generation (`skillman completion bash|zsh`).
- Check: config respected on launch and in help; state restored across restarts; `trash list --json` machine-readable; completion script sources cleanly.

## Phase 12 — Documentation consolidation (single source of truth)
- Goal: every fact is defined in exactly one document. One canonical `README.md`; every other doc becomes it or a thin pointer.
- Files: create `README.md`; rewrite `AGENTS.md` (thin); delete `PLAN.md` and `tui-philosophy.md` after absorbing them. One commit keeps pointers valid.
- Absorb into README sections: overview + architecture/file map; status, audit findings, and roadmap (phases 0–13 with ✅/⏸️, moved from `PLAN.md`); the design rules that govern this project (baseline + layer/UX/animation rules distilled from `tui-philosophy.md`); usage — CLI, the ONE canonical TUI key table, config, trash/undo, env gates, exit codes; workflow summary.
- `AGENTS.md` keeps only what agents must not lose: the IMPLEMENT→CHECK→COMMIT→NEXT loop, Termux notes, and pointers to README sections (keymap, design rules, roadmap, file map). Its "mark phase done in PLAN.md" rule becomes "mark done in the README roadmap". No duplicated facts.
- Dedup enforcement: `printHelp` (cli.go) and the `?` overlay (view.go) reworded to match the canonical key table; keys/env vars/exit codes documented in one place only.
- Check: grep confirms single definitions and that no stale references to `PLAN.md`/`tui-philosophy.md` remain (including AGENTS.md instructions and code comments); `go vet` + `go build` pass after help-text edits; README quickstart followed verbatim works; roadmap reflects the current phase.

## Phase 13 — Man page + doc parity
- Goal: `skillman.1` exists and cannot silently diverge from the source of truth.
- Files: `docs/skillman.1`, parity check in `cli_test.go` (or `scripts/check-docs.sh`).
- Built: man page (synopsis, flags, commands, exit codes, keys) derived from README sections; parity check asserting `--help` output, the README key/flag table, and the man page agree.
- Check: `man ./docs/skillman.1` renders; parity check green under `go test ./...`.

---

## Out of scope (v2)
- Phase 5 GitHub autoinstall (`install.go`, `i` key) — stays deferred; should reuse Phase 7/8 async + cancel patterns first.
- Mouse support, OSC 52 clipboard, multi-select + sort UI, enable/disable toggle, skill creation wizard, `skill-creator` integration, update-from-remote.
- Done since the last update: live 80×24 walkthrough (filter/help/delete→undo/resize/NO_COLOR) — findings recorded in the Audit above.
