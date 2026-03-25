---
layout: home
title: Home
description: Terminal-first monitor configuration for Hyprland.
permalink: /
hero:
  name: hyprmoncfg
  text: Hyprland monitor configuration from the terminal
  tagline: Profiles, hotplug auto-switching, workspace planning, and safe apply or revert without a Python runtime.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started/
    - theme: alt
      text: Command Reference
      link: /commands/
    - theme: alt
      text: GitHub
      link: https://github.com/crmne/hyprmoncfg
  image:
    src: /assets/images/screenshots/layout.png
    alt: hyprmoncfg layout editor
    width: 2000
    height: 1306
features:
  - icon: 🖥️
    title: Spatial TUI Layout Editor
    details: Drag monitors on a layout canvas, inspect them on the right, and edit mode, scale, VRR, transform, and exact position.
  - icon: 🔁
    title: Safe Apply and Revert
    details: Applying a profile writes monitors.conf, reloads Hyprland, verifies the result, and supports confirm-or-revert when you want a safety net.
  - icon: 🔌
    title: Hotplug-Aware Daemon
    details: hyprmoncfgd listens to Hyprland monitor events, debounces noisy plug cycles, and applies the best matching saved profile.
  - icon: 🗂️
    title: Workspace Planning
    details: Configure sequential, interleave, or manual workspace placement across one or more monitors from the same terminal UI.
---

`hyprmoncfg` is built for Hyprland users who want Monique-like monitor management without a Python runtime dependency and without fragile hotplug behavior.

It is terminal-first, but it is not CLI-only. The TUI has a real layout canvas, picker dialogs, profile management, a workspace planner, and the same apply engine used by the daemon.

## What it does differently

- Refuses to write the wrong `monitors.conf`: the apply path verifies that the configured file is actually sourced by `hyprland.conf` before it touches anything.
- Uses the same apply engine in the UI and in the daemon: write config, reload Hyprland, re-read monitor state, verify.
- Stores profiles as machine-owned JSON for deterministic saves and robust matching by monitor hardware identity.

## Screenshots

<div class="screenshot-grid">
  <a class="screenshot-card" href="{{ '/assets/images/screenshots/layout.png' | relative_url }}">
    <img class="screenshot" src="{{ '/assets/images/screenshots/layout.png' | relative_url }}" alt="Layout editor screenshot">
    <span>Layout editor with spatial monitor cards and live inspector.</span>
  </a>
  <a class="screenshot-card" href="{{ '/assets/images/screenshots/save-profile.png' | relative_url }}">
    <img class="screenshot" src="{{ '/assets/images/screenshots/save-profile.png' | relative_url }}" alt="Save profile dialog screenshot">
    <span>Save dialog with existing profile filtering and overwrite flow.</span>
  </a>
</div>

Start with [Getting Started](/getting-started/), then review the [command reference](/commands/) if you want to integrate it into scripts or systemd.
