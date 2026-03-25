---
title: Getting Started
description: Install hyprmoncfg, launch the TUI, and wire it into your Hyprland config.
---

## Prerequisites

- A running Hyprland session
- `hyprctl` in `PATH`
- A monitor config file that Hyprland actually sources

By default, `hyprmoncfg` targets `~/.config/hypr/monitors.conf` and verifies that `~/.config/hypr/hyprland.conf` sources it before applying.

If your setup is different, use:

```bash
hyprmoncfg --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
```

## Install

<div class="install-grid">
  <div class="install-card">
    <h3>Build from source</h3>
    <p>Good for trying the latest version directly from the repo.</p>
  </div>
  <div class="install-card">
    <h3>Local user install</h3>
    <p>Good when you want the binaries in <code>~/.local/bin</code>.</p>
  </div>
  <div class="install-card">
    <h3>Arch packaging</h3>
    <p>Use the packaged recipes under <code>packaging/arch</code>.</p>
  </div>
</div>

### Build

```bash
go build -o bin/hyprmoncfg ./cmd/hyprmoncfg
go build -o bin/hyprmoncfgd ./cmd/hyprmoncfgd
```

### Install to `~/.local/bin`

```bash
install -Dm755 bin/hyprmoncfg ~/.local/bin/hyprmoncfg
install -Dm755 bin/hyprmoncfgd ~/.local/bin/hyprmoncfgd
```

## Launch the TUI

```bash
hyprmoncfg
```

The default landing screen is the layout editor.

![Layout editor]({{ '/assets/images/screenshots/layout.png' | relative_url }})
{: .screenshot }

## Save and apply a profile

```bash
hyprmoncfg save desk
hyprmoncfg apply desk
```

For non-interactive scripts you can disable the confirm timer:

```bash
hyprmoncfg apply desk --confirm-timeout 0
```

## Start the daemon

```bash
systemctl --user enable --now hyprmoncfgd
```

If you install manually, copy the local unit first:

```bash
mkdir -p ~/.config/systemd/user
cp packaging/systemd/hyprmoncfgd.local.service ~/.config/systemd/user/hyprmoncfgd.service
systemctl --user daemon-reload
systemctl --user enable --now hyprmoncfgd
```
