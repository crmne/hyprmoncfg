---
title: Omarchy Quattro Panel
description: Create multi-monitor layouts for Hyprland in a visual editor and switch them automatically on hotplug and lid events.
nav_order: 4
---

## Displays, handled

The [hyprmoncfg: Multi-Monitor Manager for Omarchy](https://github.com/crmne/omarchy-hyprmoncfg) plugin lets you create multi-monitor layouts for Hyprland in a visual editor and switch them automatically on hotplug and lid events. Open the panel to see the layout that is live right now, the profile that matched it, and whether hyprmoncfg is managing your displays.

![hyprmoncfg panel for Omarchy Quattro](https://raw.githubusercontent.com/crmne/omarchy-hyprmoncfg/main/preview.png)

If you find the panel useful, I'd appreciate a [star on GitHub](https://github.com/crmne/omarchy-hyprmoncfg). It helps get the word out.

Turn management on and hyprmoncfg switches profiles automatically on monitor hotplug and laptop lid events. Turn it off and display ownership goes cleanly back to Omarchy. **Layout and settings** opens the full spatial editor in Omarchy's centered TUI window.

## Install the panel

Get it from the [Omarchy Plugins marketplace](https://omarchyplugins.com/plugin.html?id=crmne.hyprmoncfg), or install it directly:

```bash
omarchy plugin add https://github.com/crmne/omarchy-hyprmoncfg.git --enable
```

If hyprmoncfg is already installed, the panel connects to its daemon over the local IPC socket and updates as monitor state changes.

If it is missing, open the panel and choose **Install hyprmoncfg**. The panel uses Omarchy's normal presented package flow to install the stable AUR package, enables and starts `hyprmoncfgd.service`, and then opens the layout editor. Arrange your monitors, press `s`, and save your first profile. The panel refreshes as soon as it is ready.

## What the panel controls

- **Managed by hyprmoncfg** enables or disables the user daemon
- **Layout and settings** opens the spatial TUI
- **Profile** shows the active hardware-aware profile and whether it was selected automatically

The panel is a desktop surface for the same daemon and IPC protocol used by the TUI and CLI. It does not maintain another copy of monitor state or write Hyprland configuration on its own.

## When Omarchy still moves your displays

Omarchy manages monitors too, and two managers can disagree. Three things keep hyprmoncfg in charge, all of them automatic:

- hyprmoncfg writes its own `~/.config/hypr/hyprmoncfg-monitors.lua` and adds one line at the end of `hyprland.lua` to load it. Loading last means nothing before it, including Omarchy's clamshell toggle, can override the layout you applied. Your `monitors.lua` is left exactly as Omarchy shipped it
- the daemon stops Omarchy's monitor watcher while it owns your displays
- every apply records the internal panel's scale where Omarchy's clamshell script looks for it, so a lid or wake event brings the panel back at your scale rather than its default

Omarchy's clamshell script also drives Hyprland directly with `hyprctl`, which no load order can outrank. The daemon covers that by putting the whole profile back when something outside hyprmoncfg moves the displays, including a profile you picked by hand.

The Omarchy **Display** panel (and Super+/) is the exception. It records each scale change in `~/.local/state/omarchy/monitor-scaling.log` and does not go through hyprmoncfg. When the daemon sees a live scale that matches a recent line in that log, it writes the new scale into the active profile and then re-applies the saved layout. Clamshell and the monitor watcher do not write that log, so they are still reverted.

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
