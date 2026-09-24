---
layout: home
title: hyprmoncfg
description: Create multi-monitor layouts for Hyprland and switch them automatically on hotplug and lid events.
permalink: /
hero:
  name: hyprmoncfg
  text: Create multi-monitor layouts for Hyprland
  tagline: Arrange visually. Save each setup. Switch automatically on hotplug and lid events.
  actions:
    - theme: brand
      text: Install hyprmoncfg
      link: /getting-started/
    - theme: alt
      text: See the editor
      link: /what-is-hyprmoncfg/#screenshots
    - theme: alt
      text: GitHub
      link: https://github.com/crmne/hyprmoncfg
    - theme: terminal-trove
      text: >-
        <img class="theme-image light terminal-trove-badge-image" src="/assets/images/terminal-trove-tool-of-the-week-light.svg" alt="Terminal Trove Tool of the Week" width="220" height="58"><img class="theme-image dark terminal-trove-badge-image" src="/assets/images/terminal-trove-tool-of-the-week-dark.svg" alt="Terminal Trove Tool of the Week" width="220" height="58">
      link: https://terminaltrove.com/hyprmoncfg/
  image:
    src: /assets/images/screenshots/layout-dark.png
    alt: hyprmoncfg 1.19 layout editor with sample displays
    width: 1924
    height: 1084
features:
  - icon: 🖥️
    title: Visual Layout Editor
    details: Drag monitors on a canvas. Tune display and color settings. Preview and apply.
  - icon: 🧩
    title: Hardware-Aware Profiles
    details: Save every setup once. Profiles follow monitor make, model, and serial instead of unstable connector names.
    link: /getting-started/#create-your-first-profile
    link_text: Create your first profile
  - icon: 🔌
    title: Automatic Switching
    details: Plug in a monitor, close the lid, or undock. hyprmoncfg picks and applies the best profile.
  - icon: 🔁
    title: Safe Apply with Revert
    details: Preview writes the generated monitor config atomically, reloads Hyprland, and verifies the result. You have 30 seconds by default to confirm; unconfirmed previews roll back.
  - icon: 🗂️
    title: Workspace Planning
    details: Assign workspaces with sequential, interleave, or manual strategies and apply them together with the monitor layout.
  - icon: 🟧
    title: Omarchy Quattro Panel
    details: See your live layout and active profile in the Omarchy bar. Open the editor or hand display management back to Omarchy.
    link: /omarchy/
    link_text: Install the panel
---
