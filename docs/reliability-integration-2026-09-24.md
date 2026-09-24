# Reliability integration, 2026-09-24

The maintainer requested separating mapleroyal's reliability work from the
optional layout-reuse workflow. This is an integration record, not a release
announcement or an approval of the remaining feature.

Sources:

- [Backend PR #61](https://github.com/crmne/hyprmoncfg/pull/61), reviewed head
  `a3ab98ed253aaba56b0a344a5686eab1333a3f4c`.
- [Panel PR #18](https://github.com/crmne/omarchy-hyprmoncfg/pull/18), reviewed head
  `24024687b7c71780eece82d047d66f29e091909f`.

## Integrated independently of reuse

- Bounded compositor/connector queries and a shared connector-enrichment worker.
- Coalesced asynchronous monitor probes, topology/wake generation checks, and
  retries that do not block the event loop behind an interactive writer.
- Consistent hardware snapshot hashes and retryable `compositor_busy` responses.
  The TUI retains its daemon connection after this transient error.
- Panel status/editor response correlation, input and dirty-draft preservation,
  preview fencing, and explicit Discard intent through delayed refreshes.
- Fresh-live-snapshot Identify validation and cancellation on topology changes,
  connection loss, or preview activity. Existing explicit Identify buttons remain.

The wire additions are optional for older clients. The panel falls back to a
monitor-state signature when a daemon does not supply a snapshot hash. No saved
profile format changes or profile migrations are part of this split.

## Deferred on the contributor branches

Layout remapping, the `reuse_profile` operation/capability and typed client,
mapping UI, and click-to-identify canvas interaction remain outside main. The
remaining PRs are reconstructed on the updated main branches with their original
author attribution; their mixed reliability/feature commits are not merged whole.
No decision to ship layout reuse has been made.

## Validation and limitations

The reliability-only backend passed tidy with unchanged dependencies, the full
Go tests, vet, command builds, and race tests for apply, daemon, hypr, IPC, and TUI.
The companion panel passed 121 Node tests, 21 offscreen Qt tests, qmllint, plugin
validation, and whitespace checks.

Actual panel QML was rendered before/after in an isolated offscreen Quickshell
host at compact 430x730 and expanded 1120x812, plus 960x720 with a synthetic light
palette. The host substitutes only the native window container, disables backend
and brightness IO, and supplies sample displays. Compact/expanded dark captures
preserve the reviewed layout; the connecting state retains its previous display
view. Synthetic light colors are not a full native-theme acceptance test.

These checks do not establish physical projector/dock/lid/suspend recovery,
native layer-surface placement, or TUI/panel feature parity. No live display
settings were changed for this integration, and no new release was published.
