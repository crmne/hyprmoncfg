# hyprmoncfg

**Hyprland monitor configuration that actually works.**

You know the drill. You plug in a monitor. Nothing happens the way you want. You open `hyprland.conf`, squint at coordinate math, guess at `monitor=` lines, reload, realize the positions are wrong, edit again. You go to a conference, plug into a projector, and start the whole dance over.

hyprmoncfg fixes this.

Open a terminal. See your monitors laid out spatially. Drag them where you want. Save the layout as a profile. Next time you plug in the same monitors, the daemon applies it automatically.

Two binaries. Zero runtime dependencies. Runs over SSH. No Python, no GTK, no D-Bus.

![hyprmoncfg layout editor](docs/assets/images/screenshots/layout.png)

## The problem with Hyprland monitor configuration

Configuring monitors in Hyprland is painful:

- **No visual editor.** You write `monitor=` lines by hand and pray the coordinates are right.
- **No profiles.** Unplug your laptop from your desk, plug into a projector at a conference, and you're manually editing config files backstage.
- **No automatic switching.** Hotplug a monitor and Hyprland does its best guess. Your careful layout? Gone.
- **Connector names are unstable.** `DP-1` and `DP-2` swap randomly between boots. Workspace bindings break.
- **Existing tools pull in the world.** Python runtimes, GTK libraries, GObject introspection. Just to move a rectangle on a screen.

## The solution

hyprmoncfg ships two binaries:

| | |
|---|---|
| `hyprmoncfg` | TUI + CLI for layout editing, profile management, and workspace planning |
| `hyprmoncfgd` | Background daemon that auto-applies the best matching profile on hotplug |

Both use the same apply engine: write `monitors.conf` atomically, reload Hyprland, verify the result, revert if anything is wrong.

## Features

- **Spatial layout editor** -- drag monitors on a canvas, see them move in real time
- **Per-monitor inspector** -- mode, scale, VRR, transform, exact position
- **Named profiles** -- save "desk", "conference", "home-office", switch between them instantly
- **Hardware-identity matching** -- profiles follow your monitors, not connector names
- **Hotplug-aware daemon** -- plug in, walk away, the right profile is applied automatically
- **Workspace planner** -- sequential, interleave, or manual workspace placement across monitors
- **Safe apply with revert** -- a 10-second confirmation window so you never get locked out
- **Source-chain verification** -- refuses to write a `monitors.conf` that Hyprland isn't even reading
- **Zero runtime dependencies** -- compiled Go, statically linked, nothing to install

## Screenshots

| Layout editor | Save dialog |
| --- | --- |
| ![Layout editor](docs/assets/images/screenshots/layout.png) | ![Save profile dialog](docs/assets/images/screenshots/save-profile.png) |

## Quick start

Arch Linux:

```bash
yay -S hyprmoncfg
```

For the latest `main` branch:

```bash
yay -S hyprmoncfg-git
```

Build from source:

```bash
go build -o bin/hyprmoncfg ./cmd/hyprmoncfg
go build -o bin/hyprmoncfgd ./cmd/hyprmoncfgd
```

Install:

```bash
install -Dm755 bin/hyprmoncfg  ~/.local/bin/hyprmoncfg
install -Dm755 bin/hyprmoncfgd ~/.local/bin/hyprmoncfgd
```

Use:

```bash
hyprmoncfg                 # open the TUI
hyprmoncfg save desk       # save current layout as "desk"
hyprmoncfg apply desk      # apply it later
```

Start the daemon after an AUR install:

```bash
systemctl --user daemon-reload
systemctl --user enable --now hyprmoncfgd
```

If you installed manually into `~/.local/bin`, copy the local unit first:

```bash
mkdir -p ~/.config/systemd/user
cp packaging/systemd/hyprmoncfgd.local.service ~/.config/systemd/user/hyprmoncfgd.service
systemctl --user daemon-reload
systemctl --user enable --now hyprmoncfgd
```

## Dotfiles integration

Profiles live in `~/.config/hyprmoncfg/profiles/`. They're plain JSON files, one per profile. Add the directory to your dotfile manager and your layouts roam across every machine you own.

With [chezmoi](https://www.chezmoi.io/):

```bash
chezmoi add ~/.config/hyprmoncfg
```

Now your desk at home, your laptop on the road, and your Raspberry Pi in the closet all share the same profile library. The daemon picks the right one based on what's actually plugged in.

You don't commit `monitors.conf`. You commit your profiles. The tool writes `monitors.conf` for you.

## How it compares

| | hyprmoncfg | Monique | HyprDynamicMonitors | HyprMon | nwg-displays | kanshi |
|---|---|---|---|---|---|---|
| Spatial layout editor | Yes | Yes (GTK4) | Partial (TUI) | Yes | Yes (GTK3) | No |
| Drag-and-drop | Yes | Yes | No | Yes | Yes | No |
| Snapping | Yes | Not documented | No | Yes | Yes | No |
| Profiles | Yes | Yes | Yes | Yes | No | Yes |
| Auto-switching daemon | Yes | Yes | Yes | No (roadmap) | No | Yes |
| Workspace planning | Yes | Yes | No | No | Basic | No |
| Safe apply with revert | Yes | Yes | No | Partial (manual rollback) | No | No |
| Source-chain verification | Yes | No | No | No | No | No |
| Works over SSH | Yes | No | No | No | No | N/A |
| Runtime dependencies | None | Python + GTK4 + libadwaita | UPower, D-Bus | None | Python + GTK3 | None |

## Docs

Full documentation at **[crmne.github.io/hyprmoncfg](https://crmne.github.io/hyprmoncfg/)**.

## Development

```bash
go test ./...
go vet ./...
```

Regenerate screenshots:

```bash
./scripts/capture_screenshots.sh
```

## License

MIT
