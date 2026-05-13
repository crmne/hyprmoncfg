---
layout: home
title: hyprmoncfg
description: Hyprland monitor configuration that actually works.
permalink: /
hero:
  name: hyprmoncfg
  text: Arrange Hyprland monitors without doing coordinate math
  tagline: Drag displays into place, save hardware-aware profiles, and let hyprmoncfg switch them automatically when monitors or your laptop lid change.
  actions:
    - theme: brand
      text: Install hyprmoncfg
      link: /getting-started/
    - theme: alt
      text: Watch demo
      link: /what-is-hyprmoncfg/#demo
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
    details: Drag monitors on a canvas, edit mode, scale, VRR, mirror, and position in the inspector, then preview the result before applying.
  - icon: 🔌
    title: Hotplug and Lid-Aware Daemon
    details: Save profiles for your real setups. The daemon picks the best match when monitors change or the laptop lid closes.
  - icon: 🔁
    title: Safe Apply with Revert
    details: Every apply writes the generated monitor config atomically, reloads Hyprland, and verifies the result. A 10-second confirmation window means you never get locked out.
  - icon: 🗂️
    title: Workspace Planning
    details: Assign workspaces with sequential, interleave, or manual strategies and apply them together with the monitor layout.
---
