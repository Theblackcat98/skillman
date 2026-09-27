#!/usr/bin/env bash
# Drives one tmux session end-to-end: open, move, filter, help,
# confirm modal, quit. Run from the repo root after building:
#   go build -o tmp/skillman . && scripts/smoke-tmux.sh
set -euo pipefail

BIN="${BIN:-./tmp/skillman}"
[ -x "$BIN" ] || { echo "build first: go build -o tmp/skillman ." >&2; exit 1; }

# Deterministic fixture so badge assertions always have data.
FX="$(mktemp -d)"
trap 'rm -rf "$FX"' EXIT
mkdir -p "$FX/demo"
printf -- '---\nname: demo\ndescription: smoke fixture\n---\n\nbody\n' > "$FX/demo/SKILL.md"
export SKILLMAN_SKILLS="$FX"

S=skillman-smoke
tmux kill-session -t "$S" 2>/dev/null || true
tmux new-session -d -s "$S" -x 80 -y 24 "$BIN"
sleep 1

fail=0
assert() { # assert "label" "grep pattern"
  if tmux capture-pane -t "$S" -p | grep -q "$2"; then
    echo "ok: $1"
  else
    echo "FAIL: $1"
    fail=1
  fi
}

assert "badge visible" "\[ok\]"
assert "footer shows quit" "q quit"

tmux send-keys -t "$S" j; sleep 0.3
tmux send-keys -t "$S" /; sleep 0.2
tmux send-keys -t "$S" "zzz"; sleep 0.3
assert "no-match names query" "no match for /zzz"

tmux send-keys -t "$S" Escape; sleep 0.2
tmux send-keys -t "$S" Escape; sleep 0.2
tmux send-keys -t "$S" "?"; sleep 0.3
assert "help overlay opens" "move selection"

tmux send-keys -t "$S" Escape; sleep 0.2
tmux send-keys -t "$S" "d"; sleep 0.3
assert "confirm modal opens" "confirm delete"
tmux send-keys -t "$S" "n"; sleep 0.2

tmux send-keys -t "$S" "q"; sleep 0.6
if tmux has-session -t "$S" 2>/dev/null; then
  echo "FAIL: q did not quit"
  tmux kill-session -t "$S"
  fail=1
else
  echo "ok: clean quit"
fi

exit "$fail"
