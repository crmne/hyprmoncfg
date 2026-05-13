---
title: Dotfiles Integration
description: Version your monitor profiles and share them across machines.
nav_order: 4
---

## The idea

You have a desktop, a laptop, maybe a Raspberry Pi. Each one connects to different monitors. But your preferred layouts -- the positions, scales, workspace assignments -- those are yours. They should travel with you.

hyprmoncfg stores profiles as plain JSON files in `~/.config/hyprmoncfg/profiles/`. One file per profile. Add this directory to your dotfile manager and every machine gets your full profile library.

The daemon on each machine looks at what's actually plugged in and picks the right profile. Your desktop applies "desk". Your laptop at a conference applies "projector". Your Raspberry Pi applies "tv". Same dotfiles, different hardware, correct layout every time.

## What to commit (and what not to)

**Commit**: `~/.config/hyprmoncfg/` -- your profile library.

**Don't commit**: `~/.config/hypr/monitors.conf` or `~/.config/hypr/monitors.lua` -- this is generated output. hyprmoncfg rewrites the active target from the active profile. Committing it causes conflicts between machines with different monitors.

## chezmoi

Add the config directory:

```bash
chezmoi add ~/.config/hyprmoncfg
```

That's it. Your profiles are now versioned and will be deployed to every machine where you run `chezmoi apply`.

When you save a new profile or update an existing one, tell chezmoi to pick up the changes:

```bash
chezmoi re-add ~/.config/hyprmoncfg
```

(`re-add` updates files that chezmoi already tracks. You only need `add` for the initial setup.)

## Other dotfile managers

The same principle applies to any dotfile manager. The directory to track is:

```
~/.config/hyprmoncfg/
```

With GNU Stow, symlink the directory from your dotfiles repo. With yadm, just `yadm add ~/.config/hyprmoncfg`. With bare git repos, add the path to your tracking.

## A practical example

Say you speak at conferences. At home, you have a desk with an external monitor. On the road, you plug into whatever projector the venue provides.

1. At home, open the TUI, arrange your desk layout, save as `desk`
2. At a conference, plug into the projector, open the TUI, arrange, save as `conference-1080p`
3. At another venue with a 4K projector, save as `conference-4k`
4. Run `chezmoi re-add ~/.config/hyprmoncfg` after each new profile

Now the daemon handles it. Arrive at the venue, plug in, and the right profile is applied before you've even opened your slides. When you get home, dock the laptop, and your desk layout comes back instantly.

No manual config editing. No remembering which `monitor=` lines go where. Just plug in and go.
