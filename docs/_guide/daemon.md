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
4. If the lid is closed and an external monitor is already usable and will remain enabled, force internal laptop-panel outputs off for this apply
5. Write the active generated monitor file atomically (temp file + rename, so a crash mid-write can't corrupt your config)
6. Tell Hyprland to reload
7. Re-read monitor state and verify the result matches what was intended

If the winning profile is the same one that's already applied, the daemon skips re-applying it. You won't see unnecessary reloads.

An output absent from a non-strict profile is added to its right edge, vertically
centered against the adjacent independent display using logical dimensions. For
simultaneous arrivals, each new display becomes the next neighbor. The backend
chooses the greatest advertised pixel area, then the highest refresh at that
resolution, with VRR off and 8-bit sRGB. Existing output settings and an enabled
workspace plan are preserved; the saved profile is not overwritten.
Physical-size scale recommendations and explicit failed-wake/cold-start rescue
remain separate follow-up work.

Failed automatic applies retry independently of monitor-change events, starting
after 2 seconds with capped exponential backoff up to 30 seconds. Transient busy
reads and writer contention retry after the normal debounce interval. Successful
application stops retries. Preview ownership defers retries; suspend, intentional
display sleep and unmanaged mode stop them. Resume or a later wake/topology event
restarts reconciliation. Verification reports all failed outputs, not just the
first. Monitor/workspace discovery queries have a 750ms timeout. This does not
prove physical projector readiness.

Resuming or opening the lid is a request for light. When displays still report
DPMS off right afterwards, the daemon treats it as a wake that did not take and
repeats it up to three times before treating the displays as asleep by choice.
It stops early once any display is seen awake or the layout applies. If the
built-in panel is off while the lid is open (it was switched off with the lid
shut and the external stayed asleep), the daemon applies the profile anyway,
so the panel comes back without waiting for the external. The wake command uses
the Lua or legacy syntax Hyprland last reported, even when Hyprland is too busy
to answer right after resume.

### Displays that won't stay on

Some displays can't wake straight into a demanding mode such as 4K at 144 Hz. They
drop off the link a second or two after connecting. When they come back, they
show "No signal" and go back to sleep. Woken at about 60 Hz and switched to the
saved mode once awake, they work.

So when a display drops within 4 seconds of connecting at its saved settings,
the daemon remembers that it needs a gentle wake:

1. While the display is disconnected, its rule in the generated config wakes it
   at the refresh rate closest to 60 Hz, at the same resolution, with VRR off.
2. Once it has stayed connected for 10 seconds, the daemon switches it to its
   saved settings.

Displays that come up cleanly never drop, so they keep coming up in one step.

If that switch makes the display drop again within 8 seconds, the daemon steps
its settings down one at a time, and keeps the first that sticks:

1. VRR off.
2. The next lower refresh rate at the same resolution, for example 144 Hz to 120 Hz.
3. The refresh rate closest to 60 Hz.

The same steps apply to a display that comes back from sleep enabled but
without a mode twice in a row.

Resolution, scale and position never change, so other displays and windows stay
where they are. The saved profile never changes either. `hyprmoncfg status`
names each stepped-down display and how it runs now, and the daemon log says
why. The gentle wake and the steps are remembered per display in
`~/.config/hyprmoncfg/display-fallbacks.json`, so a restart doesn't repeat the
struggle.

Confirming any layout clears the steps, so the next switch tries the saved
settings again. It keeps the gentle wake, because that still ends at the saved
settings. Editing the saved mode or VRR of a display also clears its steps. To
forget a gentle wake, delete the file and restart the daemon.

Displays with the same description can't be told apart here, so they are never
changed. The daemon also can't see a display that stays connected but shows
nothing, because Hyprland and the kernel report that the same as a working one.
The gentle wake exists because the drop before that silence is visible.

## Profile matching

Profiles are matched by hardware identity (make, model, serial) -- not connector name. This means your layout survives when monitors swap between `DP-1` and `DP-2` across reboots. Each profile is scored against the currently connected monitors:

| Condition | Points |
|---|---|
| Monitor enabled in profile and connected | +100 |
| Monitor disabled in profile but connected | +50 |
| Connected monitor not in the profile | −20 |
| Monitor enabled in profile but not connected | −30 |
| Monitor disabled in profile and not connected | −10 |

For automatic switching, a profile must enable at least one connected display and have a positive score. A partial match can provide the base for a temporary extended layout: unfamiliar displays are added unless the profile explicitly sets `disable_unknown_outputs: true`. Deliberately disabled known displays remain off. Missing saved displays remain allowed, so undocking can still restore the laptop layout.

Among eligible profiles, the highest score wins. Ties break alphabetically by profile name. A profile that mentions a monitor which is not plugged in pays for it either way, so the profile that describes exactly the connected displays beats a larger profile that happens to include them. The profiles tab shows every score, along with this breakdown for the selected profile.

Another physical unit of the same model may have a different serial number and therefore a different identity. Extension treats it as a new display without copying another unit's calibration.

On laptops, the daemon also reads lid state. UPower is optional, but recommended: with UPower available, lid changes arrive as D-Bus events and the daemon can react immediately. Without UPower, the daemon falls back to polling `/proc/acpi/button/lid/*/state` at `--lid-poll-interval`, which defaults to `1s` and is not available on every system. If neither source exists, lid-aware switching is disabled and monitor hotplug still works.

Lid state is not a separate profile type. Save the profile for the monitor setup you actually have attached. When the lid is closed and an external output has an awake, nonzero mode and remains enabled in the target profile, hyprmoncfg treats internal laptop-panel outputs like `eDP-1`, `LVDS-1`, or `DSI-1` as forced off for that apply. Modeless, disabled, sleeping, and synthetic fallback outputs do not qualify. Saved profiles are not rewritten. If workspace rules target the forced-off internal panel, those workspaces are moved to the first enabled external output in the selected profile.

{% include alert.html type="warning" title="Remove Throwaway Profiles" content="The daemon does not know which profiles are \"real\" and which were temporary experiments. Any profile with a positive hardware match can be eligible. An old throwaway profile with a high enough score can win over the one you actually want." %}

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

Raw monitor-event arrival, slow or failed compositor queries, and reconciliation duration are logged separately. Compare these timestamps to distinguish time spent before an event arrives from time spent querying or applying a profile. Query timing messages include the operation and elapsed time without adding monitor serial numbers.

{% include alert.html type="tip" title="Separate Matching From Applying" content="If you're not sure whether the daemon picked the wrong profile or failed to apply the right one, test the profile directly with <code>hyprmoncfg apply &lt;name&gt;</code>. If the layout looks correct, the problem is matching, not applying -- check the logs and your profile directory." %}

{% include alert.html type="important" title="Filing A Matching Bug" content="If you think the daemon selected the wrong profile, <a href=\"https://github.com/crmne/hyprmoncfg/issues/new\">open an issue</a> and include <strong>all</strong> profiles from <code>~/.config/hyprmoncfg/profiles/</code>, not just the one you expected to win. Matching depends on the full candidate set." %}
