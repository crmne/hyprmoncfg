---
title: Virtual Desktops
description: Run hyprmoncfg alongside hyprland-virtual-desktops so one keypress moves every monitor at once.
nav_order: 6
---

## The problem with Hyprland workspaces

You have three monitors. You press `SUPER+2` and exactly one of them changes, because in Hyprland a workspace belongs to a single monitor.

That gets in the way when you want scenes. Work is a browser, an editor, and logs across all three screens. Music is something else entirely. You want one keypress to swap the whole set, not three keypresses to swap them one at a time. GNOME and KDE work that way; Hyprland does not.

hyprmoncfg follows Hyprland here. Its workspace planner decides which workspaces live on which display, and stops there -- it will not pretend the compositor has a concept it does not have.

## Use hyprland-virtual-desktops

[hyprland-virtual-desktops](https://github.com/levnikmyskin/hyprland-virtual-desktops) adds scenes properly. It is a Hyprland plugin, a real `.so` loaded into the compositor, so it changes how workspaces work rather than driving them from outside.

```bash
hyprctl dispatch vdesk 2        # every monitor moves together
hyprctl dispatch movetodesk 3   # send a window to another desk
hyprctl dispatch nextdesk
```

## Turn the workspace planner off

hyprmoncfg's planner writes `workspace_rule` entries pinning workspace 1 to this monitor and workspace 2 to that one. virtual-desktops assigns workspaces to monitors itself, differently per desk. Leave both on and whichever ran last wins.

So set **Strategy** to Off in the Workspaces tab, on every profile you use with the plugin. hyprmoncfg then writes no workspace rules at all, and the split is clean: hyprmoncfg puts the monitors where they belong, virtual-desktops decides what appears on them.

You lose nothing. Spreading workspaces across displays is what the plugin does, and it does it per desk rather than once.

## What happens when you dock

hyprmoncfg matches the docked profile, applies the mode, scale, rotation and position for each display, reloads Hyprland, and stops. It does not touch workspaces.

virtual-desktops takes it from there. It notices the monitor set changed and restores which workspace each monitor was showing, using `plugin:virtual-desktops:rememberlayout`. Set that to `monitors` rather than `size` -- it keys on monitor identity, which is what hyprmoncfg matches profiles on, so the two agree about what counts as the same setup.

If the plugin needs a nudge afterwards, every profile has an **Exec** field that runs once the layout is applied and Hyprland has reloaded. Point it at a script:

```bash
#!/usr/bin/env bash
hyprctl dispatch vdeskreset
hyprctl dispatch vdesk 1
```

The command runs directly rather than through a shell, so a `&&` or a pipeline belongs in the script, not in the field.

## A practical example

1. Open the TUI, arrange your docked layout, save it as `desk`
2. Undock, arrange the laptop on its own, save that as `mobile`
3. Set **Strategy** to Off on both, in the Workspaces tab
4. Set `plugin:virtual-desktops:rememberlayout = monitors` in your Hyprland config

Dock the laptop and hyprmoncfg puts the monitors back where they belong. Press `SUPER+2` and all of them move together.
