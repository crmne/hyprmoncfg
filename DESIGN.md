# hyprmoncfg product and interaction design

Status: proposed direction, 2026-09-22. This document records the requested
product contract and implementation priorities. It does **not** claim that
unshipped behavior already exists. The [baseline review](docs/design-review-2026-09-22.md)
distinguishes released behavior, reported problems, and proposed changes.

This is the canonical shared design for the daemon, CLI, TUI, and
[Omarchy panel](https://github.com/crmne/omarchy-hyprmoncfg). The panel's
[DESIGN.md](https://github.com/crmne/omarchy-hyprmoncfg/blob/main/DESIGN.md)
specifies its graphical presentation. Behavioral changes start here, then update
both frontends and the IPC reference together. A design decision is not permission
to bundle the whole roadmap into an unrelated fix.

## Product promise

Connect a display and get a usable picture. Arrange it once and remember the
arrangement when desired. Always make it easy to understand and recover what is
on screen, including on stage, on a small laptop, and after sleep.

Profiles are optional named arrangements, not a prerequisite for working displays.
Keep multiple profiles for the same hardware: work, gaming, presenting, and
different seating positions are valid uses. Automatic switching should need no
configuration for a first connection, and should explain its decisions when asked.

The TUI and panel are two interfaces to one product. They share terminology,
pages, operations, defaults, validation, and results. They use native controls:
keyboard navigation and terminal forms in the TUI; pointer controls, menus,
dragging, and accessible focus in the panel. Pixel identity is not the goal.

## Shared mental model

Keep these states distinct in the backend and in user-facing copy:

| Concept | Meaning | Example presentation |
| --- | --- | --- |
| Saved profile | Deliberate, persistent named arrangement | `Laptop` |
| Live layout | What the compositor is currently driving | `Laptop + new display` |
| Automatic draft | Live layout extended for an unfamiliar setup; saved base untouched | `Unsaved setup` |
| Editor draft | Unapplied edits owned by an editor | `Changes not applied` |
| Preview | Applied transaction awaiting a person, with a rollback deadline | `Keep these changes? 30s` |
| Current profile | Saved layout whose effective state is on screen | `Using Laptop` |
| Recommended profile | Candidate the selection policy recommends | `Preferred for these displays` |
| Manual choice | Confirmed session override for the present hardware set | `Using Presentation until displays change` |

An automatic draft is not an interactive preview. It must not turn the newly
connected screen off after an unattended confirmation timeout. Confirmed editor
changes can be used temporarily without requiring a new profile name.

Connected, enabled, usable, sleeping, mirrored, and disconnected are distinct.
An enabled output reporting a zero-size mode is not a working screen. A sleeping
screen is not automatically broken. A mirrored output remains a connected display
even though it does not own a separate desktop rectangle.

## Connecting and disconnecting displays

### Selection and automatic extension

Proposed selection order, evaluated by the backend after the hardware settles:

1. Honor an explicit forced profile or a still-valid manual choice, subject to
   display recovery and its explicit unknown-display policy. A normal manual
   choice expires when the hardware set changes; it cannot silently pin a
   laptop-only layout through a new projector connection.
2. Prefer complete hardware matches, including deliberately disabled displays.
   Within that set, use a user-selected preferred profile, then the last confirmed
   choice for this setup, then the existing score and a deterministic name tie-break.
3. Without a complete match, preserve the last confirmed working arrangement of
   the surviving displays as the base, and extend it for new outputs. At startup,
   use a suitable partial profile if there is no remembered live base; otherwise
   build a layout from discovered displays and defaults.
4. Retain base-profile and hardware-set provenance so unplugging the added screen
   can restore the original layout and workspace plan. Re-evaluate if a different
   saved profile or explicit preference has since been chosen.

Unknown means absent from the chosen layout, not necessarily never seen anywhere
on this machine. Hardware identity remains make/model/serial with existing
duplicate-device handling. Do not choose by connector name alone.

Explicit `enabled: false` for a recognized display remains intentional. Existing
`disable_unknown_outputs: true` remains a supported strict per-profile policy;
show it as `Keep other displays off` in both UIs and explain its effect when it
suppresses a connected screen. Never silently rewrite old profiles. The default
for newly created profiles is to extend. Migrating historical strict profiles
requires a visible choice, not guessing which flags were intentional.

### Default layout for a new output

Preserve existing displays' positions, scale, color settings, and workspace intent.
For a newly added independent output:

| Setting | Proposed default |
| --- | --- |
| Enabled | On |
| Position | Touch the right edge of the rightmost enabled independent display |
| Alignment | Vertically center against that adjacent display |
| Mode | Highest supported/native resolution, then highest advertised refresh at that resolution |
| Scale | Backend-recommended readable, valid, sharp scale |
| Rotation | Normal |
| Mirror | Off; extend the desktop |
| VRR | Off |
| Signal/color | Conservative SDR/8 bpc; do not copy another device's HDR or ICC calibration |

For logical sizes `(wa, ha)` and `(wn, hn)`, place a new right-hand output at
`x = xa + wa`, `y = ya + round((ha - hn) / 2)`. Account for scale and rotation
before computing the size. Negative coordinates are valid. For simultaneous
arrivals, use a stable hardware order and append each to the previous rightmost
output. Handle the no-surviving-display case explicitly, starting at `(0, 0)`.

Use a deterministic backend scale recommendation with the existing sharp-scale
rules. Sensible physical dimensions can inform readability; invalid or absent
EDID dimensions need a documented fallback, initially 1x. Never infer a projector's
viewing distance from its physical-size metadata. Show the recommendation and allow
an exact override in both clients.

Maximum resolution and refresh describe a valid advertised **pair**, not two
independent maxima. If application fails, try the preferred advertised mode,
then lower supported refresh/resolution candidates in a bounded recovery pass.
Retain a working existing screen throughout. Subsequent recovery uses backoff;
it must not keep flipping modes rapidly. Report any fallback, and never overwrite
the saved profile with the reduced mode. Compositor readback proves a mode was
applied, not that a slow projector has finished showing a picture.

### Workspaces

Inherit the base profile's enabled planner, strategy, group size, workspace count,
persistence policy, and existing monitor order. Append the new output to the end
of that order. Sequential groups of four remain sequential groups of four with
two displays. Do not replace the plan with generic defaults on every connection.

Keep Manual assignments on their surviving displays. Do not redistribute them
just because another screen appears; the new screen can receive ordinary
compositor workspaces until the person assigns more. On removal, temporarily
retarget orphaned workspaces to a usable output without rewriting the saved plan.
Mirrors do not gain an independent workspace allocation.

If the user explicitly turned planning off, preserve that decision. An unset
planner for a new setup can use the existing Sequential / 3 per group / 9 total
defaults. The current bool cannot distinguish unset from explicitly disabled, so
introduce a backward-compatible representation before implementing that distinction.
Do not guess intent from `enabled: false` in existing files.

Sequential is the preferred strategy when creating a plan. Importing consecutive
workspace rules on a single display cannot distinguish Sequential from Interleaved;
prefer Sequential with groups of three, preserving the imported total and
persistence. An exact saved profile takes precedence, including an explicitly saved
Interleaved plan. Never silently migrate existing profiles to the preferred default.

### Disconnect, lid, sleep, and recovery

Removing the temporary screen restores the original saved layout when that setup
matches again. Do not save the temporary expansion over `Laptop`. Editor changes
remain drafts; if topology changed underneath them, retain the edits, show the
conflict, and offer an explicit refresh or remap before applying.

Lid-close policy may switch off the internal screen only after another real
output is usable. A modeless output or synthetic `FALLBACK` does not satisfy that
condition. Distinguish intentional DPMS sleep from a failed wake before starting
recovery. Keep the machine's sleep and lock policy intact.

The daemon owns recovery while it owns monitor management. A bounded attempt may
end, but an unresolved intended output must remain scheduled for another attempt
with capped backoff. Stop or defer on unmanage, suspend, deliberate power-off,
hardware removal, or an interactive preview; resume appropriately. Preserve
healthy displays and expose all affected outputs. Cold startup must not depend on
a persisted external-only generated file to make the first screen accessible.

## Feedback and notification routing

After a new setup has actually been applied, send one normal desktop notification:

> Projector connected
> Added to the right of your laptop. Your Laptop profile is unchanged.
> [Adjust and save…]

Describe the actual placement, including preferences and fallbacks. Do not claim
success before verification. Group dock bursts, deduplicate daemon reconnects and
repeated status events, respect notification preferences and Do Not Disturb, and
avoid stealing focus. Known-profile restores should normally be quiet.

The daemon publishes one semantic event with a stable event/topology ID. A single
notification coordinator owns delivery, even with bars on multiple screens.
Route its action to the registered, enabled Omarchy panel when available; otherwise
open the TUI through a desktop/terminal launcher. Detect that an interface is
available, not merely that an Omarchy directory exists. If panel activation fails,
fall back to the TUI. Starting either editor loads current state and focuses the
new output; it must not erase an existing draft. A late click after unplugging
shows current displays and explains that the setup changed.

No UI process must stay open for extension or recovery to work. The compact view
also offers `Adjust and save…`, so dismissing a notification loses no capability.

## Profiles and selection preferences

Keep named profiles and add discoverable operations in both clients: Use, Edit,
Rename, Duplicate, Delete, Prefer for this setup, and an advanced post-apply command.
Profile browsing only selects a row; it never applies a layout or runs its hook.
Use starts a preview directly, without first requiring the user to find and disable
automatic matching. On confirmation it creates a clearly explained session override.

The panel offers right-click and a visible overflow button; the TUI offers an action
menu and documented shortcuts. Do not make important actions keyboard-only in the
panel or mouse-only in the TUI. Delete requires an explicit confirmation or undo.
Deleting the current saved profile must not blank screens or apply another layout
as a hidden side effect. Its live arrangement can become an unsaved setup.

Rename is a backend operation with collision checks and an atomic/recoverable
persistence contract. It updates profile references and sidecars consistently,
preserves timestamps as appropriate, respects supported symlink/dotfile workflows,
and does not execute hooks. Never implement it as an uncoordinated client-side
save followed by delete. Duplicate creates a distinct editable profile; copying
post-apply commands must be explicit. Reuse on different hardware is a separate
mapping workflow, preserving target-device calibration and never silently copying
commands or ICC settings from unrelated hardware.

Expose `Preferred for this setup` before configurable scoring weights or code.
Explain selection with plain reasons first, detailed score arithmetic on demand.
Keep automatic profile choice separate from monitor-management ownership.
Arbitrary user-supplied matcher code is deferred: a slow or broken script must
never be needed to light a newly connected display. Revisit only after preferences,
deterministic ties, and documented reasons fail to cover concrete use cases.

## Preview and save

Proposed default confirmation time: **30 seconds**, shared by both frontends and
the CLI through backend preferences. Offer 15, 30, 60, and 120 seconds. Store this
as an application preference rather than a profile field. The daemon validates
the effective duration; clients must stop hard-coding ten seconds. Older clients
that send an explicit duration remain compatible.

Use three distinct stages: Applying, Checking displays, and Awaiting confirmation.
Start the confirmation deadline after apply/verification succeeds. The current
backend already starts it after apply; extending the countdown alone does not
fix its separate three-second apply-validation window. Add a bounded, separately
tested readiness policy for slow docks/projectors. Show applying/recovering state
on a surviving display. A compositor reporting a mode cannot prove visual readiness,
which is why the subsequent human confirmation interval still matters.

Keep/Revert must remain reachable on any usable display, by mouse and keyboard,
through panel destruction, service reconnects, and screen remaps. Use the daemon's
deadline and transaction ownership; neither client can reset it locally. An
explicit `More time` action may extend it only through a bounded daemon operation;
it is a follow-up, not a prerequisite for shipping the longer default.

Separate using a layout from remembering it:

- For unapplied changes: `Preview changes`, then `Keep` or `Revert`.
- For save intent: name the profile, `Preview & save`, then `Keep & save` or `Revert`.
- For an unchanged automatic draft already live: `Save profile…` may persist that
  verified snapshot without another disruptive mode switch, with a freshness check.
- Renaming or duplicating a profile does not apply it.

Keep atomic commit-and-save. A failed save leaves the preview armed and recoverable.
Rollback restores both live state and owned config/include changes, or enters an
explicit recovery state if hardware has changed and exact restoration is impossible.
Never report success merely because the preview timer vanished.

## Shared interface structure

Proposed main page order: **Layout, Workspaces, Profiles**, numbered 1, 2, 3 in both
clients. Layout and workspaces edit one draft; Profiles manages saved arrangements.
Change labels, shortcuts, help, tests, and docs together, and mention the changed
2/3 shortcuts in release notes. Preferences is a separate small dialog/view reached
from the header or action menu, not a fourth permanent editor page.

The layout contains one canvas and a selected-display inspector. Give primary tabs
body-sized text and clear selected/focus states. Identify the selected display once.
Use `Display` and `Color` inspector tabs without a redundant `Display - Color`
heading. Show all six hardware-information fields directly, without a disclosure
toggle. Keep advanced color and timing controls available without giving
them the same initial prominence as resolution, scale, and enablement.

Connected disabled screens need a visible card/list separate from active geometry,
with `Off` and `Enable…`; selection reveals their inspector. Modeless displays show
`No usable signal` and recovery status. Disconnected saved screens show `Not connected`
and cannot be enabled until present. Mirrors show their source and remain selectable.
Text and icons must explain state without relying on color alone.

In the TUI layout editor, off/mirrored displays use separate bracketed selectable
rows with connector, explicit state, and model. An Enable button changes only the
draft; selecting the row only reveals its inspector. Reserve space above active
geometry and keep crowded rows reachable using the existing monitor-selection
keys. Profile actions are visible buttons, with shortcuts explained in help;
keep shortcuts in the footer and keyboard help, never inside action labels.
Only the numbered 1, 2, 3 navigation tabs retain inline shortcut hints.

Automatic profile selection is a global control in its own compact box above
the left-hand profile list, matching that column's width, not the full window. The
separate Saved Profiles box contains the Profile / Status / Match table, with
actions pinned below the scrolling rows and acting on the highlighted profile.
Keep selection distinct from active status; profile details sit alongside the
table, stacking below it on narrow terminals.

The panel uses `[-] [editable number] [+]` for steppers, with consistent units,
focus, direct entry, and resets. Position lives in logical pixels; arrows and
snapping remain available on the canvas. The TUI provides equivalent adjustments
and exact entry without imitating tiny graphical buttons.

Brightness stays a live hardware control in the compact panel, outside the expanded
profile editor. Use the short heading `Brightness`; show the current target as
secondary context when more than one display is connected. Capability limits such
as unavailable DDC/CI must be explicit. Do not confuse hardware brightness with
profile-owned SDR/HDR luminance controls. A shared optional brightness capability
for the TUI is follow-up work, not a reason to store brightness in profiles.

The footer must always expose the next action and current state. Scroll overflowing
content, not the whole window past its footer. Support the smallest practical
terminal (target 80x24) and a 1366x768 logical desktop with bar/panel margins. At
narrow sizes, stack or switch between canvas and inspector. Keep and Revert remain
visible even below normal editor minimums. Test fractional scaling, long names,
keyboard-only interaction, and enlarged fonts.

The expanded panel carries the compact view's setup status and contextual Create
profile action in a single full-width footer. Keep the secondary TUI launcher in
the header beside Identify, Keys, and Compact. Do not duplicate setup status in
the header or label normal automatic behavior repeatedly. A saved setup needs its
name and display count; paused matching remains explicit and actionable. Creating a
profile retains the current draft and reveals naming, Discard, and Preview & save.
Keep explicit dirty/creating/browsing state distinct from the live setup status.

## Accepted display presentation (2026-09-22)

Use connector identity, not arbitrary display numbers. The shared display summary
is connector, model with whole-inch size, current resolution/refresh,
scale with position, then assigned workspace IDs:

```text
DP-2
Microstep MPG321UR-QD 32"
3840x2160@144Hz
Scale 1.33x  Position 0,0
1, 2, 3, 4
```

Use ASCII x for resolution, no spaces around @ or before Hz, no separator dots,
and no Workspaces prefix inside display cards. Do not show logical desktop
dimensions. Round only presentation; preserve exact mode/scale data. Use a shared
formatter within each frontend, not per-view string assembly. Canvas values
describe its draft or saved profile; Identify describes a fresh live snapshot.

The inspector is hardware-only: connector, model, maximum advertised
resolution, physical millimetres, type, and serial, all visible together. Identify
is the only action in this box where supported; there is no More details action. Never
infer maximum resolution from the active mode or invent missing dimensions.

Use plain ASCII x for scale and dimensions. Panel size combines the diagonal
rounded to whole inches and exact reported millimetres: `32" (710x400mm)`.
No approximation symbol, typographic inch mark, or spaces within dimensions.
Use a plain ASCII double quote for inches. Canvas/Identify retain size beside the
model; only the inspector separates Model from Panel size. This is presentation
rounding, not a metadata edit.

Use Post-apply command in every user-facing label; exec remains the storage/API
field. Put this action last in profile details, and make Not set editable by
pointer and keyboard. Profile columns share widths and padding; selection is
highlighted without a redundant arrow. Dialogs use native theme controls,
explicit Delete profile wording, and safe cancellation by default. Graphical
menus anchor to the invoking button or pointer and clamp to the viewport.

Successful Keep & save establishes a clean baseline only after backend success.
Failed saves retain the draft and recovery controls. Do not change preview
ownership or persistence semantics as part of a visual cleanup.

## Preferences

Local implementation decision for #68: power-aware refresh is initially an
opt-in daemon flag, `--power-aware-refresh`, not an implicit default. Only enabled,
independent internal panels participate. Preserve resolution and calibration;
choose the highest advertised refresh on AC and the highest rate at or below
60Hz on battery (lowest advertised rate if none is at or below 60Hz). Unknown
power state leaves the profile unchanged. Never change the OS power profile.
Manual overrides and previews take precedence. A shared preferences control in
both frontends is follow-up work; the flag alone does not establish UI parity.

Keep one backend-owned preference source with versioned defaults and typed IPC.
Both interfaces edit it. Draft schema names must be finalized in an IPC change;
do not publish speculative config keys as working commands.

First preferences: new-display side (right/left/above/below), alignment (center or
edge), recommended versus explicit scale policy, mode policy, VRR default,
preview duration, and new-setup notifications. The defaults above remain useful
without opening this view. Per-profile explicit choices override defaults only
for recognized outputs; recovery constraints always apply.

Keep inherited workspace planning on the Workspaces page. An explicit global
fallback plan is useful only for genuinely new setups; it must not shadow the
current profile's plan. Add workspace persistence choices (`First per display`,
`All assigned`, `Custom`) beside the planner, preserving current defaults and
existing manual-rule semantics.

## Backend and IPC contract

The daemon remains the single writer when running. TUI and CLI may use the same
engine under the existing writer lock when it is absent. The panel never writes
monitor rules, implements independent matching, or starts a competing watcher.

Extend status/editor documents additively with capabilities, hardware snapshot
identity, selection reason, base profile, effective layout kind, per-output
connection/usability/disable reason, and recovery progress. Keep metadata needed
for drafts stable across reconnects. Resolve workspace plans in Go. Frontends
render that plan and may make responsive draft edits, but backend validation is
authoritative. Preserve unreported ICC/HDR/VRR settings through edits and round trips.

Planned operations include preference read/write, rename, preferred-profile
selection, notification action routing, and bounded preview extension. Update
`internal/ipc`, typed Go client helpers, `docs/_reference/ipc.md`, compatibility
tests, and both frontends together. Feature-detect additive capabilities; bump
protocol/schema versions when an old client would otherwise misinterpret data.
Keep older clients functional, with absent features clearly unavailable.

Correlate responses with request ID, editor revision, and hardware snapshot. A
slow read must not overwrite newer edits or accept a replacement monitor with
the same connector. Coalesce transient compositor queries, keep event handling
responsive, and distinguish busy from disconnected. One in-flight write/preview
owns the lifecycle. During a preview, defer automatic matching and background
recovery until the transaction finishes or an explicit safety abort occurs.

## Implementation order and acceptance

| Phase | Deliverable | Exit evidence |
| --- | --- | --- |
| 1. Usable displays | Recovery for modeless wake/startup, explicit health, slow-device readiness; reconcile existing recovery PR | A working output survives laptop/projector and desktop partial-wake failures; recovery continues without another hotplug |
| 2. Predictable hotplug | Centered extension, mode/scale/VRR defaults, inherited workspaces, reversible base state and strict-policy visibility | Laptop -> projector -> laptop changes no saved file and restores the original plan |
| 3. Safe interactions | Shared 30-second preference, draft preservation, notification action, visible off/mirror states | A slow projector can be confirmed; a stale click/read never discards edits or changes the wrong hardware |
| 4. Consistent editors | Shared page order and operations, context menus, atomic rename, compact responsive layout | Panel and TUI complete the same workflow, including at small sizes |
| 5. Advanced workflows | Preferred profiles, reuse, workspace persistence, optional automatic memory | Alternate profiles work on the same hardware; calibration and manual assignments survive |

Review relevant PRs before implementing overlapping features; see the baseline
review for dependencies and current gaps. Do not merge solely from a bot summary.

Required scenario coverage for the relevant phase:

1. Laptop with a saved single-screen layout; unknown projector arrives slowly,
   becomes usable to the right, inherits Sequential/group 4, is left unsaved,
   then disconnects and restores the original profile.
2. Known exact multi-screen layout with a deliberately disabled display, and a
   legacy strict profile with an unknown display. Show the reason and a clear
   opt-in enable action; no silent profile mutation.
3. Two new outputs, rotated neighbors, negative positions, fractional scaling,
   missing EDID dimensions, duplicate models/serials, and connector changes.
4. Existing Manual assignments, planner explicitly off, generated monitor order,
   workspace persistence, mirrors, and disappearing outputs.
5. Mode rejected, compositor busy, zero-size mode, missing capabilities, partial
   dock enumeration, wake without another event, and startup with external absent.
6. Preview confirm/revert/timeout, save failure, client loss, bar rebuild, TUI and
   panel open together, hotplug during preview, and rollback after topology change.
7. Multiple profiles for one hardware set; preferred choice, explicit one-time
   use, deletion/rename collisions, hook execution only after intended commit.
8. Notification with no panel installed, disabled plugin, action after unplug,
   multiple bars, muted notifications, and editor with existing unsaved changes.
9. 80x24 TUI, short graphical viewport, long localized labels, larger fonts,
   keyboard focus, stepper edits, and disabled-display discovery without a mouse.

Use deterministic fixtures and clocks for behavior, protocol contract tests for
both clients, and representative before/after captures for UI work. Record exactly
which physical projector/dock/lid/sleep cases were exercised; unit tests cannot
prove hardware link training. The target is a usable picture with no intervention
on supported hardware, no accidental saved-profile writes, and a visible recovery
path when the link fails. Log event-to-apply-to-usable timings to detect regressions.

## Deliberately deferred decisions

Automatic remembering of every confirmed layout may later reduce save work, but
must live separately from named profiles, be opt-in initially, and retain multiple
layouts per setup. Do not replace named profiles with one hidden mutable record.

User-programmable matching and automatic mirroring are not initial defaults.
Mirroring is an explicit presentation choice; extending protects private laptop
content from appearing on a projector unexpectedly. XWayland-primary remains the
documented opt-in Exec workflow from issue #56 unless that closed product decision
is explicitly revisited. Exact scale recommendation thresholds and readiness
timeouts need hardware evidence; the proposed values here are starting points.
