---
title: Omarchy Quattro Panel
description: Arrange displays, plan workspaces, and manage automatic monitor profiles in Omarchy's native graphical panel.
nav_order: 4
---

## Displays, handled

The [hyprmoncfg: Multi-Monitor Manager for Omarchy](https://github.com/crmne/omarchy-hyprmoncfg) plugin is a native graphical panel for Omarchy Quattro. Arrange displays visually, plan workspaces, and save hardware-aware profiles that switch automatically on hotplug and lid events. Open it from the bar to see the live layout, the matching profile, and whether hyprmoncfg is managing your displays.

hyprmoncfg is a separate project from [HyprMon](https://github.com/erans/hyprmon), the terminal monitor tool linked in Omarchy's manual.

![hyprmoncfg panel for Omarchy Quattro](https://raw.githubusercontent.com/crmne/omarchy-hyprmoncfg/main/preview.png)

If you find the panel useful, I'd appreciate a [star on GitHub](https://github.com/crmne/omarchy-hyprmoncfg). It helps get the word out.

Choose **Expand** to open the graphical editor inside the panel. Its **Layout**, **Workspaces**, and **Profiles** pages let you configure the setup without opening a terminal. The optional **TUI** button opens the companion terminal editor, which is also useful from a TTY or over SSH when you need to work on a running Hyprland session without its graphical panel.

Turn management on and hyprmoncfg switches profiles automatically on monitor hotplug and laptop lid events. Turn it off to hand display management back to Omarchy. Monitor ownership and workspace ownership are separate; see the compatibility notes below.

## Install the panel

Get it from the [Omarchy Plugins marketplace](https://plugins.omarchy.org/plugin.html?id=crmne.hyprmoncfg), or install it directly:

```bash
omarchy plugin add https://github.com/crmne/omarchy-hyprmoncfg.git --enable
```

If hyprmoncfg is already installed, the panel connects to its daemon over the local IPC socket and updates as monitor state changes.

If it is missing, open the panel and choose **Install hyprmoncfg**. The panel uses Omarchy's normal presented package flow to install the stable AUR package and enables and starts `hyprmoncfgd.service`. The current installer opens the companion TUI after installation. You can use it for initial setup or return to the panel and choose **Expand** to arrange and save your first layout graphically. The panel refreshes as soon as the daemon is ready.

## What the panel controls

- **Managed by hyprmoncfg** enables or disables the user daemon
- **Expand** opens the native graphical editor
- **Layout** provides drag-and-drop arrangement with snapping, resolution, refresh rate, scale, rotation, mirroring, and colour settings
- **Workspaces** provides manual, sequential, or interleaved plans, plus **Off** to leave workspace rules to another manager
- **Profiles** manages saved arrangements and automatic selection
- **Preview & save** lets you try a layout before confirming and saving it; unconfirmed changes revert automatically after the preview deadline
- **TUI** opens the optional terminal editor

The panel is a desktop surface for the same daemon and IPC protocol used by the TUI and CLI. It does not maintain another copy of monitor state or write Hyprland configuration on its own.

## When Omarchy still moves your displays

Omarchy also changes monitor state from its watcher, clamshell, startup, and wake paths. These can conflict with another monitor manager. hyprmoncfg includes automatic compatibility measures:

- hyprmoncfg writes its own `~/.config/hypr/hyprmoncfg-monitors.lua` and adds one line at the end of `hyprland.lua` to load it. This gives its generated rules precedence over earlier configuration during a reload. Scripts that send later monitor commands can still change the live layout
- the daemon stops Omarchy's monitor watcher while it owns your displays
- each apply shares the internal panel's position and scale through a marked block at the start of an existing `monitors.lua`, because Omarchy's clamshell script reads that file rather than following generated includes. The rest of your file is preserved, and the block is removed when management is handed back. The scale is also recorded in Omarchy's state file

The daemon also shares Omarchy's native **Laptop Display** toggle. When a profile
disables the internal display while an external display is active, it records
that choice in Omarchy's manual-disable flag before reloading. Omarchy's wake
and clamshell paths already respect this flag, so they leave the disabled panel
alone instead of repeatedly enabling it on the lock screen.

Changing **Hardware → Laptop Display** or its keybinding updates the active
profile's internal-display setting. The daemon uses the saved profile as its
base, preserving the display's position, scale, and mode when turning it back on.
If an older external-only profile omits the laptop display entirely, enabling it
adds the panel to the right of the existing layout, using its reported mode or
an advertised mode when the disabled panel has no live geometry.
The change is saved after a successful apply and keeps automatic profile
selection in its current mode. A toggle can take up to the daemon's polling and
debounce interval to appear. Preview rollback restores the previous native flag
as well as the generated configuration.

Flag changes during sleep, lid changes, and external-monitor reconnection are
treated as recovery activity, not as edits to your saved profile.

This integration uses Omarchy's existing user state; it does not modify packaged
scripts or require the proposed monitor-management switch. The flag continues
to reflect the last applied layout when management is handed back. Other tools
can still send conflicting monitor commands directly, and future Omarchy
changes may require adapting this integration.

### Omarchy 4.5 per-monitor workspaces

Omarchy 4.5 release candidates introduce separate workspace slots 1 through 10
on each monitor. Number-key bindings and bar labels use those local slots,
while Hyprland still uses global workspace IDs underneath. Omarchy also moves
and renumbers workspaces to keep them in each monitor's block.

This can conflict with a hyprmoncfg profile that assigns global workspaces
across displays, such as 1–3 on one screen and 4–6 on another. The monitor
compatibility measures in hyprmoncfg 1.25.1 and earlier do not disable Omarchy's
workspace manager. Loading workspace rules last is not sufficient to prevent
later reconciliation from changing them.

If you want Omarchy to own workspace placement, choose **Off** for the profile's
workspace strategy. hyprmoncfg then writes no workspace rules for that profile
and retains its saved plan for later use. This does not restore global
number-key behaviour or undo workspace IDs Omarchy has already changed.

Keeping global workspace assignments requires Omarchy's per-monitor behaviour
to stand down as well. The proposed [per-monitor workspace
toggle](https://github.com/omacom/omarchy/pull/16213) is tracked upstream;
it is not part of Omarchy 4.5.0rc2.

Development builds after 1.25.1 make an enabled workspace planner own global
numbering. The existing generated Lua file disables Omarchy's workspace
reassignment and restores global shortcuts, without another include or changes
to packaged Omarchy files. This applies through the shared engine used by the
daemon, TUI and panel. **Off** continues to delegate workspace management.

Pair global mode with the optional [Global Workspaces
widget](https://github.com/crmne/omarchy-global-workspaces). It displays the real
workspace IDs and highlights the globally focused workspace on every bar,
matching Omarchy's original widget. Omarchy's stock RC2 widget still assumes
per-monitor numbering. Switching ownership does not
automatically move or renumber existing windows.

### Check the configuration load order

Check the load order at any time:

```bash
hyprmoncfg doctor
```

If your Hyprland config is managed by a dotfile tool such as chezmoi or stow, keep the line `hyprmoncfg doctor` prints in your source copy. Otherwise your dotfile tool and hyprmoncfg will keep re-adding and removing it.

## Remove it

```bash
omarchy plugin remove crmne.hyprmoncfg
```

Removing the panel leaves hyprmoncfg and your saved profiles installed. Turn off **Managed by hyprmoncfg** first if you also want Omarchy to resume display management.
