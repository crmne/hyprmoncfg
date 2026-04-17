---
title: Getting Started
description: Install hyprmoncfg and go from zero to a saved profile in two minutes.
nav_order: 1
---

## Prerequisites

- A running Hyprland session
- `hyprctl` in `PATH`
- A `source` line in your `hyprland.conf` that includes a `monitors.conf` file

That last point matters. Hyprland doesn't read `monitors.conf` on its own -- your main config has to tell it to. Most setups already have a line like this in `~/.config/hypr/hyprland.conf`:

```
source = ~/.config/hypr/monitors.conf
```

If yours doesn't, add one now. hyprmoncfg writes your monitor layout to `monitors.conf`, and Hyprland needs to know about it. If the `source` line is missing, hyprmoncfg will refuse to write and tell you exactly what's wrong.

You don't need to create `monitors.conf` yourself -- hyprmoncfg creates it when you apply your first profile.

Treat `monitors.conf` as generated output. hyprmoncfg rewrites it on every apply, so keep unrelated Hyprland settings in other sourced files.

If your config files live somewhere other than the defaults:

```bash
hyprmoncfg --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
```

## Install

### Arch Linux

Stable release from AUR:

```bash
yay -S hyprmoncfg
```

Latest `main` from AUR:

```bash
yay -S hyprmoncfg-git
```

The package installs:

- `hyprmoncfg` to launch the TUI or use the CLI
- `hyprmoncfgd` for automatic profile switching
- a user service unit at `/usr/lib/systemd/user/hyprmoncfgd.service`


### Void Linux

Add [Blackhole-vl](https://github.com/Event-Horizon-VL/blackhole-vl) (Unofficial Repo) to your package manager:

```bash
echo repository=https://raw.githubusercontent.com/Event-Horizon-VL/blackhole-vl/repository-x86_64 | sudo tee /etc/xbps.d/20-repository-extra.conf
```

Install package with:

```bash
sudo xbps-install -S hyprmoncfg
```

The package installs:

- `hyprmoncfg` to launch the TUI or use the CLI
- `hyprmoncfgd` for automatic profile switching

### Build from source

```bash
git clone https://github.com/crmne/hyprmoncfg.git
cd hyprmoncfg
go build -o bin/hyprmoncfg  ./cmd/hyprmoncfg
go build -o bin/hyprmoncfgd ./cmd/hyprmoncfgd
```

### Install to `~/.local/bin`

```bash
install -Dm755 bin/hyprmoncfg  ~/.local/bin/hyprmoncfg
install -Dm755 bin/hyprmoncfgd ~/.local/bin/hyprmoncfgd
```

## Your first profile

Launch the TUI:

```bash
hyprmoncfg
```

![Layout editor]({{ '/assets/images/screenshots/layout-dark.png' | relative_url }})
{: .screenshot }

You land on the layout tab. The left side shows your connected monitors as rectangles arranged the way Hyprland currently sees them. The right side is an inspector showing every property of the selected monitor -- resolution, scale, position, refresh rate.

Drag monitors on the canvas to rearrange them. Click on the inspector fields to change resolution, scale, or position. When the layout looks right:

1. Press `s` to save
2. Type a name like `desk` or `home-office`
3. Press `Enter`

That's it. Your monitor layout is now a named profile.

{% include alert.html type="important" title="Clean Up Before You Enable The Daemon" content="`hyprmoncfgd` scores **every** profile in `~/.config/hyprmoncfg/profiles/`. Old experiments, duplicate layouts, and half-finished saves are part of matching until you delete them." %}

Before you turn on automatic switching, make sure your profile library reflects real setups you actually want auto-applied:

- Save one profile for each real desk, dock, projector, or travel setup you use
- Delete throwaway profiles you created while experimenting
- Re-save the profile you actually use instead of keeping old variants around

## Apply from the command line

```bash
hyprmoncfg apply desk
```

Your monitors rearrange immediately, and a 10-second countdown starts. If the layout looks right, press any key to confirm. If something looks wrong, just wait -- it reverts automatically. This is the same safety mechanism you see on TVs and projectors when you change the resolution.

For scripts and automation, skip the countdown:

```bash
hyprmoncfg apply desk --confirm-timeout 0
```

## Start the daemon

The daemon watches for monitor changes and applies the best matching profile automatically. Set it up once and forget about it.

If you installed from AUR:

```bash
systemctl --user daemon-reload
systemctl --user enable --now hyprmoncfgd
```

If you installed from Blackhole-vl (Void Linux) add this line to your `~/.config/hypr/hyprland.conf`:

```bash
exec-once hyprmoncfgd
```

If you built from source and installed into `~/.local/bin`:

```bash
mkdir -p ~/.config/systemd/user
cp packaging/systemd/hyprmoncfgd.local.service ~/.config/systemd/user/hyprmoncfgd.service
systemctl --user daemon-reload
systemctl --user enable --now hyprmoncfgd
```

Now when you plug in a monitor, unplug one, dock your laptop, or close the lid, the daemon finds the profile that best matches your current hardware and applies it. No interaction needed.

If the daemon ever applies a layout you didn't expect, the most common cause is stale or duplicate profiles in `~/.config/hyprmoncfg/profiles/`. The daemon scores every profile it finds, not just the ones you remember saving. Delete old experiments, keep one profile per real setup, and the matching becomes predictable. See [Daemon Behavior](/daemon/) for the full scoring breakdown.

## Add profiles to your dotfiles

Profiles are plain JSON files stored in `~/.config/hyprmoncfg/profiles/`. Add the whole config directory to your dotfile manager and your layouts roam across every machine.

With [chezmoi](https://www.chezmoi.io/):

```bash
chezmoi add ~/.config/hyprmoncfg
```

Your desk at home, your laptop bag setup, your conference projector layout -- all versioned, all portable. The daemon on each machine picks the right profile based on what's actually plugged in.

You never commit `monitors.conf`. You commit your profiles. hyprmoncfg writes `monitors.conf` for you.

## Next steps

- [TUI Walkthrough](/tui/) -- learn the full editor interface
- [Daemon Behavior](/daemon/) -- understand how auto-switching works
- [Dotfiles Integration](/dotfiles/) -- set up profile portability
- [Command Reference](/commands/) -- every flag and subcommand
