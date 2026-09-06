---
title: Configuration Files
description: Where profiles are stored and what hyprmoncfg writes to Hyprland.
nav_order: 2
---

## Profile storage

Canonical profile JSON files live in:

```
~/.config/hyprmoncfg/profiles/*.json
```

Each profile has a canonical JSON file. The filename is a simplified version of the profile name -- spaces become hyphens, special characters are dropped. For example, a profile named "Home Office" becomes `home-office.json`.

hyprmoncfg also writes generated `home-office.conf` and `home-office.lua` sidecars next to the JSON file. These are fallback exports for people who want to stop using hyprmoncfg but keep the saved layouts as Hyprland config snippets. The JSON remains the source of truth used by hyprmoncfg and `hyprmoncfgd`.

{% include alert.html type="warning" title="Every Profile File Is A Match Candidate" content="`hyprmoncfgd` scans every `*.json` file in this directory. Old backups, temporary experiments, and duplicate layouts are not ignored just because you forgot about them." %}

Override the storage directory with `--config-dir`:

```bash
hyprmoncfg --config-dir /path/to/profiles
hyprmoncfgd --config-dir /path/to/profiles
```

### What's in a profile

Each profile stores:

- **Monitor outputs**: hardware identity (make, model, serial), resolution, refresh rate, scale, position, transform, VRR mode
- **Workspace settings**: strategy, max workspaces, group size, monitor order, explicit rules

Monitors are identified by hardware key (`make|model|serial`), not connector name. This means your profiles survive connector swaps between boots.

### Commands after applying a profile

The profile's **Exec** field runs a command after Hyprland has applied the layout,
and after confirmation for an interactive preview. It receives
`HYPRLAND_INSTANCE_SIGNATURE` for the compositor that applied the profile, even
when the daemon started without a session environment. Put multiple commands in
an executable script and point Exec at that script.

For commands which also need the graphical session's environment, ask Hyprland
to launch them. For example, on Lua-based Hyprland, this Exec value selects the
TV as XWayland's primary output:

```text
hyprctl dispatch 'hl.dsp.exec_cmd("xrandr --output HDMI-A-1 --primary")'
```

On legacy Hyprland, use `hyprctl dispatch exec 'xrandr --output HDMI-A-1 --primary'`.
Install `xrandr` and replace `HDMI-A-1` with the output you want from
`xrandr --query`. Choose it explicitly for each relevant profile; the leftmost,
largest, or first output is not necessarily the one you want to game on.
The recipe uses a connector name, so update it if you move the cable to another
port. It affects XWayland applications which consult the primary flag, not
native Wayland window placement. Some applications need to be restarted.

Hyprland supplies `DISPLAY` to the launched command. Its dispatch acknowledges
the launch, not the eventual `xrandr` result; run the command in a terminal first
to check the output name. This avoids scanning X sockets, and leaves the profile
schema and existing Exec editor unchanged.

## Profile hygiene

If you want predictable daemon behavior, keep this directory curated:

- Create profiles for every real monitor scenario you expect auto-switching to cover
- Keep one profile per real monitor setup you actually want auto-applied
- Delete stale profiles instead of renaming them and leaving them in place
- Store backups somewhere else if you do not want them considered during matching
- Re-save a profile after major hardware changes instead of accumulating near-duplicates

## Hyprland targets

hyprmoncfg writes a file it creates and owns, so it never has to overwrite a config you or your distribution wrote. Default legacy apply target:

```
~/.config/hypr/hyprmoncfg-monitors.conf
```

Default legacy root config used for source verification:

```
~/.config/hypr/hyprland.conf
```

On Hyprland 0.55+, if `~/.config/hypr/hyprland.lua` exists, hyprmoncfg switches to Lua mode and uses:

```
~/.config/hypr/hyprmoncfg-monitors.lua
~/.config/hypr/hyprland.lua
```

If Hyprland 0.55+ is still using `hyprland.conf`, hyprmoncfg stays in legacy mode. When `HYPRLAND_CONFIG` is set, hyprmoncfg uses it as the root config path. An explicit `--hypr-config` takes precedence, and paths ending in `.conf` or `.lua` force the matching format.

Override either path:

```bash
hyprmoncfg --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
hyprmoncfgd --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
hyprmoncfg --monitors-conf /path/to/monitors.lua --hypr-config /path/to/hyprland.lua
hyprmoncfgd --monitors-conf /path/to/monitors.lua --hypr-config /path/to/hyprland.lua
```

To configure both binaries once, set the equivalent environment variables:

```bash
export HYPRMONCFG_MONITORS_CONF=/path/to/monitors.conf
export HYPRLAND_CONFIG=/path/to/hyprland.conf
```

Explicit flags take precedence. A systemd-managed daemon must receive these variables through the user manager's environment; restart `hyprmoncfgd` after changing them.

## The include-chain check

hyprmoncfg confirms that Hyprland loads the generated monitor file. This catches a surprisingly common problem: a tool writes a config file that Hyprland never reads, so nothing happens and you're left wondering why.

hyprmoncfg adds the include itself, at the end of your root config, and moves it back to the end if something is appended after it. Loading last is what makes the applied layout final: any monitor rule read afterwards would override it. `hyprmoncfg doctor` reports the current state and `--fix` settles it on demand.

Saved profiles are authoritative for the connected monitor set. When a connected output is absent from the selected profile, the generated file writes an explicit disabled rule for it. This overrides an earlier wildcard such as Omarchy's preferred/automatic monitor default instead of letting that default leave an unwanted display enabled.

The Lua include resolves its own path at load time:

```lua
dofile((os.getenv("XDG_CONFIG_HOME") or (os.getenv("HOME") .. "/.config")) .. "/hypr/hyprmoncfg-monitors.lua")
```

so the same config works under a different user, a different home, or a dotfile repo shared between machines. Legacy configs get `source = ~/.config/hypr/hyprmoncfg-monitors.conf`. If your Hyprland config is managed by a dotfile tool, keep that line in your source copy so the two do not keep undoing each other.

For Lua configs, hyprmoncfg reloads Hyprland and asks the active Lua state to confirm that the generated monitor file actually ran. If it did not, hyprmoncfg restores the previous file (or removes a newly created one) and reloads again. This works with `require`, `pcall`, `dofile`, custom `package.path` values, and computed include paths because Hyprland loads the real config itself.

If your files live elsewhere, point hyprmoncfg at them with `--monitors-conf` and `--hypr-config`; it will add an include for whatever target you name.

Upgrading from a version that wrote `monitors.conf` or `monitors.lua` needs nothing from you. On the next apply, hyprmoncfg writes its own file, adds the include, and replaces the body of the file it used to generate with a note saying where the rules moved. A `monitors.conf` or `monitors.lua` that hyprmoncfg did not generate is never touched.

## What gets written

When you apply a profile (via TUI, CLI, or daemon), hyprmoncfg writes the active generated monitor file. Legacy configs get `hyprmoncfg-monitors.conf` with either `monitorv2 { }` blocks (Hyprland 0.50+) or legacy `monitor = ` lines, depending on your Hyprland version. Lua configs get `hyprmoncfg-monitors.lua` with `hl.monitor({ ... })` and `hl.workspace_rule({ ... })` calls.

hyprmoncfg marks generated files with `Generated by hyprmoncfg` on the first line. It fully manages and rewrites a missing target or a target carrying that marker.

An existing file without the marker is treated as user-owned. The TUI and interactive CLI ask before replacing it; the daemon and non-interactive CLI refuse to replace it. To preserve a hand-written `monitors.conf` or `monitors.lua`, select a separate generated target such as `hyprmoncfg-monitors.conf` or `hyprmoncfg-monitors.lua` with `--monitors-conf` (or `HYPRMONCFG_MONITORS_CONF`) and include that file from the Hyprland root config.

Per-profile `*.conf` and `*.lua` files in `~/.config/hyprmoncfg/profiles/` are different: they are exported copies of each saved profile. Applying a profile still regenerates the active target from JSON and the current monitor state so lid handling, duplicate monitor resolution, Hyprland version detection, verification, and rollback keep working.

Do not put unrelated Hyprland settings in that file. Keep blocks like `render`, `cursor`, `misc`, and `env` in other sourced config files.

The file is written atomically (temp file + rename) to prevent partial writes from corrupting your config. Interactive applies keep an in-memory snapshot and restore it on rejection, timeout, interruption, or quit unless you explicitly keep the new configuration.

## Portability

Profile JSON files are portable across machines. The daemon uses hardware identity matching to score profiles, so a profile saved on your desktop will work on your laptop if the same monitors are connected. The generated `.conf` and `.lua` sidecars are useful fallback snippets, but JSON is the portable profile format hyprmoncfg reads.

Add `~/.config/hyprmoncfg` to your dotfile manager to share profiles across all your machines. See the [dotfiles guide](/dotfiles/) for details.
