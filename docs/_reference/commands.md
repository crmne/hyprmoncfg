---
title: Commands and Flags
description: CLI and daemon command reference.
---

## `hyprmoncfg`

```bash
hyprmoncfg                 # open the TUI
hyprmoncfg tui             # explicit TUI entrypoint
hyprmoncfg monitors        # list current outputs
hyprmoncfg profiles        # list saved profiles
hyprmoncfg save desk       # save current state as a profile
hyprmoncfg apply desk      # apply a saved profile
hyprmoncfg delete desk     # delete a saved profile
hyprmoncfg version         # build metadata
```

### Common flags

```bash
--config-dir <path>
--monitors-conf <path>
--hypr-config <path>
```

### Apply flags

```bash
hyprmoncfg apply desk --confirm-timeout 10
hyprmoncfg apply desk --confirm-timeout 0
```

`--confirm-timeout 0` disables the revert timer.

## `hyprmoncfgd`

```bash
hyprmoncfgd
hyprmoncfgd version
```

### Daemon flags

```bash
--config-dir <path>
--monitors-conf <path>
--hypr-config <path>
--profile <name>
--debounce 1200ms
--poll-interval 5s
--quiet
```

## Exit behavior

- CLI commands return non-zero on failed Hyprland queries, invalid layout, missing profiles, or source-chain verification failures.
- Apply returns an error before writing anything if the configured monitor file is not sourced by the configured Hyprland root config.
