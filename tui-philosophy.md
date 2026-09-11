# The Respectable TUI CLI App

A reference for interactive terminal UIs that are also well-behaved command-line programs.

Assuming “TUI CLI app” means an interactive terminal UI that is also a well-behaved CLI program, the baseline is: **keyboard-first, discoverable, responsive, configurable, scriptable, and polite to the terminal.**

---

## 1. Core Features and Baseline

### Non-negotiables

- **Keyboard-only operation**  
  Everything must be reachable without a mouse: arrows/`hjkl`, `Tab`/`Shift-Tab`, `Enter`, `Esc`, `/` search, `?` help, `q` quit.

- **Discoverable help**  
  A `?` overlay, context-sensitive footer hints, `--help`, and ideally a man page. Users should not have to guess keybindings.

- **Search/filter/jump**  
  Incremental search, fuzzy filter, or jump-to-item. For anything list-based, `/` or `Ctrl-F` is table stakes.

- **Clear status and feedback**  
  Status bar, loading indicators, progress for long tasks, error messages that don’t destroy the screen, and a visible indication of current mode/context.

- **Resize-safe layout**  
  Works at 80×24 and degrades gracefully below that. No fixed dimensions, no overflow, no broken borders.

- **Clean terminal lifecycle**  
  Restores cursor, screen, and raw mode on normal exit, `Ctrl-C`, signals, and crashes. Alternate screen should not leak.

- **Configurability**  
  Config file in `$XDG_CONFIG_HOME`, env vars, CLI flags, keybinding overrides, theme/color settings, and persisted state like last selection or layout.

- **Respect for terminal capabilities**  
  Honors `NO_COLOR`, `TERM`, 256/truecolor, monochrome fallback. Never relies on color alone to convey meaning.

- **Async / non-blocking UI**  
  The interface must never freeze while doing I/O, network calls, or heavy computation. Long operations should be cancellable with `Esc` or `Ctrl-C`.

- **Fast and efficient rendering**  
  Quick startup, low memory, diff-based redraw, no flicker, handles large datasets without choking.

- **Proper exit semantics**  
  `q` and `Ctrl-C` exit predictably; exit codes are meaningful; destructive actions ask for confirmation or offer undo.

- **Non-interactive mode**  
  If it’s also a CLI: flags for automation, machine-readable output (JSON/YAML), stdin/stdout support, and stable exit codes.

### Expected in a polished app

- **Command palette / `:` actions** for discoverability and power users.
- **Vim-like motions** or at least consistent navigation conventions.
- **Mouse support that can be disabled** — optional, never required.
- **Clipboard integration** via OSC 52 or system tools, plus copy/paste of selections.
- **Unicode and wide-character handling** — emoji, CJK, combining marks, box drawing.
- **Accessibility considerations** — high contrast, no color-only cues, screen-reader-friendly where possible.
- **Integration with the environment** — `$EDITOR`, `$PAGER`, shell completion, OSC 8 hyperlinks, desktop notifications.
- **Error logging** — errors visible in UI, details in a log file, never corrupting the display.
- **Security hygiene** — safe temp files, no shell injection, respects file permissions.
- **Documentation and examples** — README, man page, `--help`, keymap reference.

### The minimum respectable baseline

If it can’t do:

- `q` to quit
- `?` for help
- `/` to search
- resize without breaking
- `NO_COLOR`
- clean terminal restore
- async operations
- a config file
- `--help`
- a non-interactive mode

...then it’s a toy — not a respectable TUI CLI app.

---

## 2. UI Structure, Layers, and User Flow

The next level is about **making the TUI feel like a navigable place**, not a pile of keybindings. That means a clear UI hierarchy, a real layer stack, and flows that never dead-end.

### UI structure and components

- **Responsive pane model**  
  Splits, tabs, sidebars, detail panes, and preview panes with min sizes, resizing, and collapse. Works at 80×24 and scales up gracefully.

- **Strong visual hierarchy**  
  Titles, separators, padding, dimmed inactive panes, highlighted active pane, and a consistent focus indicator. Users should always know where input will go.

- **Reusable TUI components**  
  Lists, tables, trees, forms, dialogs, command palette, autocomplete, progress bars, status bars, and toasts. Consistent behavior across every screen.

- **Tables that behave like tables**  
  Sortable columns, sticky headers, row selection, multi-select, column resize, horizontal scroll, and a details view for long fields.

- **Forms that don’t fight you**  
  Tab order, inline validation, defaults, required markers, cancel/confirm, and no lost input on accidental `Esc`.

- **Scrolling and selection**  
  Page up/down, home/end, smooth scroll indicators, sticky headers, range select, visual mode, and copy support.

- **Theming as tokens**  
  Semantic colors like `error`, `warning`, `accent`, `dim`, not hardcoded ANSI codes. Respects `NO_COLOR`, 16/256/truecolor, and monochrome fallback.

- **Accessibility in the UI**  
  High contrast, no color-only meaning, visible focus, screen-reader-friendly labels where possible, and reduced reliance on animation.

### Layer model

A respectable TUI has an explicit **layer stack**, not just “screens.”

- **Base layer**  
  The main view: list, table, dashboard, editor, etc.

- **Overlay layer**  
  Non-modal or lightly modal: command palette, search, autocomplete, context menu, help, quick switcher. Usually doesn’t steal focus unless it needs input.

- **Modal layer**  
  Dialogs, confirmations, wizards, forms. Traps focus, dims the background, and requires an explicit accept/cancel.

- **Notification layer**  
  Toasts, transient errors, background progress. Never blocks, stacks sensibly, and auto-dismisses unless it’s an error.

- **Status/input layer**  
  Status bar, footer hints, command line, prompt. Always visible or easily summonable.

- **Debug layer**  
  Logs, metrics, event trace. Hidden by default, toggleable, never corrupts the main view.

### Rules that make layers feel right

- `Esc` pops exactly one layer. It should not quit the app while a dialog is open.
- `q` only quits when the stack is at the base layer.
- Modals restore focus to the element that opened them.
- Covered layers preserve their state: scroll position, selection, filter query.
- Global keybindings vs. layer-local keybindings are clearly separated.
- Nested modals are rare and discouraged; prefer a wizard or inline flow.
- Toasts never steal focus. Errors do, but only if action is required.

### User flow and navigation

- **First-run and empty states**  
  Don’t show a blank screen. Offer sample data, a quick tutorial, or clear next actions.

- **Predictable back/cancel**  
  `Esc` backs out one step. `q` quits from base. `Ctrl-C` cancels the current operation before exiting.

- **Navigation history**  
  Back/forward, breadcrumbs, jump lists, and a command palette so users can move without memorizing paths.

- **Search that respects context**  
  `/` searches the current view; `Ctrl-P` or `:` searches globally. Preserve the query when switching panes.

- **Multi-step workflows**  
  Wizards, progress indicators, draft saving, and the ability to cancel without losing everything.

- **Action safety**  
  Destructive actions require confirmation or offer undo. Prefer undo over confirmation when possible.

- **Error recovery**  
  Errors are inline, actionable, and retryable. Details go to a log or expandable panel, not a full-screen crash.

- **Session persistence**  
  Restore last view, scroll position, selection, layout, and open tabs on restart.

- **Interruptibility**  
  Long operations show progress and can be cancelled. The UI never freezes.

- **Deep linking**  
  CLI args can open a specific view, filter, or item: `app --view logs --filter error`.

- **Consistent keymap**  
  Same action = same key everywhere. `?` always opens help. `:` or `Ctrl-P` always opens the command palette.

If you get the layer stack and user flow right, the app feels like a tool you can explore. If you get them wrong, even a beautiful TUI becomes a memorization test.

---

## 3. UX and Animations

At this stage, the question shifts from “does it work?” to “does it feel right?” In a TUI, UX and animation are not decoration — they’re how you communicate state, continuity, and confidence over a text protocol.

### UX: the feel of the flow

A respectable TUI should feel like a place with predictable physics, not a keyboard maze.

- **State is always visible**  
  Loading, empty, error, partial, offline, stale, dirty, saved, read-only. The user should never wonder what just happened.

- **Every action gets immediate feedback**  
  Under ~100ms: visual acknowledgement. Over ~1s: spinner or progress. Over ~10s: progress + cancel + ETA.

- **Latency is managed, not hidden**  
  Optimistic updates where safe, skeletons for unknown data, background tasks with toasts, and a clear “working…” state.

- **Modes are explicit**  
  If you have insert/normal/visual/filter modes, show them in the status bar. Never let a mode silently eat keys.

- **Focus is never ambiguous**  
  Active pane border, cursor, highlighted row, dimmed inactive regions. One glance tells you where input goes.

- **Errors are recoverable**  
  Inline, plain-language, actionable. Offer retry, edit, skip, or view details. Never dump a stack trace over the UI.

- **Undo beats confirmation**  
  Destructive actions should be undoable. Confirmation dialogs are a last resort, not a default.

- **Progressive disclosure**  
  Simple defaults first. Advanced actions live in `?`, `:`, command palette, config, or a details pane.

- **Flow never dead-ends**  
  Empty states suggest next actions. Wizards allow cancel and resume. `Esc` backs out one layer. `q` quits only from base.

- **Consistency is a feature**  
  Same action = same key everywhere. `/` searches current view. `?` always opens help. `:` or `Ctrl-P` always opens the palette.

- **Terminal realities are respected**  
  80×24, SSH, tmux, scrollback, copy/paste, no hover, variable refresh, screen readers, non-TTY, CI. The app should degrade, not break.

### Animations: motion with meaning

In a terminal, every frame is a diff over a pty. Animation is expensive and easy to get wrong. Use it only when it communicates something.

#### What to animate

- **Activity**: spinners, pulsing status dots, progress bars.
- **Progress**: determinate first, indeterminate second, ETA when possible.
- **Continuity**: dimming the background when a modal opens, highlighting the focused pane.
- **Attention**: a subtle pulse on a new error or notification.
- **Transitions**: layer enter/exit, toast appear/dismiss, selection movement.
- **Loading**: skeleton rows, shimmering placeholders, “fetching…” lines.

#### Rules that keep it respectable

- **Never block input.** Animation must not delay typing, scrolling, or cancellation.
- **Always skippable.** Any keypress should cancel or fast-forward an animation.
- **Short and purposeful.**
  - Micro: 80–150ms (focus, highlight)
  - Layer: 150–300ms (modal, toast)
  - Large: 300–500ms max (view change)
- **Easing matters.**  
  Ease-out for entering, ease-in for exiting, linear for progress. Avoid bounce/elastic in a TUI.
- **Frame rate is a budget, not a goal.**  
  10–20 fps is plenty for spinners. 30–60 fps only for cursor or smooth scroll. Measure CPU.
- **Render efficiently.**  
  Diff-based rendering, dirty regions, double buffering, coalesced redraws. Use synchronized output (`\e[?2026h` / `\e[?2026l`) where supported.
- **Respect the environment.**  
  Disable animation when non-TTY, `TERM=dumb`, `CI=true`, `NO_ANIMATIONS`, `REDUCE_MOTION`, `--no-animations`, or screen reader mode. Provide a config toggle.
- **No seizure triggers.**  
  No flashing, no rapid strobing, no full-screen wipes.
- **Spinners only after a delay.**  
  Don’t flash a spinner for a 50ms operation. Show it after 100–200ms.
- **Progress must not lie.**  
  If you can’t estimate, use indeterminate. If you can, show real progress and allow cancel.
- **Toasts don’t steal focus.**  
  Errors may demand attention; success notifications should not.
- **Modals dim, not freeze.**  
  Background should remain visible but inactive. Focus returns to the opener on close.
- **Typewriter effects are almost always wrong.**  
  Never use them for logs, code, or data. They slow reading and break copy/paste.

#### Anti-patterns

- Animation that delays the user.
- Full-screen wipes or fades that hide content.
- Spinners for instant operations.
- Progress bars that jump backward or lie.
- Motion in non-interactive or piped output.
- Cursor blink or motion that ignores reduced-motion settings.
- Layer transitions that reset scroll, selection, or filter state.
- Toasts that stack infinitely or never dismiss.
- `Esc` quitting the app while a dialog is open.

### The UX + animation checklist

- Immediate feedback for every action.
- Visible state: loading, empty, error, success, dirty, saved.
- One-layer `Esc`, base-layer `q`.
- Focus always obvious.
- Errors inline, actionable, recoverable.
- Undo for destructive actions.
- Command palette for discoverability.
- Consistent keymap across screens.
- Animations interruptible and skippable.
- Reduced-motion and no-animation support.
- Non-TTY and `TERM=dumb` fallbacks.
- Diff rendering, no flicker, low CPU.
- Persisted layout, selection, and history.
- Tested at 80×24, 120×40, SSH, tmux, monochrome.

If you get this right, the app stops feeling like a script and starts feeling like a tool you can live in.
