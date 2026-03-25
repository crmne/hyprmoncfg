---
title: Configuration Files
description: Where hyprmoncfg stores profiles and what it writes to Hyprland.
---

## Profile storage

By default, `hyprmoncfg` stores machine-owned profile files in:

```text
~/.config/hyprmoncfg/profiles/*.json
```

JSON is used here deliberately:

- the files are written by the program, not designed for hand-authoring
- encoding and decoding are straightforward in Go
- diffs stay predictable

## Hyprland targets

Default apply target:

```text
~/.config/hypr/monitors.conf
```

Default root config used for source verification:

```text
~/.config/hypr/hyprland.conf
```

Override either path when your setup is non-standard:

```bash
hyprmoncfg --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
hyprmoncfgd --monitors-conf /path/to/monitors.conf --hypr-config /path/to/hyprland.conf
```

## Why the source check exists

Applying by rewriting `monitors.conf` only works if Hyprland is actually reading that file. `hyprmoncfg` checks that relationship before writing so it does not silently update the wrong config file.
