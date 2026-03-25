#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-$repo_root/docs/assets/images/screenshots}"
app_bin="${APP_BIN:-$HOME/.local/bin/hyprmoncfg}"
terminal_bin="${TERMINAL_BIN:-alacritty}"
window_class="${WINDOW_CLASS:-hyprmoncfg-docshot}"
window_width="${WINDOW_WIDTH:-1500}"
window_height="${WINDOW_HEIGHT:-980}"
window_x="${WINDOW_X:-320}"
window_y="${WINDOW_Y:-120}"

mkdir -p "$output_dir"

if ! command -v "$terminal_bin" >/dev/null 2>&1; then
  echo "missing terminal emulator: $terminal_bin" >&2
  exit 1
fi
if ! command -v hyprctl >/dev/null 2>&1; then
  echo "missing hyprctl" >&2
  exit 1
fi
if ! command -v grim >/dev/null 2>&1; then
  echo "missing grim" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "missing jq" >&2
  exit 1
fi
if ! command -v wtype >/dev/null 2>&1; then
  echo "missing wtype" >&2
  exit 1
fi
if [[ ! -x "$app_bin" ]]; then
  echo "missing executable: $app_bin" >&2
  exit 1
fi

client_by_title() {
  local title="$1"
  hyprctl -j clients | jq -c --arg title "$title" '.[] | select(.title == $title)' | head -n1
}

wait_for_client() {
  local title="$1"
  local client=""
  for _ in $(seq 1 80); do
    client="$(client_by_title "$title")"
    if [[ -n "$client" ]]; then
      printf '%s\n' "$client"
      return 0
    fi
    sleep 0.15
  done
  return 1
}

focus_client() {
  local address="$1"
  hyprctl dispatch focuswindow "address:$address" >/dev/null
}

close_window() {
  local pid="$1"
  local address="${2:-}"
  if [[ -n "$address" ]]; then
    hyprctl dispatch closewindow "address:$address" >/dev/null 2>&1 || true
  fi
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" 2>/dev/null || true
}

capture_state() {
  local name="$1"
  local key_action="${2:-}"
  local title="hyprmoncfg-shot-$name"
  local screenshot="$output_dir/$name.png"

  env -u NO_COLOR COLORTERM=truecolor TERM=xterm-256color "$terminal_bin" \
    --title "$title" \
    --class "$window_class,$window_class" \
    -o "window.dimensions.columns=132" \
    -o "window.dimensions.lines=38" \
    -o "font.size=14" \
    -o "window.opacity=1" \
    -o "window.padding.x=12" \
    -o "window.padding.y=10" \
    -e bash -lc "cd '$repo_root' && '$app_bin'" >/dev/null 2>&1 &
  local term_pid=$!

  local client
  client="$(wait_for_client "$title")"
  local address
  address="$(printf '%s' "$client" | jq -r '.address')"

  hyprctl dispatch setfloating "address:$address" >/dev/null
  hyprctl dispatch resizewindowpixel "exact $window_width $window_height,address:$address" >/dev/null
  hyprctl dispatch movewindowpixel "exact $window_x $window_y,address:$address" >/dev/null

  sleep 0.9
  focus_client "$address"
  sleep 0.6

  if [[ -n "$key_action" ]]; then
    eval "$key_action"
    sleep 0.7
  fi

  client="$(hyprctl -j clients | jq -c --arg addr "$address" '.[] | select(.address == $addr)' | head -n1)"
  local x y w h
  x="$(printf '%s' "$client" | jq -r '.at[0]')"
  y="$(printf '%s' "$client" | jq -r '.at[1]')"
  w="$(printf '%s' "$client" | jq -r '.size[0]')"
  h="$(printf '%s' "$client" | jq -r '.size[1]')"

  grim -g "$x,$y ${w}x${h}" "$screenshot"
  close_window "$term_pid" "$address"
}

capture_state "layout"
capture_state "save-profile" "wtype -k s"

printf 'Captured screenshots in %s\n' "$output_dir"
