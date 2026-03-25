---
title: TUI Walkthrough
description: The main TUI surfaces and the controls that matter.
---

## Layout editor

The layout tab is split into two panes:

- left: a spatial monitor canvas
- right: a selected-monitor inspector

Drag monitors on the canvas to reposition them. Change `Mode` to change their size. Use `Position X` and `Position Y` when you need exact logical coordinates.

![Layout editor]({{ '/assets/images/screenshots/layout.png' | relative_url }})
{: .screenshot }

### Main controls

- `1`, `2`, `3`: switch tabs
- `a`: apply current draft
- `s`: save current draft as a profile
- `r`: reset from live Hyprland state
- `q`: quit

### Canvas controls

- drag with the mouse to move monitors
- arrows move by `100px`
- `Shift+arrows` move by `10px`
- `Ctrl+arrows` move by `1px`
- snap hints show while moving, but keyboard movement is not forced into snap positions

### Inspector controls

- `Enter` opens pickers or numeric editors
- `Mode` opens a scrollable selector
- `Scale`, `Position X`, and `Position Y` accept typed values

## Save dialog

The save flow is built around one input plus the existing profile list.

- type to filter existing names
- arrow keys select an existing profile
- `Enter` creates a new profile or asks before overwrite

![Save profile dialog]({{ '/assets/images/screenshots/save-profile.png' | relative_url }})
{: .screenshot }

## Workspace planner

The workspace planner supports:

- `manual`
- `sequential`
- `interleave`

You can configure:

- whether workspace rules are enabled
- max workspace count
- group size
- monitor ordering

The workspace plan is stored inside each profile, so the daemon can apply layout and workspace placement together.
