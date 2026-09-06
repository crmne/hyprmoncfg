<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/images/logotype_dark.svg">
  <img src="docs/assets/images/logotype.svg" alt="hyprmoncfg" height="120">
</picture>

<strong>Create multi-monitor layouts for Hyprland.</strong><br>
Arrange visually. Save each setup. Switch automatically on hotplug and lid events.

[![GitHub Release](https://img.shields.io/github/v/release/crmne/hyprmoncfg)](https://github.com/crmne/hyprmoncfg/releases)
[![AUR](https://img.shields.io/aur/version/hyprmoncfg)](https://aur.archlinux.org/packages/hyprmoncfg)
[![CI](https://github.com/crmne/hyprmoncfg/actions/workflows/ci.yml/badge.svg)](https://github.com/crmne/hyprmoncfg/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

<a href="https://terminaltrove.com/hyprmoncfg/">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/images/terminal-trove-tool-of-the-week-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/images/terminal-trove-tool-of-the-week-light.svg">
    <img alt="Terminal Trove Tool of the Week" src="docs/assets/images/terminal-trove-tool-of-the-week-light.svg" height="48">
  </picture>
</a>

</div>

---

hyprmoncfg is a visual multi-monitor layout editor and automatic profile switcher for Hyprland. Drag displays into place, save each setup as a hardware-aware profile, and let the daemon apply the right one when monitors or your laptop lid change.

![hyprmoncfg demo](docs/assets/images/demo.gif)

## What you get

- **Spatial layout editor** -- drag monitors on a canvas and tune mode, scale, VRR, mirror, transform, and exact position
- **Named profiles** -- save setups like `desk`, `conference`, or `home-office`
- **Hardware-identity matching** -- profiles follow monitor make, model, and serial instead of unstable connector names
- **Hotplug and lid-aware daemon** -- apply the right profile automatically when monitors change or the laptop lid closes
- **Workspace planner** -- assign workspaces across monitors with sequential, interleave, or manual strategies
- **Safe apply with revert** -- reload Hyprland, verify the result, and revert unless you confirm
- **One-writer IPC** -- when the daemon is running, the TUI, CLI, and desktop panels send changes through it instead of racing over config files
- **Include-chain verification** -- refuse to write generated monitor config that Hyprland is not reading
- **Hyprland 0.55 Lua config support** -- write Lua automatically when `hyprland.lua` is active, while preserving legacy `.conf` setups
- **One hard runtime dependency** -- Hyprland; UPower is optional for immediate lid events

## Install

Arch Linux:

```bash
yay -S hyprmoncfg-bin
# or
yay -S hyprmoncfg-git
```

Fedora COPR:

```bash
sudo dnf copr enable paolino/hyprmoncfg
sudo dnf install hyprmoncfg
```

Nix / NixOS:

```bash
nix run nixpkgs#hyprmoncfg
nix profile install nixpkgs#hyprmoncfg
```

Gentoo GURU:

```bash
sudo eselect repository enable guru
sudo emaint sync -r guru
sudo emerge gui-apps/hyprmoncfg
```

Void Linux, via the unofficial [Blackhole-vl](https://github.com/Event-Horizon-VL/blackhole-vl) repo:

```bash
printf 'repository=https://mirror.black-hole.dev/%s/\n' "$(uname -m)" | sudo tee /etc/xbps.d/00-repository-blackhole.conf
sudo xbps-install -S
sudo xbps-install -S hyprland hyprmoncfg
```

Build from source:

```bash
git clone https://github.com/crmne/hyprmoncfg.git
cd hyprmoncfg
go build -o bin/hyprmoncfg  ./cmd/hyprmoncfg
go build -o bin/hyprmoncfgd ./cmd/hyprmoncfgd
install -Dm755 bin/hyprmoncfg  ~/.local/bin/hyprmoncfg
install -Dm755 bin/hyprmoncfgd ~/.local/bin/hyprmoncfgd
```

Distro packagers should use [PACKAGING.md](PACKAGING.md).

## Configure Hyprland

hyprmoncfg writes `~/.config/hypr/hyprmoncfg-monitors.lua` (or `.conf` on legacy configs), a file it creates and owns, and adds one line at the end of your root Hyprland config to load it. Loading last is what makes an applied layout final: any monitor rule read afterwards would override it. Your own `monitors.conf` or `monitors.lua` is never replaced. Run `hyprmoncfg doctor` to check the load order at any time.

## Create your first profile

```bash
hyprmoncfg
```

Drag monitors into place, press `s`, type a profile name like `desk`, and press `Enter`.

Apply it later from the CLI:

```bash
hyprmoncfg apply desk
```

## Enable automatic switching

AUR, Fedora COPR, Nixpkgs, and Gentoo GURU:

```bash
systemctl --user daemon-reload
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

The daemon scores every profile in `~/.config/hyprmoncfg/profiles/`, so delete throwaway profiles before relying on automatic switching.

On Omarchy versions that launch `omarchy-hyprland-monitor-watch`, `hyprmoncfgd` stops that exact transient user scope while it owns monitor profiles and restores the watcher when the daemon exits during a live Hyprland session. Generated configuration used without the daemon cannot provide this runtime ownership; static-config users must disable the Omarchy watcher separately.

Omarchy's lock/wake script reads `~/.config/hypr/monitors.lua` directly, before its remembered scale. To prevent it from resetting the laptop's scale and position, hyprmoncfg keeps a marked, connector-specific wake rule at the top of that file, alongside Omarchy's remembered scale. Your existing rules and defaults stay intact. Canceling a preview restores the previous wake settings; turning management off removes the marked block. Read-only dotfiles are left alone with a diagnostic. Omarchy still controls lid recovery and uses its preferred mode when re-enabling an entirely disabled panel; hyprmoncfg then restores the full profile.

When the daemon is running, it is the canonical monitor-config writer. The TUI, CLI, and desktop integrations use its versioned Unix-socket IPC; when it is absent, the TUI and CLI keep working through the same core engine in direct mode. A profile selected interactively stays selected until the next monitor hotplug or lid change, when automatic matching resumes.

## Omarchy Quattro panel

On Omarchy Quattro, [hyprmoncfg: Multi-Monitor Manager for Omarchy](https://omarchyplugins.com/plugin.html?id=crmne.hyprmoncfg) lets you create multi-monitor layouts for Hyprland in a visual editor and switch them automatically on hotplug and lid events. The panel shows your live layout and active profile right in the bar:

![hyprmoncfg panel for Omarchy Quattro](https://raw.githubusercontent.com/crmne/omarchy-hyprmoncfg/main/preview.png)

Get it from the [Omarchy Plugins marketplace](https://omarchyplugins.com/plugin.html?id=crmne.hyprmoncfg), or install it directly:

```bash
omarchy plugin add https://github.com/crmne/omarchy-hyprmoncfg.git --enable
```

If hyprmoncfg is not installed yet, open the panel and choose **Install hyprmoncfg**. It installs the stable AUR package, starts the daemon, and opens the layout editor so you can arrange and save your first profile. See the [Omarchy Quattro panel guide](https://hyprmoncfg.dev/omarchy/) for the full workflow.

## Screenshots

hyprmoncfg adapts to your theme. Here are some examples:

| Layout editor | Save dialog |
| --- | --- |
| ![Layout editor](docs/assets/images/screenshots/layout-dark.png) | ![Save profile dialog](docs/assets/images/screenshots/save-profile-dark.png) |

## Why it exists

Configuring monitors in Hyprland means writing `monitor=` lines by hand. A 4K display at 1.33333x scale is effectively 2880x1620 pixels, so the monitor next to it needs to start at x=2880. Vertically centering a 1080p panel against it means doing division in your head, reloading, noticing the layout is wrong, and editing again.

It gets worse when setups change:

- **No visual editor.** You write `monitor=` lines by hand and hope the coordinates are right.
- **No profiles.** Desk, projector, travel, and docked setups all need different layouts.
- **No automatic switching.** Hotplug a monitor and Hyprland guesses again.
- **Connector names are unstable.** `DP-1` and `DP-2` can swap between boots.
- **Some tools pull in too much.** Python, GTK, and GObject introspection are a lot of stack just to move a rectangle.

## How it works

hyprmoncfg ships two binaries:

| | |
|---|---|
| `hyprmoncfg` | TUI + CLI for layout editing, profile management, and workspace planning |
| `hyprmoncfgd` | Background daemon that auto-applies the best matching profile on hotplug and lid changes |

Both use the same apply engine:

```bash
write monitors.conf -> reload Hyprland -> verify live state -> confirm or revert
```

There is no separate best-effort daemon path. If the TUI can apply a profile correctly, the daemon uses the same machinery.

## Dotfiles integration

Profiles live in `~/.config/hyprmoncfg/profiles/`. Each profile has a canonical JSON file plus generated `.conf` and `.lua` sidecars you can keep as plain Hyprland snippets if you stop using hyprmoncfg. Add the directory to your dotfile manager and your layouts roam across every machine you own.

With [chezmoi](https://www.chezmoi.io/):

```bash
chezmoi add ~/.config/hyprmoncfg
```

Now your desk at home, your laptop on the road, and your Raspberry Pi in the closet all share the same profile library. The daemon picks the right one based on what's actually plugged in.

You don't commit the generated `~/.config/hypr/hyprmoncfg-monitors.{conf,lua}`. You commit your profiles. The tool writes the generated monitor config for you.

## How it compares

| | hyprmoncfg | Monique | HyprDynamicMonitors | HyprMon | nwg-displays | kanshi |
|---|---|---|---|---|---|---|
| GUI or TUI | TUI | GUI | TUI | TUI | GUI | CLI |
| Spatial layout editor | Yes | Yes | Partial | Yes | Yes | No |
| Drag-and-drop | Yes | Yes | No | Yes | Yes | No |
| Snapping | Yes | Not documented | No | Yes | Yes | No |
| Profiles | Yes | Yes | Yes | Yes | No | Yes |
| Auto-switching daemon | Yes | Yes | Yes | No (roadmap) | No | Yes |
| Workspace planning | Yes | Yes | No | No | Basic | No |
| Mirror support | Yes | Yes | Yes | Yes | Yes | No |
| Safe apply with revert | Yes | Yes | No | Partial (manual rollback) | No | No |
| Hyprland 0.55 Lua config | Yes | No | No | No | Yes | N/A |
| Include-chain verification | Yes | No | No | No | No | No |
| Additional runtime dependencies | None | Python + GTK4 + libadwaita | UPower, D-Bus | None | Python + GTK3 | None |

## Docs

Full documentation at **[hyprmoncfg.dev](https://hyprmoncfg.dev)**.

## Development

Install the pre-commit hook to run CI checks locally before each commit:

```bash
ln -sf "$(pwd)/scripts/pre-commit" .git/hooks/pre-commit
```

The hook runs `go mod tidy`, `go vet`, `go test`, and `go build`.

Regenerate demo videos and screenshots:

```bash
./scripts/capture_media.sh
```

The media scripts use the installed `hyprmoncfg` from `PATH`.

Regenerate only the GIF and MP4 demo:

```bash
./scripts/capture_demo.sh
```

Regenerate only screenshots:

```bash
./scripts/capture_screenshots.sh
```

## License

MIT
