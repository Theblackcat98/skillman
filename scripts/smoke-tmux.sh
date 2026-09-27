#!/usr/bin/env bash
# Drives tmux sessions end-to-end. Run from the repo root after building:
#   go build -o tmp/skillman . && scripts/smoke-tmux.sh
#
# This is the only automated coverage the TUI has: the Go tests assert on
# rendered strings, but nothing else sends real keystrokes through a real
# terminal. The review noted this script claimed to drive delete and undo
# and did not (G6), so those are here now, along with Ctrl-C, a resize and
# a NO_COLOR run. It runs in CI, so it also cannot be skipped.
set -euo pipefail

BIN="${BIN:-./tmp/skillman}"
[ -x "$BIN" ] || { echo "build first: go build -o tmp/skillman ." >&2; exit 1; }
command -v tmux >/dev/null 2>&1 || { echo "tmux not installed" >&2; exit 1; }

# Deterministic fixtures, in throwaway directories. The trash and the
# config go to a temp XDG home too, so a smoke run never touches the
# developer's real skills, trash or state.
# Two temp roots, deliberately separate: putting the XDG dirs inside the
# skills dir would make skillman scan them and report two extra broken
# skills, which quietly invalidates every count assertion below.
FX="$(mktemp -d)"
XDGROOT="$(mktemp -d)"
export XDG_DATA_HOME="$XDGROOT/data"
export XDG_CONFIG_HOME="$XDGROOT/config"
mkdir -p "$XDG_DATA_HOME" "$XDG_CONFIG_HOME"

mk_skill() { # mk_skill <dir> <name> <description> [body]
  mkdir -p "$FX/$1"
  {
    printf -- '---\nname: %s\ndescription: %s\n---\n\n' "$2" "$3"
    printf -- '%s\n' "${4:-body}"
  } >"$FX/$1/SKILL.md"
}
mk_skill demo demo "smoke fixture"
mk_skill alpha alpha "the first skill"
mk_skill beta beta "the second skill"
# A CJK name, so the width arithmetic is exercised for real.
mk_skill 日本語-skill 日本語 "cjk name" "wide characters in the body"
# A long body, so scrolling has something to scroll.
mkdir -p "$FX/longbody"
{
  printf -- '---\nname: longbody\ndescription: a skill with a very long body\n---\n\n'
  for i in $(seq 1 120); do printf 'line %d of the long body\n' "$i"; done
} >"$FX/longbody/SKILL.md"
# A skill with a problem, so the badge has something to say.
mkdir -p "$FX/broken"
printf -- 'no frontmatter at all\n' >"$FX/broken/SKILL.md"
export SKILLMAN_SKILLS="$FX"

S=skillman-smoke
fail=0
pass() { echo "ok: $1"; }
bad() { echo "FAIL: $1"; fail=1; }

cleanup() { tmux kill-session -t "$S" 2>/dev/null || true; rm -rf "$FX" "$XDGROOT"; }
trap cleanup EXIT

start() { # start <suffix> <width> <height> <command...>
  local suffix="$1" w="$2" h="$3"
  shift 3
  tmux kill-session -t "$S" 2>/dev/null || true
  tmux new-session -d -s "$S" -x "$w" -y "$h" "$@"
  sleep 1
}

# assert <label> <grep pattern>
assert() {
  if tmux capture-pane -t "$S" -p | grep -q -- "$2"; then
    pass "$1"
  else
    bad "$1"
    tmux capture-pane -t "$S" -p | sed 's/^/    | /'
  fi
}

# refute <label> <grep pattern>
refute() {
  if tmux capture-pane -t "$S" -p | grep -q -- "$2"; then
    bad "$1"
    tmux capture-pane -t "$S" -p | sed 's/^/    | /'
  else
    pass "$1"
  fi
}

# Every line of the frame must fit the terminal, in cells. A frame that
# is one column too wide scrolls the whole screen, and no amount of
# substring matching notices.
assert_frame_fits() { # assert_frame_fits <label> <width>
  local label="$1" w="$2" bad_lines
  bad_lines="$(tmux capture-pane -t "$S" -p | awk -v w="$w" '
    { n = length($0); gsub(/\033\[[0-9;]*m/, "", $0); n = length($0); if (n > w) print NR": "n" cols" }' | head -3)"
  if [ -n "$bad_lines" ]; then
    bad "$label (lines too wide: $bad_lines)"
  else
    pass "$label"
  fi
}

tmux kill-session -t "$S" 2>/dev/null || true
tmux new-session -d -s "$S" -x 80 -y 24 "$BIN"
sleep 1

# --- open -----------------------------------------------------------------
assert "skills are listed" "demo"
assert "badge visible" "\[ok\]"
assert "badge for a broken skill" "\[warn\]"
assert "footer shows quit" "q quit"
assert "list title shows the position" "skills  1-6/6\|skills  1"
assert_frame_fits "frame fits 80 columns" 80

# --- move -----------------------------------------------------------------
tmux send-keys -t "$S" j; sleep 0.3
assert "j moves the selection" "> \(alpha\|beta\)"
tmux send-keys -t "$S" G; sleep 0.3
tmux send-keys -t "$S" g; sleep 0.3
pass "g and G do not crash"

# --- filter ---------------------------------------------------------------
tmux send-keys -t "$S" /; sleep 0.3
tmux send-keys -t "$S" "longbody"; sleep 0.4
assert "filter narrows the list" "1/6 skills"
tmux send-keys -t "$S" "zzz"; sleep 0.4
assert "no-match names the query" "no match for /"
assert "no-match offers the escape" "esc clears the filter"
tmux send-keys -t "$S" Escape; sleep 0.2
tmux send-keys -t "$S" Escape; sleep 0.3
assert "esc esc clears the filter" "6/6 skills"

# --- help -----------------------------------------------------------------
tmux send-keys -t "$S" "?"; sleep 0.4
assert "help overlay opens" "move selection"
assert "help lists the env gates" "NO_COLOR"
# The whole env-gate line must survive at the reference size: the old
# hardcoded 20-row box cut it (review E2).
assert "help env-gate line is not cut" "NO_ANIMATIONS, REDUCED_MOTION, CI"
tmux send-keys -t "$S" Escape; sleep 0.3

# --- confirm modal --------------------------------------------------------
tmux send-keys -t "$S" g; sleep 0.3
tmux send-keys -t "$S" "d"; sleep 0.4
assert "confirm modal opens" "confirm delete"
refute "no redundant close line" "\-\-\- \[Esc\] close"
tmux send-keys -t "$S" "n"; sleep 0.3
assert "n cancels the delete" "6/6 skills"

# --- delete then undo, the path the old script never drove ----------------
tmux send-keys -t "$S" "d"; sleep 0.3
tmux send-keys -t "$S" "y"; sleep 0.6
assert "y deletes the skill" "5/5 skills"
assert "the toast offers undo" "to undo"
tmux send-keys -t "$S" "u"; sleep 0.6
assert "u restores the skill" "6/6 skills"
assert_frame_fits "frame still fits after delete and undo" 80

# --- long body scrolls ---------------------------------------------------
tmux send-keys -t "$S" /; sleep 0.2
tmux send-keys -t "$S" "longbody"; sleep 0.3
tmux send-keys -t "$S" Enter; sleep 0.3
tmux send-keys -t "$S" Tab; sleep 0.3
tmux send-keys -t "$S" G; sleep 0.4
assert "preview scrolls to the last line" "line 1[12]0"
tmux send-keys -t "$S" Tab; sleep 0.3
tmux send-keys -t "$S" Escape; sleep 0.2
tmux send-keys -t "$S" Escape; sleep 0.3

# --- resize ---------------------------------------------------------------
tmux resize-window -t "$S" -x 50 -y 20 2>/dev/null || tmux resize-pane -t "$S" -x 50 -y 20
sleep 0.5
assert_frame_fits "frame fits after resize to 50x20" 50
tmux resize-window -t "$S" -x 80 -y 24 2>/dev/null || true
sleep 0.5
assert_frame_fits "frame fits after resize back to 80x24" 80

# --- CJK name -------------------------------------------------------------
tmux send-keys -t "$S" /; sleep 0.2
tmux send-keys -t "$S" "日本語"; sleep 0.4
assert "a CJK skill can be selected" "日本語"
assert_frame_fits "frame fits with a CJK name" 80
tmux send-keys -t "$S" Escape; sleep 0.2
tmux send-keys -t "$S" Escape; sleep 0.3

# --- clean quit -----------------------------------------------------------
tmux send-keys -t "$S" "q"; sleep 0.8
if tmux has-session -t "$S" 2>/dev/null; then
  bad "q did not quit"
  tmux kill-session -t "$S"
else
  pass "clean quit"
fi

# --- Ctrl-C quits from a modal, not just the base layer ------------------
start ctrlc 80 24 "$BIN"
tmux send-keys -t "$S" "?"; sleep 0.4
assert "help is open before ctrl-c" "move selection"
tmux send-keys -t "$S" C-c; sleep 0.8
if tmux has-session -t "$S" 2>/dev/null; then
  bad "ctrl-c did not quit from the help overlay"
  tmux kill-session -t "$S"
else
  pass "ctrl-c quits from the help overlay"
fi

start ctrlc2 80 24 "$BIN"
tmux send-keys -t "$S" "/"; sleep 0.3
tmux send-keys -t "$S" "demo"; sleep 0.3
tmux send-keys -t "$S" C-c; sleep 0.8
if tmux has-session -t "$S" 2>/dev/null; then
  bad "ctrl-c did not quit from the filter"
  tmux kill-session -t "$S"
else
  pass "ctrl-c quits from the filter"
fi

# --- NO_COLOR and TERM=dumb ---------------------------------------------
start nocolor 80 24 env NO_COLOR=1 TERM=xterm-256color "$BIN"
assert "NO_COLOR still renders" "demo"
refute "NO_COLOR emits no escapes" "$(printf '\033')"
assert_frame_fits "frame fits with NO_COLOR" 80
tmux send-keys -t "$S" "q"; sleep 0.6
tmux kill-session -t "$S" 2>/dev/null || true

start dumb 80 24 env TERM=dumb "$BIN"
assert "TERM=dumb still renders" "demo"
assert_frame_fits "frame fits with TERM=dumb" 80
tmux send-keys -t "$S" "q"; sleep 0.6
tmux kill-session -t "$S" 2>/dev/null || true

# --- a narrow terminal ---------------------------------------------------
start narrow 36 12 "$BIN"
assert "narrow terminal renders" "demo"
assert_frame_fits "frame fits at 36x12" 36
tmux send-keys -t "$S" Tab; sleep 0.4
assert "tab switches to the preview when narrow" "preview"
assert_frame_fits "preview fits at 36x12" 36
tmux send-keys -t "$S" "q"; sleep 0.6
tmux kill-session -t "$S" 2>/dev/null || true

# --- the CLI in the same environment -------------------------------------
if "$BIN" list --names | grep -q demo; then
  pass "cli list works in the smoke environment"
else
  bad "cli list failed in the smoke environment"
fi
# The fixture set deliberately contains one broken skill, so the
# documented contract is exit 1 for "validation issues", not 0.
if "$BIN" validate >/dev/null 2>&1; then
  bad "cli validate exited 0 with a broken skill present"
else
  pass "cli validate reports issues"
fi
# And with only good skills it must be 0.
GOOD="$FX-good"
mkdir -p "$GOOD/good-one"
printf -- '---\nname: good-one\ndescription: fine\n---\n\nbody\n' >"$GOOD/good-one/SKILL.md"
if SKILLMAN_SKILLS="$GOOD" "$BIN" validate >/dev/null 2>&1; then
  pass "cli validate exits 0 when every skill is fine"
else
  bad "cli validate exited non-zero with only good skills"
fi
rm -rf "$GOOD"

# --- install, against a throwaway local repository -------------------------
# This builds a git repository in a temp directory and installs into a
# second temp directory. It never touches the real skills directory and
# never reaches the network, so the dry run and the real path are both
# exercised for real rather than mocked.
command -v git >/dev/null 2>&1 && {
  REPO="$FX-repo"
  mkdir -p "$REPO/skills/installed-one" "$REPO/skills/installed-two"
  printf -- '---\nname: installed-one\ndescription: from the fixture repo\n---\n\nbody one\n' \
    >"$REPO/skills/installed-one/SKILL.md"
  printf -- '---\nname: installed-two\ndescription: also from the repo\n---\n\nbody two\n' \
    >"$REPO/skills/installed-two/SKILL.md"
  printf 'not a skill\n' >"$REPO/README.md"
  git -C "$REPO" init -q 2>/dev/null &&
    git -C "$REPO" -c user.email=t@e -c user.name=t add -A 2>/dev/null &&
    git -C "$REPO" -c user.email=t@e -c user.name=t commit -qm init 2>/dev/null || true

  if [ -d "$REPO/.git" ]; then
    DEST="$FX-install"
    mkdir -p "$DEST"

    # A dry run must report and write nothing.
    if SKILLMAN_SKILLS="$DEST" "$BIN" install "file://$REPO" --dry-run 2>&1 |
      grep -q "would install"; then
      pass "install --dry-run reports what it would do"
    else
      bad "install --dry-run did not report"
    fi
    if [ -z "$(ls -A "$DEST" 2>/dev/null)" ]; then
      pass "install --dry-run wrote nothing"
    else
      bad "install --dry-run wrote into the skills directory"
    fi

    # A remote helper is remote code execution and must be refused.
    if SKILLMAN_SKILLS="$DEST" "$BIN" install 'ext::sh -c id' >/dev/null 2>&1; then
      bad "install accepted a git remote helper"
    else
      pass "install refuses a git remote helper"
    fi

    # The real thing.
    if SKILLMAN_SKILLS="$DEST" "$BIN" install "file://$REPO" >/dev/null 2>&1; then
      pass "install copies from a repository"
    else
      bad "install failed against a local repository"
    fi
    if [ -f "$DEST/installed-one/SKILL.md" ] && [ -f "$DEST/installed-two/SKILL.md" ]; then
      pass "the installed skills are on disk"
    else
      bad "the installed skills are missing"
    fi
    if SKILLMAN_SKILLS="$DEST" "$BIN" validate >/dev/null 2>&1; then
      pass "the installed skills validate"
    else
      bad "the installed skills do not validate"
    fi

    # Installing again must not clobber what is already there.
    if SKILLMAN_SKILLS="$DEST" "$BIN" install "file://$REPO" >/dev/null 2>&1; then
      bad "install overwrote existing skills"
    else
      pass "install refuses to clobber an existing skill"
    fi
  else
    echo "skip: git could not make a fixture repository here"
  fi
  rm -rf "$REPO" "$FX-install"
} || echo "skip: git is not installed"

exit "$fail"
