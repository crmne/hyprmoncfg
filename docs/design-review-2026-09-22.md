# Display workflow review, 2026-09-22

Evidence for [DESIGN.md](../DESIGN.md). This is a dated review, not current user
documentation or a claim that every issue was reproduced.

## Baseline and limits

- Backend release: `v1.18.4`, commit `219d98712bf85ccdc864ba96dfd8c76870c5d473`.
- Backend main inspected: `626aa69ef50d363495301e97f6dad33a6287869c`.
- Panel release: `v2.3.5`, commit prefix `cf05a04`; local removal of the permanent
  marketplace row remains an uncommitted change from the preceding task.
- Installed CLI and daemon both report 1.18.4. The clean local backend checkout
  was fast-forwarded to upstream main before documentation work.
- Inspected committed panel compact/layout/profiles screenshots and TUI layout
  screenshot. They are historical references, not new live captures; the TUI
  screenshot visibly shows 1.15.0. Current behavior was checked against code.
- No conference projector was available. No monitor layout, user profile,
  package, service, or installed plugin was changed for this review.

The current local Laptop profile has no strict disable-unknown flag and enables
only its internal display. That does not establish the historical conference
cause. The pre-1.18.4 apply path disabled omitted monitors, so the reported failure
is consistent with the old behavior; event logs and the exact conference version
would be needed to confirm it. Do not attribute it to manual plugin installation.

## Released behavior versus the requested behavior

| Area | Evidence in 1.18.4 / panel 2.3.5 | Gap |
| --- | --- | --- |
| Unknown display | `profile.ExtendConnected` adds omitted displays without changing the stored profile | Explicit strict flag and explicitly disabled outputs remain off |
| Position | `internal/profile/hotplug.go` uses the rightmost independent output's right edge and Y origin | Top aligned, not vertically centered |
| Mode and scale | Retains live mode/scale; for zero dimensions uses first parseable advertised mode; nonpositive scale becomes 1 | No highest-resolution/refresh ranking or new-device readability policy |
| VRR/mirroring | Clears mirror and rotation, but `FromMonitors` copies live VRR and color | VRR is not explicitly off; conservative color policy is not defined |
| Workspaces | Preserves an enabled plan and appends the output; disabled planner becomes Sequential/3/9 | Cannot distinguish deliberate planner-off from missing defaults |
| Draft identity | `ExactStateMatch` refuses to call the extended layout an exact saved state | No explicit automatic-draft provenance field or action notification |
| Disconnect | Matching runs again and can select the original saved profile | Must test base restoration with alternate profiles, manual choice, and edited drafts |
| Matching | Scores enabled/disabled/missing/unknown hardware; ties alphabetically; holds recognized setups through transient wake | No user-selected preferred profile for one hardware set |
| Preview | Backend default and frontend requests are 10 seconds; backend starts deadline after apply | Shared preference missing; slow-device verification also needs attention |
| Apply verification | `internal/apply/apply.go` has a separate 3-second validation timeout | Increasing confirmation alone does not give mode setup more time |
| Closed lid | `ApplyClosedLidPolicy` checks presence of any external before disabling internal | Presence does not establish a usable mode |
| Recovery | Polling compares state hashes; a stable bad state can stop generating new apply attempts | Open reports require persistent recovery scheduling and health reporting |
| Panel hierarchy | Top page buttons use `Style.font.caption`; inspector uses `Display - Color` plus tabs; large permanent Info pane | Smaller primary navigation and repeated labels compete with controls |
| Disabled displays | `DisplayCanvas.qml` reduces omitted geometry to a caption string | No selectable disabled-display card or direct enable action there |
| Brightness | Live Omarchy hardware control appears in compact and expanded views | Expanded profile context suggests it is saved, although it is not |
| Profile operations | Both clients have delete and Exec editing; panel offers keyboard paths | Discoverable graphical menu missing; no atomic rename IPC operation in either client |
| Strict policy parity | TUI 1.18.4 has `U` and `EditorEdit.DisableUnknownOutputs` exists | Panel has no corresponding control |
| Manual use | Both frontends require turning automatic selection off first | Unnecessary friction before explicitly choosing a profile |

Key source entry points:

- [Automatic extension](../internal/profile/hotplug.go) and
  [extension tests](../internal/profile/hotplug_test.go).
- [Daemon selection](../internal/daemon/daemon.go),
  [matching](../internal/profile/match.go), and
  [closed-lid policy](../internal/profile/clamshell.go).
- [Editor preservation](../internal/profile/editor.go),
  [workspace resolution](../internal/profile/workspaces.go), and
  [status schema](../internal/appstatus/status.go).
- [Preview lifecycle](../internal/daemon/ipc.go),
  [apply verification](../internal/apply/apply.go), and
  [IPC reference](_reference/ipc.md).
- [TUI shortcuts](../internal/tui/keybindings.go) and
  [panel source](https://github.com/crmne/omarchy-hyprmoncfg/blob/v2.3.5/Panel.qml).

## Open community work

Read issue bodies and conversations, PR descriptions, comments, changed-file
lists, and available review summaries. This is product triage, not a full independent
line-by-line review or approval of any PR. States below were checked on 2026-09-22.
There were four open backend issues, two open backend PRs, no open panel issues,
and two open panel PRs.

| Work | Status and contribution | Design consequence |
| --- | --- | --- |
| [Backend #67](https://github.com/crmne/hyprmoncfg/issues/67) | Open report: 1.18.4 desktop outputs stay at 0x0 after DPMS wake; retries stop and status says enabled | Highest priority: distinguish enabled from usable, report all failed outputs, retry with backoff |
| [Backend #62](https://github.com/crmne/hyprmoncfg/issues/62) | Open report: closed lid plus modeless external leaves no usable output | Usability must gate internal-panel disable; recovery must not require a new plug event |
| [Backend #60](https://github.com/crmne/hyprmoncfg/issues/60) | Open report: footer inaccessible on a 1368x768 screen | Responsive layout and persistent action footer are functional requirements |
| [Backend #65](https://github.com/crmne/hyprmoncfg/issues/65) | Open request: all assigned workspaces persistent; workaround is disabling planner | Add persistence policy to shared planner, not user edits to generated files |
| [Backend PR #59](https://github.com/crmne/hyprmoncfg/pull/59) | Open: failed-wake internal-panel recovery, 45-second retry window, cold-start safeguard; candidate hardware validation outstanding | Reconcile with #62/#67; bounded retries alone do not meet indefinite degraded-state supervision |
| [Backend PR #61](https://github.com/crmne/hyprmoncfg/pull/61) | Open: bounded discovery, topology snapshots, layout reuse and calibration preservation; no physical unfamiliar-dock test reported | Reuse its foundations after review; reconcile its complete-display automatic-match policy with 1.18.4 partial-profile extension |
| [Panel PR #18](https://github.com/crmne/omarchy-hyprmoncfg/pull/18) | Open companion to #61: reuse, selected-screen identification, draft-preserving refresh, busy/retry handling; review still calls out interaction issues | Coordinate API capabilities; validate stale-response sequences and identification during busy/preview states |
| [Panel PR #17](https://github.com/crmne/omarchy-hyprmoncfg/pull/17) | Open: Identify all displays, persistent non-interactive overlay and numbers; overlaps #18 | Share one identification service/model; offer Identify all and identify selected without conflicting overlays |

Additional history matters:

- [Merged #66](https://github.com/crmne/hyprmoncfg/pull/66) fixes generated workspace
  order being replaced by physical order on readback. Preserve that fix during
  extension, reuse, and UI refactoring.
- [Closed #56](https://github.com/crmne/hyprmoncfg/issues/56) deliberately chose an
  Exec recipe for XWayland-primary instead of a new profile field. Copying macOS's
  main-display control would reopen that decision and is outside the initial plan.
- Closed #54, #53, #47, and #58 document rollback/include, profile symlink,
  synthetic-output, and lock/wake regressions worth retaining in acceptance tests.

Do not execute reporter workaround scripts as part of design review. Do not
publish replies, change labels, or merge these PRs merely to create a roadmap.

## macOS reference and interpretation

Apple's [Displays settings guide](https://support.apple.com/guide/mac-help/change-displays-settings-mh40768/mac)
describes selected-display controls, arrangement, extend/mirror choices, automatic
resolution, refresh rate, and advanced options. Its
[external-display guide](https://support.apple.com/guide/mac-help/use-one-or-more-external-displays-with-your-mac-mchl7c7ebe08/mac)
explains connection and arrangement workflows.

Design inference: adopt low-friction connection, direct spatial arrangement,
contextual controls, and progressive disclosure. Neither guide establishes a
public contract for hidden per-combination profile persistence, centered-right
placement, or always selecting maximum refresh. Those are our proposed policies,
not claims about macOS internals. Keep hyprmoncfg's explicit alternate profiles
and workspace planning. Treat Apple's automatic resolution as inspiration for
an understandable recommendation, not evidence that every projector supports
the largest advertised mode reliably.

## Visual review and next evidence

The committed screenshots already expose the repeated Display/Color headings,
caption-sized primary tabs, large diagnostic Info area, brightness in the profile
editor, and dense monitor-card text. The compact panel has a clearer hierarchy and
is a useful reference for the expanded view.

The panel's `design/monitor-manager.html` is a new interactive design study using
sample data. It demonstrates page order, selected-output controls, explicit off
states, preferences, profile actions, and a 30-second preview. It is not a running
QML implementation or evidence of hardware behavior. Prefer this editable control
prototype to raster UI art for validating these interactions. Capture new real
panel and TUI screenshots when implementation changes them.

The user accepted the mockup as the visual direction on 2026-09-22. Browser checks
exercised selection, numeric editing, workspace navigation, profile context menus,
off-display enable/keep/revert, preview-duration preference, and footer visibility
at 1440x1000, 1366x768, and 760x850. This validates the study only. Existing backend
profile and daemon package tests passed; the physical projector workflow remains
an implementation acceptance task.
