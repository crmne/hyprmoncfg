#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-$repo_root/docs/assets/images/screenshots}"
app_bin="${APP_BIN:-hyprmoncfg}"
terminal_bin="${TERMINAL_BIN:-ghostty}"
window_class="${WINDOW_CLASS:-TUI.float}"
font_size="${FONT_SIZE:-12}"
light_theme="${LIGHT_THEME:-ruby-llm}"
dark_theme="${DARK_THEME:-ruby-llm-dark}"
omarchy_path="${OMARCHY_PATH:-/usr/share/omarchy}"

mkdir -p "$output_dir"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/hyprmoncfg-screenshots.XXXXXX")"

cleanup() {
  find "$temp_dir" -maxdepth 1 -type f -delete
  rmdir "$temp_dir" 2>/dev/null || true
}

trap cleanup EXIT INT TERM

for cmd in "$app_bin" "$terminal_bin" hyprctl grim jq wtype omarchy-theme-color; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "missing: $cmd" >&2
    exit 1
  fi
done

render_ghostty_theme() {
  local colors_file="$1"
  local output_file="$2"
  local template="$omarchy_path/default/themed/ghostty.conf.tpl"

  if [[ ! -f "$template" ]]; then
    printf 'Ghostty theme template not found: %s\n' "$template" >&2
    return 1
  fi

  awk -F '\t' '
    NR == FNR { colors[$1] = $2; next }
    {
      line = $0
      while (match(line, /\{\{[[:space:]]*[A-Za-z0-9_]+[[:space:]]*\}\}/)) {
        token = substr(line, RSTART, RLENGTH)
        key = token
        gsub(/[{}[:space:]]/, "", key)
        line = substr(line, 1, RSTART - 1) colors[key] substr(line, RSTART + RLENGTH)
      }
      print line
    }
  ' <(omarchy-theme-color --file "$colors_file" --all) "$template" >"$output_file"
}

theme_file() {
  local theme="$1"
  local candidate
  for candidate in \
    "$HOME/.config/omarchy/themes/$theme/ghostty.conf" \
    "/usr/share/omarchy/themes/$theme/ghostty.conf"; do
    if [[ -f "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done

  for candidate in \
    "$HOME/.config/omarchy/themes/$theme/colors.toml" \
    "$omarchy_path/themes/$theme/colors.toml"; do
    if [[ -f "$candidate" ]]; then
      local rendered="$temp_dir/$theme-ghostty.conf"
      if [[ ! -f "$rendered" ]]; then
        render_ghostty_theme "$candidate" "$rendered"
      fi
      printf '%s\n' "$rendered"
      return 0
    fi
  done

  printf 'Ghostty theme or colors.toml not found: %s\n' "$theme" >&2
  return 1
}

wait_for_client() {
  local title="$1"
  for _ in $(seq 1 80); do
    local client
    client="$(hyprctl -j clients | jq -c --arg title "$title" '.[] | select(.title == $title)' | head -n1)"
    if [[ -n "$client" ]]; then
      printf '%s\n' "$client"
      return 0
    fi
    sleep 0.15
  done
  return 1
}

wait_for_client_gone() {
  local address="$1"
  local attempts="${2:-20}"
  for _ in $(seq 1 "$attempts"); do
    if [[ -z "$(hyprctl -j clients | jq -c --arg address "$address" '.[] | select(.address == $address)')" ]]; then
      return 0
    fi
    sleep 0.15
  done
  return 1
}

# Ask the window to close, then make sure it did. Closing politely depends on
# the terminal: ghostty's confirm-close-surface defaults to on, and a terminal
# asking "really close?" would leave a window per shot behind. Hyprland knows
# which process owns the window, so fall back to ending that.
close_client() {
  local address="$1"

  hyprctl dispatch closewindow "address:$address" >/dev/null 2>&1 || true
  wait_for_client_gone "$address" && return 0

  local pid
  pid="$(hyprctl -j clients | jq -r --arg address "$address" '.[] | select(.address == $address) | .pid')"
  if [[ -n $pid && $pid != "null" ]]; then
    kill "$pid" 2>/dev/null || true
    wait_for_client_gone "$address" && return 0
    kill -9 "$pid" 2>/dev/null || true
  fi

  wait_for_client_gone "$address" 40
}

focused_monitor() {
  hyprctl -j monitors | jq -c '((map(select(.focused)) | .[0]) // .[0])'
}

fit_and_center_window() {
  local address="$1"
  local monitor
  monitor="$(focused_monitor)"

  local monitor_x monitor_y monitor_w monitor_h monitor_scale
  monitor_x="$(printf '%s' "$monitor" | jq -r '.x')"
  monitor_y="$(printf '%s' "$monitor" | jq -r '.y')"
  monitor_w="$(printf '%s' "$monitor" | jq -r '.width')"
  monitor_h="$(printf '%s' "$monitor" | jq -r '.height')"
  monitor_scale="$(printf '%s' "$monitor" | jq -r '.scale')"

  local logical_w logical_h target_w target_h
  logical_w="$(awk -v w="$monitor_w" -v s="$monitor_scale" 'BEGIN { printf "%d", w / s }')"
  logical_h="$(awk -v h="$monitor_h" -v s="$monitor_scale" 'BEGIN { printf "%d", h / s }')"
  target_w=$((logical_w * 5 / 10))
  target_h=$((logical_h * 5 / 10))

  hyprctl eval "hl.dispatch(hl.dsp.window.resize({ x = $target_w, y = $target_h, relative = false, window = \"address:$address\" }))" >/dev/null
  hyprctl eval "hl.dispatch(hl.dsp.window.center({ window = \"address:$address\" }))" >/dev/null
  hyprctl eval "hl.dispatch(hl.dsp.focus({ window = \"address:$address\" }))" >/dev/null

}

capture_state() {
  local name="$1"
  local theme="$2"
  local key_action="${3:-}"
  local title="hyprmoncfg-shot-$name"
  local screenshot="$output_dir/$name.png"
  local colors
  colors="$(theme_file "$theme")"

  env -u NO_COLOR -u FORCE_COLOR -u CLICOLOR_FORCE \
    CLICOLOR=1 COLORTERM=truecolor TERM=xterm-256color \
    "$terminal_bin" \
      --class="$window_class" \
      --title="$title" \
      --font-size="$font_size" \
      --background-opacity=1 \
      --config-file="$colors" \
      -e "$app_bin" >/dev/null 2>&1 &

  local client
  client="$(wait_for_client "$title")"
  local address
  address="$(printf '%s' "$client" | jq -r '.address')"

  fit_and_center_window "$address"
  # Keep desktop content out of documentation captures even when the user's
  # normal terminal rules use compositor transparency. Only this window changes.
  hyprctl eval "hl.dispatch(hl.dsp.window.set_prop({ window = \"address:$address\", prop = \"opaque\", value = \"1\" }))" >/dev/null
  sleep 1

  if [[ -n "$key_action" ]]; then
    eval "$key_action"
    sleep 0.8
  fi

  client="$(hyprctl -j clients | jq -c --arg address "$address" '.[] | select(.address == $address)' | head -n1)"
  local x y w h border
  x="$(printf '%s' "$client" | jq -r '.at[0]')"
  y="$(printf '%s' "$client" | jq -r '.at[1]')"
  w="$(printf '%s' "$client" | jq -r '.size[0]')"
  h="$(printf '%s' "$client" | jq -r '.size[1]')"
  border="$(hyprctl getoption general:border_size -j | jq -r '.int')"

  grim -g "$((x - border)),$((y - border)) $((w + 2 * border))x$((h + 2 * border))" "$screenshot"
  # Every shot lands in the same centred rectangle, so a window still closing
  # shows through the next one and leaves the previous screen ghosted into it.
  close_client "$address"
}

capture_themed() {
  local theme="$1"
  local suffix="$2"

  printf 'Capturing %s...\n' "$theme"
  capture_state "layout${suffix}" "$theme"
  capture_state "save-profile${suffix}" "$theme" "wtype -k s"
  capture_state "profiles${suffix}" "$theme" "wtype -k 3"
  capture_state "workspaces${suffix}" "$theme" "wtype -k 2"
}

capture_themed "$light_theme" "-light"
capture_themed "$dark_theme" "-dark"

cp "$output_dir/layout-light.png" "$output_dir/layout.png"
cp "$output_dir/save-profile-light.png" "$output_dir/save-profile.png"

printf 'Captured screenshots in %s\n' "$output_dir"
