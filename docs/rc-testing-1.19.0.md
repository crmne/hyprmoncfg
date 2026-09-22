# 1.19.0 release-candidate test rubric

Test backend `v1.19.0-rc.1` with panel `v2.4.0-rc.1`. Back up
`~/.config/hyprmoncfg` first. Keep a TTY or another usable display available for
physical display tests. Do not test a presentation-critical setup for the first
time on stage.

Record the exact hardware, Hyprland and Omarchy versions, whether each check
passed, and the relevant `journalctl --user -u hyprmoncfgd` excerpt on failure.

## Safety baseline

- Start with a known-good saved profile. Confirm both displays are usable and
  `hyprmoncfg status --json` reports `usable: true` / `health: usable`.
- Preview a harmless position change. Confirm Keep, Revert and timeout rollback
  all work and remain reachable after the bar rebuilds.
- Confirm saved profile JSON and generated sidecars change only after an explicit
  save. `hyprmoncfg unmanage` must hand display ownership back cleanly.

## Hotplug and presentation

- From a laptop-only profile, connect an unfamiliar projector or display. It
  should turn on to the right, vertically centered, without altering the saved
  laptop profile. Disconnect it and confirm the laptop layout returns.
- Repeat with a slow projector and capture time from connection to usable image.
  Report any `0x0`, `no_signal`, repeated mode flip, or retry loop.
- Test a deliberately disabled known display and a strict profile. The display
  must remain intentional and discoverable rather than being silently enabled.

## Lid, sleep and recovery

- With an external display usable, close the lid; the external must remain usable.
  With the external at `0x0` or otherwise unusable, the internal panel must not be
  disabled solely because the external connector exists.
- Suspend/resume with lid open, then lid closed with an external display. Test an
  external DPMS off/on cycle. Recovery must defer during sleep/preview, retry a
  failed apply without a new hotplug event, and stop after success.
- Confirm healthy displays do not blink repeatedly during recovery. All affected
  outputs should appear in the error/status rather than only the first.

## Workspaces and small terminals

- At 80x24, check Layout, Workspaces and Profiles. Apply, Save and Keys must remain
  visible. Open and cancel Save/Delete dialogs without changing stored profiles.
- Save one generated plan with First per display and one with All assigned. Inspect
  generated rules, restart the daemon, reopen both editors, and confirm the policy
  survives. Manual rules must retain their individual persistence flags.

## Optional power-aware refresh

- Only when explicitly testing it, run the daemon with
  `--power-aware-refresh`. On a laptop internal panel, unplug AC and confirm the
  chosen mode is the highest supported rate at or below 60Hz; reconnect AC and
  confirm the highest supported rate at the same resolution returns.
- External displays, scale, position, color settings, saved profiles and the OS
  power profile must remain unchanged. Unknown power telemetry should do nothing.

## Panel interaction

- Check Layout / Workspaces / Profiles at normal and short panel sizes. Verify
  display summaries, all hardware details, Identify/Identify all, disabled and
  mirrored rows, profile context-menu anchoring, themed delete confirmation, and
  Post-apply command placement/editing.
- Confirm automatic-selection and Saved Profiles are distinct boxes, columns align,
  no redundant selection arrow appears, and a successful Keep & save removes the
  Preview & save state. An older backend must show workspace persistence as
  unavailable instead of pretending to save it.

Passing unit tests do not replace the projector, lid, dock, suspend, DPMS or
AC/battery checks above. Do not promote the RC while any test can leave all real
outputs unusable, overwrite an unrelated profile, lose rollback, or race a preview.
