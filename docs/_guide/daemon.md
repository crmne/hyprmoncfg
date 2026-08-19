---
title: Daemon Behavior
description: How hyprmoncfgd watches for monitor changes and applies the right profile automatically.
nav_order: 3
---

## Why a daemon

You save profiles with the TUI. But who applies them when you're not looking?

That's what `hyprmoncfgd` does. It runs in the background, watches for monitor hotplug and lid events, and applies the best matching profile automatically. Plug in a monitor, close the lid, undock your laptop, connect to a projector -- the daemon handles it.

On Omarchy releases that start `omarchy-hyprland-monitor-watch`, the daemon takes exclusive ownership of monitor state. It stops the watcher's exact transient user scope, keeps later copies suppressed, and starts the watcher again when `hyprmoncfgd` exits while Hyprland is still running. This prevents Omarchy's clamshell reconciliation from overwriting an active hyprmoncfg profile.

{% include alert.html type="warning" title="Static Configs On Omarchy" content="A generated monitor config cannot provide runtime process ownership by itself. If you use a generated <code>monitors.lua</code> without running <code>hyprmoncfgd</code>, Omarchy's monitor watcher remains active and may reconcile the laptop panel using Omarchy's own settings. Disable that watcher separately or run the daemon." %}

This is especially useful if you move between setups regularly. A conference projector, a coworking space monitor, your desk at home -- each one has different resolution, position, and scale requirements. Save a profile once, and the daemon takes care of it from then on.

## Setup

AUR, Fedora COPR, Nixpkgs, and Gentoo GURU:

```bash
systemctl --user enable --now hyprmoncfgd
```

Void Linux with Blackhole-vl:

```text
exec-once = hyprmoncfgd
```

Manual install:

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
2. Score every saved profile against the connected hardware (see [Profile matching](#profile-matching) below for how scoring works)
3. Pick the highest-scoring profile
4. If the lid is closed and an external monitor is connected, force internal laptop-panel outputs off for this apply
5. Write the active generated monitor file atomically (temp file + rename, so a crash mid-write can't corrupt your config)
6. Tell Hyprland to reload
7. Re-read monitor state and verify the result matches what was intended

If the winning profile is the same one that's already applied, the daemon skips re-applying it. You won't see unnecessary reloads.

On Omarchy, a scale change from the built-in Display panel is treated as an edit to the active profile rather than an external drift to revert. The daemon recognizes it from `~/.local/state/omarchy/monitor-scaling.log` (written only by that panel and Super+/), saves the new scale, and re-applies the rest of the layout. See [Omarchy Quattro Panel](/omarchy/).

When every enabled display is DPMS-off, the daemon treats monitor add/remove events as part of display sleep rather than physical hotplug. It keeps the current profile in place, waits for the displays to wake, and then waits for two seconds without another monitor event before matching once. A real dock or undock that happened while the machine slept is still applied after the monitor set stabilizes.

Suspend gets the same respect. The daemon listens for logind's sleep signal, and a lid close that suspends the machine is treated as suspend, not clamshell: the pending switch is discarded instead of being carried across the nap, because by the time it would run, the lid is open again and acting on the stale close would turn the panel off in your face. On resume the daemon re-reads the physical lid switch, tells Hyprland to wake every display -- so both screens light up from the lid opening, not from your first keypress -- and re-matches once the monitor set settles. Opening the lid wakes the displays the same way. As a last line of defense, every apply re-reads the lid switch first, so a stale cached lid state can never decide what happens to your panel.

The daemon uses the **same apply engine** as the TUI. There is no separate "best effort" code path. If the TUI can apply a profile correctly, so can the daemon.

When the daemon is running, it is also the canonical writer. The TUI, CLI, and integrations connect to `$XDG_RUNTIME_DIR/hyprmoncfgd.sock` and ask the daemon to preview, confirm, revert, save, or delete through the same versioned IPC protocol. If the daemon is not running, the TUI and CLI acquire the writer lock and use the core engine directly.

Interactive profile changes are deliberate overrides. After you confirm one, the daemon keeps it in place until the monitor set or lid state changes. Automatic matching then resumes.

## Profile matching

Profiles are matched by hardware identity (make, model, serial) -- not connector name. This means your layout survives when monitors swap between `DP-1` and `DP-2` across reboots. Each profile is scored against the currently connected monitors:

| Condition | Points |
|---|---|
| Monitor enabled in profile and connected | +100 |
| Monitor disabled in profile but connected | +50 |
| Connected monitor not in the profile | −20 |
| Monitor enabled in profile but not connected | −30 |
| Monitor disabled in profile and not connected | −10 |

Highest score wins. Ties break alphabetically by profile name. A profile that mentions a monitor which is not plugged in pays for it either way, so the profile that describes exactly the connected displays beats a larger profile that happens to include them. The profiles tab shows every score, along with this breakdown for the selected profile.

On laptops, the daemon also reads lid state. UPower is optional, but recommended: with UPower available, lid changes arrive as D-Bus events and the daemon can react immediately. Without UPower, the daemon falls back to polling `/proc/acpi/button/lid/*/state` at `--lid-poll-interval`, which defaults to `1s` and is not available on every system. If neither source exists, lid-aware switching is disabled and monitor hotplug still works.

Lid state is not a separate profile type. Save the profile for the monitor setup you actually have attached. When the lid is closed and an external monitor is connected, hyprmoncfg treats internal laptop-panel outputs like `eDP-1`, `LVDS-1`, or `DSI-1` as forced off for that apply. Saved profiles are not rewritten. If workspace rules target the forced-off internal panel, those workspaces are moved to the first enabled external output in the selected profile.

{% include alert.html type="warning" title="Every Profile Is A Candidate" content="The daemon does not know which profiles are \"real\" and which were temporary experiments. It scores every JSON file in your profiles directory. An old throwaway profile with a high enough score will win over the one you actually want." %}

If you want reliable auto-switching:

- Save profiles for every real monitor setup you want the daemon to handle
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
hyprmoncfgd --wake-settle 2s      # quiet period after displays wake
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

The daemon owns a per-session writer lock. A second daemon or direct writer exits instead of fighting over the generated monitor config.

## Logs

```bash
journalctl --user -u hyprmoncfgd -f
```

The log shows every step: which profiles were scored, what each one scored, which one won, what generated monitor config was written, and whether verification passed. This is the first place to look when you want to understand why the daemon picked a particular profile.

{% include alert.html type="tip" title="Separate Matching From Applying" content="If you're not sure whether the daemon picked the wrong profile or failed to apply the right one, test the profile directly with <code>hyprmoncfg apply &lt;name&gt;</code>. If the layout looks correct, the problem is matching, not applying -- check the logs and your profile directory." %}

{% include alert.html type="important" title="Filing A Matching Bug" content="If you think the daemon selected the wrong profile, <a href=\"https://github.com/crmne/hyprmoncfg/issues/new\">open an issue</a> and include <strong>all</strong> profiles from <code>~/.config/hyprmoncfg/profiles/</code>, not just the one you expected to win. Matching depends on the full candidate set." %}
