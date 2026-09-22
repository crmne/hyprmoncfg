# Follow-up issue and PR assessment, 2026-09-22

## Implemented and installed after the assessment

The assessment below is historical. These changes became `1.19.0-rc.1` after
validation in a local `1.18.4-dev.followup` build:

- #60: essential Apply/Save/Keys footer hints take priority over optional links
  and version text. The automated size matrix covers all three pages; live
  80x24 Layout and Workspaces checks confirm the footer stays visible.
- #62/#67: closed-lid usability guard regression coverage; independent automatic
  apply retries with 2s initial/30s maximum backoff, preview deferral and suspend/
  sleep/unmanage guards; all-output validation errors and per-output health.
  All-DPMS-off failed wake, mode fallback and cold-start rescue remain incomplete.
- #65: `persist_all` preserves existing defaults, supports all assigned generated
  workspaces, preserves Manual flags and survives JSON/readback. Both editors
  have Persistence controls, with an additive daemon capability and older-daemon
  rejection in the typed client.
- #68: opt-in `--power-aware-refresh` with sysfs power polling and supported-mode
  selection for internal panels only. Default off; UI preferences remain pending.
- #61 foundation: 750ms monitor/workspace-query bounds. Coordinated snapshots,
  reuse and the remaining #59/#61/#18 PR changes have not been merged or installed.

Validation: complete Go test and race suites, vet, build, module tidy and diff
checks passed; the final suspend guard also passed daemon race tests. Panel:
79 Node tests and 21 Qt tests, QML lint and plugin validation passed. Runtime:
the new daemon reports both external outputs usable, retains Desktop Work and
the exact prior mode/position/scale, and all saved profile/sidecar checksums are
unchanged. The real 80x24 TUI displayed and toggled Persistence as an unsaved
draft; the test draft was discarded without applying or saving. Existing dirty
review sessions were left alone. The clean profile-review session was restarted
with the installed binary. Installed panel QML/JS match the repository.

The local TUI capture was actual runtime evidence; the later capture did not
show the panel and is not panel visual validation. No physical lid, projector,
dock-failure, suspend or
AC/battery transition was tested. This host has no internal panel/lid source.
No additional issue closure or PR merge was performed. This statement predates
the later RC commit and tag.

## Original assessment

Local, uncommitted review. No live display sleep, suspend, lid, or mode changes
were performed. PR code was inspected selectively, not fully audited or merged.

## Actions and tests

- Panel PR #17 was closed at the owner's request, thanking the contributor and
  explicitly stating that the replacement Identify implementation is still local
  and unreleased. No other GitHub item was changed.
- Issue #60: added 18 rendered page-size cases across Layout, Workspaces and
  Profiles at 80x24, 90x24, 96x24, 113x33, 136x38 and 80x16. Width, height, footer
  placement and selected-row/action visibility pass with 20 saved profiles.
  This is terminal-cell coverage, not a physical 1368x768 reproduction.
- Remaining #60 defect: the 80x24 Layout footer ends with `a ap... Ask Donate dev`;
  Save is not discoverable there. Prioritize essential action hints before project
  links/version, and add a failing-then-fixed regression for Save/Help visibility.
  Do not close the issue based on dimensions alone.
- Issue #62: new policy tests cover an already-disabled live laptop with a saved
  enabled panel, a modeless external with DPMS on/off, profile immutability, and
  restoration of ordinary clamshell policy once the external becomes usable.
  A fake-compositor daemon apply test also verifies the generated layout restores
  the laptop instead of disabling it. These pass. They do not prove persistent
  recovery or the all-displays-asleep path; `applyBest` still defers that path.
- Focused #60/#62, unusable-external and preview rollback tests passed five runs
  with the race detector. Tests use temporary files and fake compositor commands.

## Recommended work

1. Fix #60's essential footer hints, then verify save/preview/revert controls at
   the reported desktop size and font/scaling combinations before closure.
2. Treat #62/#67 and PR #59 as one recovery design, preserving the safety guard.
   The current loop clears pending after apply failure; unchanged monitor hashes
   do not schedule another apply. Add independent degraded-output retry state,
   capped backoff, all-output usability reporting and bounded per-attempt work.
   Pause during intentional sleep, suspend, unmanage and previews; preserve saved
   profiles and healthy displays. Do not restore a competing monitor writer.
3. #65: add a backend workspace persistence policy (first per display / all),
   retaining current first-only behavior as the backward-compatible default.
   Current generated rules mark only each output's first workspace persistent.
   Preserve Manual rule flags; update inference/readback, IPC and both controls.
4. #68: optional internal-panel AC/battery refresh policy needs an actual power
   event source, not a post-apply hook (a hook does not run on an otherwise idle
   AC transition). Keep external outputs unchanged, select supported modes and
   defer while asleep or previewing. Unsupported 60Hz needs an explicit fallback
   policy. This remains a proposal, not an implemented capability.

## PR integration considerations

- Backend #59, head `35cb97a033a2e5f0288939b78ea6fb4491c3bfb8`: GitHub reports
  merge conflicts; test check successful. Useful explicit wake and cold-start
  safeguards. Its recovery window is 45 seconds, starts on resume/lid-open, and
  does not directly cover #67's desktop DPMS-only wake. `wakeRecovered` checks
  enablement/DPMS but not positive width/height; strengthen that condition and
  regression coverage before using it as a usability test. Its internal-panel
  rescue is open-lid-only, so it is not a complete closed-lid #62 solution.
- Backend #61, head `fc62fe699e4034c997306ec3eca70737c1d8c44c`: GitHub reports
  conflicts and no checks in the queried rollup. Its new automatic matcher
  excludes any profile with unknown connected outputs. That conflicts with our
  partial-profile extension and inherited workspace intent; do not adopt that
  policy blindly. Bounded discovery, correlated snapshots, calibration-safe reuse
  and typed busy responses are worth integrating separately after review.
- Panel #18, head `034231672808109155e3521f7e103f52805103ca`: mergeable against
  published main and successful test check, not evidence of compatibility with
  our dirty redesign. Preserve reuse, snapshot correlation, queued Discard and
  background draft protection while adapting them to our new controls. Reuse
  depends on #61. Reconcile Identify ownership with our implementation rather
  than adding a second overlay. Test races and capability fallback as a pair.

Suggested order: footer usability; recovery and health (#62/#67/#59); persistence
(#65); coordinated discovery/reuse (#61/#18); optional power policy (#68).
No PR implementation was installed or executed during this assessment.

## Additional runtime test pass

- Full `go test -race ./...` passed. The wake/lid, issue #62, unusable-external
  and preview-ownership/rollback group also passed 20 consecutive race runs.
- Panel: 77 Node tests and 21 offscreen Qt tests passed; QML lint and plugin
  validation passed. No daemon or panel code was changed during this test pass.
- Ran the actual development TUI in an isolated 80x24 tmux terminal against the
  live daemon. Inspected Layout, Workspaces and Profiles; opened and cancelled
  Save and Delete without confirming either. All pages and dialog actions fit.
- Runtime confirms essential footer hints truncate more severely with the longer
  development version label: Layout hides Save, Workspaces also hides Save, and
  Profiles truncates the command/delete hints. Save dialog explanatory text also
  truncates, although Save / Save & Apply / Cancel remain visible.
- The isolated test TUI was closed. No profile was saved/deleted, no preview was
  started, and the live Desktop Work layout was unchanged.
- Current hardware reports two external displays and no internal panel, so the
  physical closed-lid scenario cannot be tested on this setup. DPMS/suspend fault
  reproduction remains an attended hardware test, not covered by these results.
