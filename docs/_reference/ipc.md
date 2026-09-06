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
| `preview` | `profile_name` or a full `profile`; optional `timeout_seconds` and `save_on_commit` | Transaction |
| `confirm` | `transaction_id` | none |
| `commit` | `transaction_id`; `save` boolean | none |
| `revert` | `transaction_id` | none |
| `save` | full `profile` | none |
| `delete` | `name` | none |
| `set_profile_auto` | `enabled` boolean | none |

A transaction contains an opaque `id`, the effective profile, and an RFC 3339 `deadline`.

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

After `subscribe`, the daemon pushes a fresh status document whenever monitor or profile state changes:

```json
{"type":"event","protocol_version":1,"event":"status","data":{}}
```

The document schema is versioned independently with `schema_version`. `daemon.profile_override` names the session-scoped manual profile for the current monitor set; when it is absent, profile choice is automatic. While an interactive change is awaiting confirmation, `daemon.preview` contains its `transaction_id`, `profile_name`, `deadline`, full effective `profile`, and optional `save_on_commit` intent. The profile lets a screen-bound integration reconstruct the target layout after the monitor change replaces its process or panel. Each profile summary includes `connected_outputs`, `connected_enabled_outputs`, `exact_display_match`, `match_score`, and a `match_reasons` breakdown with the count and score contribution for each connection state. `exact_display_match` is true when the profile accounts for every connected display and has no saved display missing; a connected display deliberately kept off still counts as part of that setup. Integrations should only offer a profile for manual selection when `connected_enabled_outputs` is greater than zero. This guarantees that applying the profile leaves at least one currently connected output enabled. The two connection counts let a browser distinguish all available saved outputs from the subset the profile enables.

Monitor summaries include the connector, make, model, active mode, physical and logical dimensions, position, scale, transform, internal/focused flags, and enabled state. Integrations can therefore render the same monitor identity and layout information as the TUI without querying Hyprland separately. `hyprmoncfg status --json` prints the same shape without requiring clients to speak the socket protocol.

`editor_state` is intentionally fetched on demand rather than included in every status event. Its `profile` is the live display and workspace state in the same shape accepted by `preview` and `save`; `profiles` contains the complete saved profiles for profile browsers. `workspace_plan` contains the resolved workspace assignments for the editable live profile, while `profile_workspace_plans` maps every saved profile name to its resolved assignments so read-only browsers can preview the same plan without reimplementing workspace strategies. `displays` adds supported modes, focus, DPMS, active workspace, internal/external type, and physical panel size for each connected output. `source_profile` is present only when the live state exactly matches one saved profile. Settings that Hyprland cannot report back reliably, such as ICC and HDR luminance overrides, are preserved from that profile or from the best hardware match so an editor does not erase advanced configuration it never displayed.

`edit_profile` applies one typed edit to a client-owned draft. Display mode, scale, VRR, transform, mirroring, position, enablement, color/HDR/ICC settings, and workspace settings are normalized and validated by the same profile geometry used by the bundled TUI. Resize edits reflow displays beyond the old right and bottom edges; dragged position edits can request edge snapping with `snap_distance`, and an overlapping drop is placed on the nearest clear outside edge. The method is stateless and does not touch the live displays until the returned profile is sent to `preview`.
