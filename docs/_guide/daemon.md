---
title: Daemon Behavior
description: How hyprmoncfgd matches profiles and applies them safely.
---

`hyprmoncfgd` is the automatic profile switcher. It listens for monitor hotplug events from Hyprland `socket2`, falls back to polling, then applies the best matching saved profile.

## What it does

1. Read the current monitor set from Hyprland.
2. Score saved profiles against connected monitor hardware.
3. Pick the best match.
4. Write the configured `monitors.conf` target.
5. Reload Hyprland.
6. Re-read monitor state and verify the result.

The daemon uses the same apply engine as the TUI. There is no separate “best effort” code path.

## Run it manually

```bash
hyprmoncfgd
```

Useful flags:

```bash
hyprmoncfgd --profile desk
hyprmoncfgd --debounce 1500ms --poll-interval 5s
hyprmoncfgd --monitors-conf ~/.config/hypr/monitors.conf
hyprmoncfgd --hypr-config ~/.config/hypr/hyprland.conf
hyprmoncfgd --quiet
```

## When to use a forced profile

Use `--profile <name>` when you want the daemon to stop matching automatically and always re-apply one chosen profile for the current session.

## Logs

```bash
journalctl --user -u hyprmoncfgd -f
```
