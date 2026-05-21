---
title: Configuration Files
description: Where profiles are stored and what hyprmoncfg writes to Hyprland.
nav_order: 2
---

## Profile storage

Profiles live in:

```
~/.config/hyprmoncfg/profiles/*.json
```

Each profile is a single JSON file. The filename is a simplified version of the profile name -- spaces become hyphens, special characters are dropped. For example, a profile named "Home Office" becomes `home-office.json`.

These files are managed by hyprmoncfg. You can read them, but there's no reason to edit them by hand -- the TUI and CLI handle that for you.

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

## Profile hygiene

If you want predictable daemon behavior, keep this directory curated:

- Create profiles for every real monitor scenario you expect auto-switching to cover
- Keep one profile per real monitor setup you actually want auto-applied
- Delete stale profiles instead of renaming them and leaving them in place
- Store backups somewhere else if you do not want them considered during matching
- Re-save a profile after major hardware changes instead of accumulating near-duplicates

## Hyprland targets

Default legacy apply target:

```
~/.config/hypr/monitors.conf
```

Default legacy root config used for source verification:

```
~/.config/hypr/hyprland.conf
```

On Hyprland 0.55+, if `~/.config/hypr/hyprland.lua` exists, hyprmoncfg switches to Lua mode and uses:

```
~/.config/hypr/monitors.lua
~/.config/hypr/hyprland.lua
```

If Hyprland 0.55+ is still using `hyprland.conf`, hyprmoncfg stays in legacy mode. Explicit `--hypr-config` paths ending in `.conf` or `.lua` force the matching format.

Override either path:

```bash
hyprmoncfg --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
hyprmoncfgd --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
hyprmoncfg --monitors-conf /path/to/monitors.lua --hypr-config /path/to/hyprland.lua
hyprmoncfgd --monitors-conf /path/to/monitors.lua --hypr-config /path/to/hyprland.lua
```

## The include-chain check

Before writing anything, hyprmoncfg confirms Hyprland includes the generated monitor file. This catches a surprisingly common problem: a tool writes a config file that Hyprland never reads, so nothing happens and you're left wondering why.

For legacy configs, add `source = ~/.config/hypr/monitors.conf` to `hyprland.conf`. For Lua configs, add `require("monitors")` to `hyprland.lua`. If your files live elsewhere, point hyprmoncfg at them with `--monitors-conf` and `--hypr-config`.

## What gets written

When you apply a profile (via TUI, CLI, or daemon), hyprmoncfg writes the active generated monitor file. Legacy configs get `monitors.conf` with either `monitorv2 { }` blocks (Hyprland 0.50+) or legacy `monitor = ` lines, depending on your Hyprland version. Lua configs get `monitors.lua` with `hl.monitor({ ... })` and `hl.workspace_rule({ ... })` calls.

`monitors.conf` and `monitors.lua` are generated output. hyprmoncfg fully manages and rewrites the active target file on every apply.

Do not put unrelated Hyprland settings in that file. Keep blocks like `render`, `cursor`, `misc`, and `env` in other sourced config files.

The file is written atomically (temp file + rename) to prevent partial writes from corrupting your config.

## Portability

Profile JSON files are portable across machines. The daemon uses hardware identity matching to score profiles, so a profile saved on your desktop will work on your laptop if the same monitors are connected.

Add `~/.config/hyprmoncfg` to your dotfile manager to share profiles across all your machines. See the [dotfiles guide](/dotfiles/) for details.
