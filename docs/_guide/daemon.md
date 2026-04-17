---
title: Daemon Behavior
description: How hyprmoncfgd watches for monitor changes and applies the right profile automatically.
nav_order: 3
---

## Why a daemon

You save profiles with the TUI. But who applies them when you're not looking?

That's what `hyprmoncfgd` does. It runs in the background, watches for monitor hotplug and lid events, and applies the best matching profile automatically. Plug in a monitor, close the lid, undock your laptop, connect to a projector -- the daemon handles it.

This is especially useful if you move between setups regularly. A conference projector, a coworking space monitor, your desk at home -- each one has different resolution, position, and scale requirements. Save a profile once, and the daemon takes care of it from then on.

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

That's it. The daemon is running. The rest of this page explains how it decides which profile to apply and how to troubleshoot it.

## How it works

When the daemon detects a monitor or lid-state change, it runs through these steps:

1. Read the current monitor set from Hyprland
2. Score every saved profile against the connected hardware and known lid state (see [Profile matching](#profile-matching) below for how scoring works)
3. Pick the highest-scoring profile
4. Write `monitors.conf` atomically (temp file + rename, so a crash mid-write can't corrupt your config)
5. Tell Hyprland to reload
6. Re-read monitor state and verify the result matches what was intended

If the winning profile is the same one that's already applied, the daemon skips re-applying it. You won't see unnecessary reloads.

The daemon uses the **same apply engine** as the TUI. There is no separate "best effort" code path. If the TUI can apply a profile correctly, so can the daemon.

## Profile matching

Profiles are matched by hardware identity (make, model, serial) -- not connector name. This means your layout survives when monitors swap between `DP-1` and `DP-2` across reboots. Each profile is scored against the currently connected monitors:

| Condition | Points |
|---|---|
| Monitor enabled in profile and connected | +100 |
| Monitor disabled in profile but connected | +50 |
| Connected monitor not in the profile | −20 |
| Monitor enabled in profile but not connected | −30 |

Highest score wins. Ties break alphabetically by profile name.

On laptops, the daemon also reads lid state. UPower is optional, but recommended: with UPower available, lid changes arrive as D-Bus events and the daemon can react immediately. Without UPower, the daemon falls back to polling `/proc/acpi/button/lid/*/state` at `--lid-poll-interval`, which defaults to `1s` and is not available on every system. If neither source exists, lid-aware switching is disabled and monitor hotplug still works.

If an internal panel and an external monitor are both present, closed-lid matching prefers profiles where the internal panel is disabled or omitted. Open-lid matching prefers profiles where the internal panel is enabled. Workspace rules in the winning profile are applied too, so existing workspaces move to the profile's configured monitor targets.

{% include alert.html type="warning" title="Every Profile Is A Candidate" content="The daemon does not know which profiles are \"real\" and which were temporary experiments. It scores every JSON file in your profiles directory. An old throwaway profile with a high enough score will win over the one you actually want." %}

If you want reliable auto-switching:

- Save profiles for every real monitor setup you want the daemon to handle
- For clamshell mode, save one lid-open profile and one lid-closed profile with the internal panel disabled
- Keep one profile per setup -- don't accumulate near-duplicates
- Delete experimental profiles when you're done experimenting
- If two profiles tie, the one whose name comes first alphabetically wins
- When auto-switching picks the wrong profile, start by listing the files in `~/.config/hyprmoncfg/profiles/` -- a forgotten profile is almost always the answer

## Run manually

For testing or one-off use:

```bash
hyprmoncfgd
```

### Useful flags

```bash
hyprmoncfgd --debounce 1500ms     # wait longer before applying after a plug event
hyprmoncfgd --poll-interval 5s    # how often to run fallback monitor checks
hyprmoncfgd --lid-poll-interval 1s # how often to run fallback lid checks
hyprmoncfgd --profile desk        # always apply this specific profile
hyprmoncfgd --quiet               # suppress log output
```

## Forcing a specific profile

Use `--profile <name>` to bypass automatic matching entirely. The daemon applies this one profile every time, regardless of what's connected. This is useful when you know exactly which setup you're on and want to eliminate any chance of a wrong match.

Stop the running daemon first, then start it with the flag:

```bash
systemctl --user stop hyprmoncfgd
hyprmoncfgd --profile conference-projector
```

Running two daemons at the same time causes them to fight over `monitors.conf` -- your monitors will flicker back and forth between profiles.

## Logs

```bash
journalctl --user -u hyprmoncfgd -f
```

The log shows every step: which profiles were scored, what each one scored, which one won, what was written to `monitors.conf`, and whether verification passed. This is the first place to look when you want to understand why the daemon picked a particular profile.

{% include alert.html type="tip" title="Separate Matching From Applying" content="If you're not sure whether the daemon picked the wrong profile or failed to apply the right one, test the profile directly with <code>hyprmoncfg apply &lt;name&gt;</code>. If the layout looks correct, the problem is matching, not applying -- check the logs and your profile directory." %}

{% include alert.html type="important" title="Filing A Matching Bug" content="If you think the daemon selected the wrong profile, <a href=\"https://github.com/crmne/hyprmoncfg/issues/new\">open an issue</a> and include <strong>all</strong> profiles from <code>~/.config/hyprmoncfg/profiles/</code>, not just the one you expected to win. Matching depends on the full candidate set." %}
