---
title: TUI Walkthrough
description: The layout editor, display and color controls, save dialog, and workspace planner.
nav_order: 2
---

## Layout editor

At narrow widths, footer Apply/Save/Keys actions take priority over project links
and build information. The Workspaces planner includes Persistence: First per
display (default) or All assigned. Manual plans display Custom (per rule) and
preserve individual flags.

When you launch `hyprmoncfg`, you land on the layout tab. This is where you arrange your monitors and tune their settings. The screen is split into two panes:

- **Left**: a stage showing your monitors as draggable screens on a dotted field, positioned the way Hyprland currently sees them, with the selected display's **Hardware** facts directly below it
- **Right**: switchable **Display** and **Color** controls -- resolution, scale, position, transform, VRR, color management, and more

Display cards consistently show connector, model with whole-inch size (`32"`),
`3840x2160@144Hz`, `Scale 1.33x  Position 0,0`, and workspace IDs. The connector
and model sit at the top of the card with the workspace IDs as chips beside the
connector; the mode and the scale and position sit on the card's bottom rows.
The selected card has a heavy border. There are no arbitrary display numbers or
logical desktop dimensions. Small cards prioritize identity and workspaces.
Formatting never rounds the stored mode or scale. Lists outside the canvas,
such as the workspace plan, keep the `1, 2, 3` form.

The Hardware box shows connector, model, and maximum advertised
resolution, panel dimensions, type, and serial, all shown directly without a
More details action. It uses two columns when the stage is wide enough. In
narrow terminals the stage, Hardware, and the controls stack, and the stage
takes only the rows its arrangement needs, so the controls keep the rest. The TUI currently has no standalone on-screen Identify overlay; this
remains a panel capability, not a requirement to install Omarchy for the TUI.
Panel size shows a whole-inch diagonal and exact reported dimensions, such as
`32" (710x400mm)`, separately from the inspector's model name.

Drag monitors on the canvas to reposition them. The information and controls update in real time. When you need pixel-perfect placement, use the `Position X` and `Position Y` fields in **Display** instead of dragging.

While the TUI is open, it also refreshes live monitor state in the background. Plugging or unplugging a monitor, docking, undocking, or changing lid state reloads the editor so the canvas matches the current hardware.

The canvas only draws displays that are on and not mirroring another one. Off
and mirrored displays have separate bracketed rows above the geometry, showing
connector, state, and model. Click a row to inspect it; click **Enable** to turn an
off display on in the draft. Keyboard users can select it with `[` / `]` and press
`Space`. This does not apply the draft. Missing hardware says **Not connected**
without an Enable action. In crowded layouts, keyboard selection reveals hidden
rows without covering the active display cards.

A display that is on but has no mode shows nothing, so it gets a row too, marked
**No usable signal**, instead of an empty rectangle. When `hyprmoncfgd` runs a
display below its saved settings because it would not stay on at them, that
display's card says how it runs now, for example `running at 120 Hz without
VRR`. The saved profile is unchanged. See [Daemon Behavior](/daemon/#displays-that-wont-stay-on).

![Layout editor]({{ '/assets/images/screenshots/layout-dark.png' | relative_url }})
{: .screenshot }

### Main controls

| Key | Action |
|-----|--------|
| `1` `2` `3` | Switch tabs (layout, workspaces, profiles) |
| `a` | Preview the current draft or selected profile, then Keep or Revert |
| `s` | Save current draft as a named profile |
| `r` | Reset from live Hyprland state |
| `?` | Show every key for the tab you are on |
| `q` | Quit |

### Canvas controls

| Input | Action |
|-------|--------|
| Mouse drag | Move the selected monitor |
| Arrow keys | Move by 100px |
| `Shift` + arrows | Move by 10px |
| `Ctrl` + arrows | Move by 1px |
| `Alt` + arrows | Snap beside the nearest enabled monitor |
| `0` | Move the selected monitor to 0,0, where Hyprland's own `position = auto` starts |
| `[` `]` | Select the previous or next monitor |
| `Tab` `Shift+Tab` | Move between the canvas, **Display**, and **Color** |

Edits never make displays overlap, and the canvas follows the same placement rules as
the Omarchy panel:

- **Drag** a display and it follows the pointer; the stage holds still until
  you let go. On release it snaps to a neighbour's edge or alignment within
  about one canvas row. Dropped on top of another display, it moves to the
  nearest clear edge instead. If no clear spot exists it returns to where you
  picked it up.
- **Arrow keys**, **Position X/Y**, and `0` are exact: a move that would cover
  another display is refused and the top bar says which one.
- `Alt` + arrows place the selected monitor flush left, right, above, or below
  the nearest enabled monitor and center it on the other axis.
- A mode, scale, rotation, or Enabled change that would push a display into a
  neighbour is refused too. A layout that already overlapped stays editable so
  you can fix it.

### Display and Color controls

Press `Enter` on any **Display** or **Color** field to edit it:

- **Enabled**, **VRR**, **Color depth**, **SDR EOTF**, **WCG capability**, and **HDR capability** are choice rows, like the scale pills in Omarchy's own Display panel: every option is visible, `←` `→` or `h` `l` move one option and stop at the ends, `Enter` advances and wraps, and a click picks one directly. A row too narrow for its options shows the selected value and still steps
- **Mode** opens a scrollable picker with every supported resolution and refresh rate
- **Scale** is a row of pills like Omarchy's own scale row: 1x, 1.25x, 1.5x, 1.6x, 2x, and 3x (plus 4x on modes at least 5120 pixels wide), each moved up to the next scale that gives whole logical pixels on this display, so a preset can read 1.6x or 3.2x, and presets that land on the same scale show once. The same pills appear in the Omarchy panel. A current scale that is not a preset gets its own pill; one that is not sharp is marked ⚠ and never rewritten. `←` `→` step through every sharp scale from 1x to 4x, the list the panel uses. **More…**, `Enter`, or a click on the Scale label opens that full list, whose **Custom…** types any exact scale in a dialog that explains sharpness. Wide terminals wrap the pills; narrow ones keep one line and slide it, with ‹ and › marking more
- **Position X** and **Position Y** are typed in place on their row, so the stage stays visible; `←` `→` move by 10px and `Shift` + `←` `→` by 1px. Snapping stays on the canvas with `Alt` + arrows
- **Rotation** cycles with `←` `→` or opens a picker with Enter
- **Mirror** lets you mirror the selected monitor to any other connected display. For a crisp image, set the mirrored monitor's Mode to match the source resolution. If the resolutions don't match, Hyprland upscales the image, which looks blurry

The **Color** tab uses the same terminology as the Omarchy panel. **Color space / EOTF** combines the primaries and transfer function (for example, **BT.2020 + PQ (HDR)**). The picker shows descriptive labels but saves Hyprland's original values, such as `hdr`.

**SDR luminance scale** and **SDR saturation scale** are unitless SDR-to-HDR multipliers, not physical brightness controls. An omitted or zero multiplier uses the neutral value `1`. Black, white, peak, and frame-average luminance are measured in **cd/m²**. Display luminance and WCG/HDR capability fields override display metadata; leave them at their defaults to use EDID. Narrow terminals shorten the labels without changing their meaning.

## Keep or Revert

Every preview ends in a **Keep this layout?** (or **Keep this profile?**)
dialog. It names what is live, says when the previous layout returns, and
drains a countdown meter that ends in the seconds left, so the countdown does
not rely on color. When the terminal has rows to spare, a miniature of the
arrangement being kept sits under the meter. **[Revert]** and **[Keep]** are
buttons you can click; `Enter` or `y` keeps, `Esc` or `n` reverts. The daemon's
deadline is authoritative; the dialog only shows it. On short terminals the
dialog drops its spacing so both buttons stay on screen.

## Save dialog

Press `s` from the layout tab. You'll see a text input and the list of existing profiles.

- Type to filter existing profiles
- Arrow keys to select one (overwrites after confirmation)
- Type a new name and press `Enter` to create a fresh profile

![Save profile dialog]({{ '/assets/images/screenshots/save-profile-dark.png' | relative_url }})
{: .screenshot }

## Profiles

The third tab lists every saved profile and how well it fits the displays that are plugged in right now.

**Automatic profile selection** has its own compact box above the left-hand
profile list, matching its width. The details column starts at the top alongside it.
Click its On/Off control to change automatic matching. The **Saved Profiles** box
contains only the table and its actions: Preview, Edit, and Delete stay pinned
below the scrolling list and act on the highlighted row. Profile details remain
alongside the list, or below it in narrow terminals.

- **Match** is the profile's score against the connected hardware, the same score the daemon uses to pick a profile automatically. A dash means the profile has no display in common with what is connected
- **active** marks the profile your screens are already showing
- **best** marks the highest scoring profile -- the one the daemon would apply on the next hotplug

Selecting a profile fills the right side: its monitor arrangement on top, its details below. The details spell out the score as the arithmetic that produced it, so a surprising number is never a mystery, and they list what the canvas cannot draw -- displays the profile keeps off and displays that mirror another one. On the canvas, a display the profile expects but cannot find is outlined and labelled `not connected`.

![Profiles tab]({{ '/assets/images/screenshots/profiles-dark.png' | relative_url }})
{: .screenshot }

| Key | Action |
|-----|--------|
| `↑` `↓` | Select a profile |
| `Enter`, `a` | Preview the profile, even with automatic selection on |
| `l` | Load the profile into the layout editor |
| `e` | Edit the profile's post-apply command |
| `d` | Ask to delete the profile; `y` or **[Delete profile]** confirms, Enter, Esc, or **[Cancel]** cancels |
| `s` | Save the current draft |

**Post-apply command** is the final profile detail. Click its
**Edit command** button or its heading to edit it; `e` provides the same operation. Selection
uses highlighting without an extra arrow. The profile action context menu remains
a panel-only affordance for now; the TUI has visible **Preview**, **Edit**,
and **Delete** buttons acting on the highlighted profile, with keyboard shortcuts
listed in the footer and help.
Selection alone does not apply it. The Status column distinguishes the active or
best match from the row currently selected for inspection.

## Workspace planner

The second tab lets you distribute workspaces across monitors. **Strategy** is the first row: pick Off or one of three plans.

Sequential is preferred for new plans. When importing consecutive rules from one
display, the editor uses Sequential with groups of three and keeps the existing
workspace total and persistence. Saved profiles retain their explicit strategy,
including Interleaved; this default does not migrate existing profiles.

| Strategy | What it does | When to use it |
|----------|-------------|----------------|
| Off | Writes no workspace rules. The profile keeps its plan, so choosing a strategy again brings it back | Another tool manages workspaces, such as [virtual-desktops](virtual-desktops.md) or hyprsplit |
| `sequential` | Groups workspaces in chunks (e.g., 1-3 on monitor A, 4-6 on monitor B) | You think of each monitor as having "its own" workspaces |
| `interleave` | Round-robins workspaces across monitors (1 on A, 2 on B, 3 on A, ...) | You want next/previous workspace to alternate screens |
| `manual` | Shows every workspace as an assignment you can move between monitors | You need full control over exactly which workspace lives where |

You can also configure:

- **Max workspaces** -- how many workspaces to generate rules for
- **Group size** (sequential only) -- how many consecutive workspaces to assign to each monitor before moving to the next. With 2 monitors and a group size of 3, monitor A gets 1-3, monitor B gets 4-6, and so on
- **Monitor order** -- which monitor gets the first batch of generated workspaces. Applied assignments preserve this order when the editor reads the live configuration back, independently of the monitors' physical positions. Save the profile to reuse it later.
- **Workspace → display** (manual only) -- select a workspace and press `←` or `→` to assign it to a different monitor

While the strategy is Off, the other rows show `—` and the workspace plan is empty.

There is no fixed workspace or group-size limit. Select **Max workspaces** or **Group size** and press `Enter` to type an exact count; `←` and `→` still make one-step adjustments.

Long manual lists stay navigable: the mouse wheel scrolls three rows at a time, `Page Up` and `Page Down` move by a visible page, and `Home` and `End` jump to the first or last row. Only the visible assignment rows are rendered.

Switching from a generated strategy to `manual` starts with the plan already on screen, so you can adjust only the exceptional workspaces instead of rebuilding the whole layout. In manual mode, changing **Max workspaces** adds or removes numbered workspace assignments.

The right side previews the result twice: **Monitor Layout** paints the workspaces onto the monitors themselves as chips, and **Workspace Plan** below it lists which workspaces each monitor owns. Both update as you change the strategy, so you can see where workspace 1 lands before you save.

The workspace plan is stored inside each profile. When the daemon applies a profile, it applies workspace rules too -- layout and workspace assignment in one shot.

## Laptop lids

Internal laptop panels are marked as internal displays in the layout view. The TUI also shows the current lid state when it is available.

Profiles are still profiles for the attached monitor setup, not separate open-lid and closed-lid variants. The closed-lid policy only forces internal laptop panels off when a real external output already has a usable, awake mode and the target profile keeps it enabled. A modeless dock output, sleeping display, or synthetic fallback does not qualify. Workspace rules move away from a forced-off panel.

Interactive previews default to 30 seconds after verification. Confirming a saved
profile pauses automatic selection for the current display setup. There is no
need to turn automatic selection off before starting a preview.
