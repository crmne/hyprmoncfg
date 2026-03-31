---
layout: home
title: Home
description: Hyprland monitor configuration that actually works.
permalink: /
hero:
  name: hyprmoncfg
  text: Stop editing Hyprland monitor lines by hand
  tagline: A spatial layout editor, named profiles, automatic hotplug switching, and workspace planning. From the terminal. No Python, no GTK, no nonsense.
  actions:
    - theme: brand
      text: What is hyprmoncfg?
      link: /what-is-hyprmoncfg/
    - theme: alt
      text: Guide
      link: /getting-started/
    - theme: alt
      text: GitHub
      link: https://github.com/crmne/hyprmoncfg
  image:
    src: /assets/images/demo.gif
    alt: hyprmoncfg demo
    width: 1400
    height: 800
features:
  - icon: 🖥️
    title: Spatial Layout Editor
    details: Drag monitors on a canvas, tweak mode, scale, VRR, mirror, and exact position in the inspector. See the result before you apply it.
  - icon: 🔌
    title: Hotplug-Aware Daemon
    details: Plug in a monitor and walk away. The daemon scores your saved profiles against connected hardware and applies the best match automatically.
  - icon: 🔁
    title: Safe Apply with Revert
    details: Every apply writes monitors.conf atomically, reloads Hyprland, and verifies the result. A 10-second confirmation window means you never get locked out.
  - icon: 🗂️
    title: Workspace Planning
    details: Assign workspaces across monitors with sequential, interleave, or manual strategies. Stored inside each profile, applied together with the layout.
---
