---
title: IPC Protocol
description: Versioned Unix-socket protocol for controlling hyprmoncfgd.
nav_order: 3
---

## Overview

`hyprmoncfgd` exposes a newline-delimited JSON protocol on:

```text
$XDG_RUNTIME_DIR/hyprmoncfgd.sock
```

The socket is mode `0600`. Protocol version 1 is designed for the bundled TUI and CLI as well as local desktop integrations such as an Omarchy bar panel.

When the socket exists, the daemon is the canonical writer. Clients should not edit profiles or the active generated monitor file alongside it. The bundled TUI and CLI fall back to the same core engine only when no daemon owns the session writer lock.

## Envelopes

Every message is one JSON object followed by a newline. A request looks like:

```json
{"type":"request","protocol_version":1,"id":"1","method":"status"}
```

The matching response repeats the client-generated ID:

```json
{"type":"response","protocol_version":1,"id":"1","result":{}}
```

Errors replace `result` with an object containing a stable `code`, a human-readable `message`, and optional `data`.

## Methods

| Method | Parameters | Result |
|---|---|---|
| `status` | none | Status document |
| `subscribe` | none | Current status document, followed by `status` events |
| `editor_state` | none | Editable live profile, supported modes, and source-profile metadata |
| `edit_profile` | full `profile` and one `edit` operation | Updated profile draft |
| `reuse_profile` | saved profile `name` and `mapping` from saved output keys to current output keys | Unnamed profile draft, workspace plan, and optional warnings |
| `preview` | `profile_name` or a full `profile`; optional `timeout_seconds` and `save_on_commit` | Transaction |
| `confirm` | `transaction_id` | none |
| `commit` | `transaction_id`; `save` boolean | none |
| `revert` | `transaction_id` | none |
| `save` | full `profile` | none |
| `delete` | `name` | none |
| `set_profile_auto` | `enabled` boolean | none |

A transaction contains an opaque `id`, the effective profile, and an RFC 3339 `deadline`.

An omitted or nonpositive `timeout_seconds` uses the 30-second default. Explicit
durations from older clients are still honored. The deadline starts after apply
verification succeeds; the confirmation interval is separate from link readiness.

## Safe preview lifecycle

Only one preview can be active at a time. It belongs to the connection that created it. If that connection disappears—for example, because changing the monitor layout rebuilt a screen-bound panel—the preview stays armed until its original deadline. Its transaction metadata remains available as `daemon.preview` in status, and a replacement client can reclaim it by sending the same transaction ID to `confirm`, `commit`, or `revert`.

`daemon.preview.reclaimable` is true only after the owning connection disappears.
A subscriber must not present a modal confirmation for another live client's
preview: observing a transaction ID does not grant ownership. Present controls
for transactions created by your connection, or for a reclaimable transaction.
Treat a missing `reclaimable` field from an older daemon as false. The daemon's
deadline remains authoritative after reclamation, and an expired preview cannot
be kept by a late commit.

A preview is reverted when any of these happens:

- the client calls `revert`
- the deadline expires
- the daemon shuts down
- monitor management is turned off

Rollback restores both the generated monitor rules and any root-config include
added or moved by that preview. If the root config was edited in the meantime,
rollback reports the conflict and preserves the edited config and its target
instead of overwriting those edits or creating a dangling include.

After `confirm`, the selected profile becomes a session-scoped override for the current connected monitor set. The daemon still owns monitor management, but automatic best-match selection is paused until the hardware set changes, the daemon restarts, or `set_profile_auto` is called with `enabled: true`. A late `confirm` or `revert` returns the `transaction_unavailable` error code.

`commit` completes the same safe preview transaction and can atomically save its effective profile before making the layout permanent. If saving fails, the transaction stays armed and still reverts at its deadline. Compact editors should use `save_on_commit: true` when creating a draft preview (or `commit` with `save: true`) for “keep and save” rather than racing separate `confirm` and `save` requests. Keeping that intent in the daemon lets a replacement panel finish the transaction correctly after reconnecting.

## Status events

Monitor summaries add `usable` and `health` (`usable`, `off`, `sleeping`,
`no_signal`, or `synthetic`). `enabled` alone is not evidence of a working mode.
Older daemons omit these fields; clients must not interpret absence as failure.

Editor documents add `workspace_persistence_supported`. Only when true may an
editor offer `workspaces.persist_all`. Missing capability means unavailable, not
false user intent. The typed client rejects sending this setting to older
daemons rather than silently losing it. Older clients retain their existing
first-per-display default; upgrade both clients before editing profiles that use
the new policy, since old clients cannot preserve an unknown field.

After `subscribe`, the daemon pushes a fresh status document whenever monitor or profile state changes:

```json
{"type":"event","protocol_version":1,"event":"status","data":{}}
```

The document schema is versioned independently with `schema_version`. `daemon.profile_override` names the session-scoped manual profile for the current monitor set; when it is absent, profile choice is automatic. While an interactive change is awaiting confirmation, `daemon.preview` contains its `transaction_id`, `profile_name`, `deadline`, full effective `profile`, and optional `save_on_commit` intent. The profile lets a screen-bound integration reconstruct the target layout after the monitor change replaces its process or panel. Each profile summary includes `connected_outputs`, `connected_enabled_outputs`, `exact_display_match`, `match_score`, and a `match_reasons` breakdown with the count and score contribution for each connection state. `exact_display_match` is true when the profile accounts for every connected display and has no saved display missing; a connected display deliberately kept off still counts as part of that setup. Integrations should only offer a profile for manual selection when `connected_enabled_outputs` is greater than zero. This guarantees that applying the profile leaves at least one currently connected output enabled. The two connection counts let a browser distinguish all available saved outputs from the subset the profile enables.

Monitor summaries include the connector, make, model, serial, output `key`, hardware `match_key`, active mode, physical and logical dimensions, position, scale, transform, internal/focused flags, and enabled state. Integrations can therefore render the same monitor identity and layout information as the TUI without querying Hyprland separately. `hyprmoncfg status --json` prints the same shape without requiring clients to speak the socket protocol.

`editor_state` is intentionally fetched on demand rather than included in every status event. Its `profile` is the live display and workspace state in the same shape accepted by `preview` and `save`; `profiles` contains the complete saved profiles for profile browsers. `workspace_plan` contains the resolved workspace assignments for the editable live profile, while `profile_workspace_plans` maps every saved profile name to its resolved assignments so read-only browsers can preview the same plan without reimplementing workspace strategies. `displays` adds supported modes, focus, DPMS, active workspace, internal/external type, and physical panel size for each connected output. `source_profile` is present only when the live state exactly matches one saved profile. Settings that Hyprland cannot report back reliably, such as ICC and HDR luminance overrides, are preserved from that profile or from the best hardware match so an editor does not erase advanced configuration it never displayed.

`edit_profile` applies one typed edit to a client-owned draft. Display mode, scale, VRR, transform, mirroring, position, enablement, color/HDR/ICC settings, and workspace settings are normalized and validated by the same profile geometry used by the bundled TUI. Resize edits reflow displays beyond the old right and bottom edges; dragged position edits can request edge snapping with `snap_distance`, and an overlapping drop is placed on the nearest clear outside edge. The method is stateless and does not touch the live displays until the returned profile is sent to `preview`.

## Reuse a saved layout

`editor_state` advertises optional operations in `capabilities`. Check for `"reuse_profile"` before offering layout reuse; an absent field means that an older daemon does not advertise it. This additive capability keeps protocol version 1 compatible with existing clients.

`reuse_profile` maps roles from any saved layout onto explicitly chosen current displays. For example:

```json
{"type":"request","protocol_version":1,"id":"reuse-1","method":"reuse_profile","params":{"name":"desk","mapping":{"saved-left":"current-left","saved-right":"current-right","saved-extra":""}}}
```

Use actual output keys from the saved profile and the live `editor_state.profile`. Each current output can fill at most one saved role. Every enabled saved role needs an explicit target or an empty string to skip it; a disabled saved role may also be omitted. Unknown saved keys, disconnected targets, and duplicate target assignments fail validation. A stale mapping should prompt the client to refresh the current hardware and ask the user to review it again.

The response has the same shape as an editor draft: `profile`, `workspace_plan`, optional `warnings: string[]`, and `monitor_set_hash` identifying the current hardware snapshot. The profile name is empty. The method reads fresh monitor and workspace state, copies the requested geometry onto current hardware identities, and remaps mirror targets and workspace output keys and names. Skipped roles lose their workspace references. Unassigned current displays retain their settings and move clear of reused roles if needed.

Generated workspace plans retain the template's effective display order, including when `monitor_order` is omitted. Unassigned current displays follow the mapped roles in that order.

Unsupported saved modes fall back to a current or available mode, with a warning; scale adjustments are reported too. If those adjustments enlarge a mapped display into another role, the adapted display moves to the right with a warning, and its mirrors follow it. Unchanged mapped positions are preserved; a remaining overlap is rejected before returning a draft. Color, HDR, ICC and VRR settings stay with current hardware unless the saved and current display have the same unambiguous serial identity. Settings Hyprland cannot report are recovered from a separate current exact/best profile by current output key, including unassigned outputs. The selected template supplies unreported settings only for an unambiguous matching serial, so an uncertain template cannot restore its own old calibration through this fallback. If no safe stored settings are available, the draft uses reported values and defaults for unreported settings. The selected template's `disable_unknown_outputs` policy is retained independently of calibration donors. Invalid mirror dependencies, including disabled sources, become independent displays with a warning. Saved `Exec` hooks are never copied.

At least one enabled independent real display must have positive width and height in the resulting draft. Zero-size and synthetic outputs do not satisfy that requirement. An inactive display may still be mapped and enabled using a valid advertised mode; live DPMS or disabled state does not prevent preparing that draft.

Reuse is draft-only: it does not apply a layout, write a profile, execute a hook, or alter an existing preview transaction. Show the returned warnings, let the user review and name the draft, then use the normal preview and commit lifecycle. The original saved profile remains unchanged.

## Connecting displays and query timeouts

Monitor and workspace-rule reads have a 750 ms deadline per query by default. A slow compositor or blocked DRM identity probe returns `error.code = "compositor_busy"` with a human-readable connecting message. No stale or partially enriched hardware snapshot is substituted for a successful response.

Status, editor-state, and reuse responses include an additive `monitor_set_hash`. Treat it as an opaque equality token for the output keys and their current connector bindings; it includes the connector paths used to distinguish otherwise identical outputs. It excludes layout, focus and power state. These daemon reads check hardware before and after reading workspace rules and return `compositor_busy` if the bindings changed. They therefore perform two monitor queries and one workspace-rule query; the 750 ms limit is per query, not a total response-time guarantee.

Clients should retain the last visible snapshot while connecting, show the message, and retry while visible with at most one request in flight. Do not treat a failed refresh as an empty display list. Compare editor/reuse hashes with the latest status hash when both are present, and discard mismatches. Also track observed topology changes locally and reject requests from an older generation, including a change away and back to the same hardware. The hash is not an event counter or an atomic snapshot guarantee. Older daemons omit it; clients can fall back to their available identity/signature fields. Stateless `edit_profile` replies do not include a fresh hardware hash. Known-profile apply and rollback retain their existing serialized lifecycle.
