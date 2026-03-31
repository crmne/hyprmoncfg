---
title: Daemon Behavior
description: How hyprmoncfgd watches for monitor changes and applies the right profile automatically.
nav_order: 3
---

## Why a daemon

You save profiles with the TUI. But who applies them when you're not looking?

That's what `hyprmoncfgd` does. It runs in the background, watches for monitor hotplug events, and applies the best matching profile automatically. Plug in a monitor, undock your laptop, connect to a projector -- the daemon handles it.

This is especially useful if you move between setups regularly. A conference projector, a coworking space monitor, your desk at home -- each one has different resolution, position, and scale requirements. Save a profile once, and the daemon takes care of it from then on.

## How it works

1. Read the current monitor set from Hyprland
2. Score every saved profile against the connected hardware
3. Pick the best match (by hardware identity, not connector name)
4. Write `monitors.conf` atomically
5. Reload Hyprland
6. Re-read monitor state and verify the result

The daemon uses the **same apply engine** as the TUI. There is no separate "best effort" code path. If the TUI can apply a profile correctly, so can the daemon.

## Profile matching

Profiles are matched by hardware identity (make, model, serial) — not connector name. Each profile is scored against the connected monitors:

| Condition | Points |
|---|---|
| Monitor enabled in profile and connected | +100 |
| Monitor disabled in profile but connected | +50 |
| Monitor enabled in profile but not connected | −30 |
| Connected monitor not in the profile at all | −20 |

Highest score wins. Ties break alphabetically. The daemon skips re-applying the same profile twice.

## Setup

If you installed via the AUR or a package manager, the systemd service is already installed. Just enable it:

```bash
systemctl --user enable --now hyprmoncfgd
```

If you built from source and the service file is not installed:

```bash
mkdir -p ~/.config/systemd/user
cp packaging/systemd/hyprmoncfgd.local.service ~/.config/systemd/user/hyprmoncfgd.service
systemctl --user daemon-reload
systemctl --user enable --now hyprmoncfgd
```

## Run manually

For testing or one-off use:

```bash
hyprmoncfgd
```

### Useful flags

```bash
hyprmoncfgd --debounce 1500ms     # wait longer before applying after a plug event
hyprmoncfgd --poll-interval 5s    # how often to check if socket2 is unavailable
hyprmoncfgd --profile desk        # always apply this specific profile
hyprmoncfgd --quiet               # suppress log output
```

## Forcing a specific profile

Use `--profile <name>` when you want the daemon to skip matching and always apply one chosen profile. Useful when you know exactly what's connected and don't want the scoring algorithm involved.

```bash
hyprmoncfgd --profile conference-projector
```

## Logs

```bash
journalctl --user -u hyprmoncfgd -f
```

You'll see profile scoring, match results, apply steps, and any verification failures.
