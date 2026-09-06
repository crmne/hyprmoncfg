# Packaging hyprmoncfg

This repository is the upstream source of truth for release artifacts and shared
packaging assets. Distro-specific package recipes should usually live in the
package repository for that distro, not in this repository.

## Upstream Release Assets

Each tagged release publishes:

- `hyprmoncfg_<version>_linux_amd64.tar.gz`
- `hyprmoncfg_<version>_linux_arm64.tar.gz`
- `hyprmoncfg-<version>-deps.tar.xz`
- `checksums.txt`
- GitHub's automatic source archive for the tag

The binary archives contain:

- `hyprmoncfg`
- `hyprmoncfgd`
- `README.md`
- `LICENSE`
- `packaging/applications/hyprmoncfg.desktop`
- `packaging/applications/hyprmoncfg-omarchy.desktop`
- `packaging/icons/hyprmoncfg.svg`
- `packaging/systemd/hyprmoncfgd.service`
- `packaging/systemd/hyprmoncfgd.local.service`

Source-based packages should avoid fetching Go modules during the package build.
Use a pre-fetched Go module cache tarball or the distro's native Go dependency
mechanism.

## Dependencies

Runtime:

- `hyprland`, specifically `hyprctl` in `PATH`
- `systemd` only for the packaged user service
- UPower is optional; it improves immediate lid-change detection

Build time:

- Go `1.26.1` or newer, matching `go.mod`

## Build From Source

Packagers should set build metadata through `internal/buildinfo`:

```sh
version=1.18.2
commit="$(git rev-parse --short HEAD)"
build_date="$(date -u +%FT%TZ)"
ldflags="-s -w"
ldflags="$ldflags -X github.com/crmne/hyprmoncfg/internal/buildinfo.Version=$version"
ldflags="$ldflags -X github.com/crmne/hyprmoncfg/internal/buildinfo.Commit=$commit"
ldflags="$ldflags -X github.com/crmne/hyprmoncfg/internal/buildinfo.Date=$build_date"

CGO_ENABLED=0 go build -trimpath -mod=readonly -ldflags "$ldflags" -o hyprmoncfg ./cmd/hyprmoncfg
CGO_ENABLED=0 go build -trimpath -mod=readonly -ldflags "$ldflags" -o hyprmoncfgd ./cmd/hyprmoncfgd
go test ./...
```

For offline builds with a Go module cache tarball:

```sh
tar -xf hyprmoncfg-1.18.2-deps.tar.xz
GOMODCACHE="$PWD/go-mod" GOPROXY=off CGO_ENABLED=0 go build -trimpath -mod=readonly ./cmd/hyprmoncfg
```

## Installed Files

Recommended installed files:

```text
/usr/bin/hyprmoncfg
/usr/bin/hyprmoncfgd
/usr/share/applications/hyprmoncfg.desktop
/usr/share/applications/hyprmoncfg-omarchy.desktop
/usr/share/icons/hicolor/scalable/apps/hyprmoncfg.svg
/usr/share/licenses/hyprmoncfg/LICENSE
/usr/share/doc/hyprmoncfg/README.md
```

For systemd-based distros, also install:

```text
/usr/lib/systemd/user/hyprmoncfgd.service
```

Do not enable or start the user service from package scripts. Users should opt in
with:

```sh
systemctl --user enable --now hyprmoncfgd
```

For non-systemd distros, document `exec-once = hyprmoncfgd` in Hyprland config as
the daemon startup path.

## Package Status

Current status as of 2026-09-06, for release **1.18.2**:

| Channel | Status | Notes |
|---|---|---|
| Portable Linux | Published | [Release 1.18.2](https://github.com/crmne/hyprmoncfg/releases/tag/v1.18.2) provides statically linked x86_64 and ARM64 binaries, source, and checksummed offline Go dependencies. The panel is Omarchy-specific; the TUI and daemon work independently of it. |
| Arch AUR | Published | [`hyprmoncfg`](https://aur.archlinux.org/packages/hyprmoncfg) and [`hyprmoncfg-bin`](https://aur.archlinux.org/packages/hyprmoncfg-bin) are published at 1.18.2-1. [`hyprmoncfg-git`](https://aur.archlinux.org/packages/hyprmoncfg-git) tracks `main`; its displayed metadata version does not pin the checkout. |
| Fedora COPR | Awaiting credentials | [`paolino/hyprmoncfg`](https://copr.fedorainfracloud.org/coprs/paolino/hyprmoncfg/) still publishes 1.17.0 ([build 10939280](https://copr.fedorainfracloud.org/coprs/build/10939280)). The 1.18.2 spec and source RPM are prepared; publication needs COPR credentials. |
| Nixpkgs | Awaiting human review | The 1.18.2 update for [PR 552223](https://github.com/NixOS/nixpkgs/pull/552223) is prepared locally. A sandboxed x86_64 build, the upstream tests, and install/version checks pass. Human review of the change and restored PR template is required before submission under Nixpkgs' AI contribution policy; the remote PR still targets 1.17.1. |
| Gentoo GURU | Awaiting signing key | The 1.18.2 ebuild and manifest are staged and `pkgcheck scan --net` passes. Publishing to GURU's `dev` branch requires the hardware OpenPGP key; the signing prompt timed out. The published package remains 1.17.1. |
| Void Linux official | Blocked upstream | The local template targets 1.18.2, but official submission remains blocked by the absence of Hyprland in official Void. |
| Void Blackhole-vl | Open PR | [PR 288](https://github.com/Event-Horizon-VL/blackhole-vl/pull/288) targets 1.18.2. Its x86_64 and ARM64 package builds pass for both glibc and musl. Maintainer merge/publication remains pending. |
| Alpine aports | Open MR | [aports!103051](https://gitlab.alpinelinux.org/alpine/aports/-/merge_requests/103051) now targets 1.18.2. Lint and the supported x86_64/ARM64 builds pass in [pipeline 469594](https://gitlab.alpinelinux.org/crmne/aports/-/pipelines/469594); an unrelated architecture job remains queued. No official Alpine package is published yet. |
| Debian and Ubuntu | Sponsor-ready source | The 1.18.2 [`debian/sid` branch and upstream tag are on Salsa](https://salsa.debian.org/crmne/hyprmoncfg). Source artifacts were generated and the release payload passes the full Go tests/build offline. A native Debian package build, policy review, and sponsor/upload flow remain; no official Debian/Ubuntu binary package is claimed. |
| openSUSE OBS | Awaiting credentials | The 1.18.2 spec and source RPM are staged for [`home:paolino/hyprmoncfg`](https://build.opensuse.org/package/show/home:paolino/hyprmoncfg). Publishing needs OBS credentials; no new OBS build was submitted. The last recorded published version is 1.15.1. |
| SlackBuilds.org | Awaiting native validation | The 1.18.2 SlackBuild payload is staged. Manual submission requires validation on a fully patched Slackware 15.0 system. |

Distro-specific recipes should remain in the distro package repository or the
external packaging workspace until they are accepted upstream. Keep this
repository limited to release assets and shared packaging files.

## Smoke Tests

After packaging, run:

```sh
hyprmoncfg version
hyprmoncfg --help
hyprmoncfgd --help
test -f /usr/share/applications/hyprmoncfg.desktop
test -f /usr/share/applications/hyprmoncfg-omarchy.desktop
test -f /usr/share/icons/hicolor/scalable/apps/hyprmoncfg.svg
```

In a real Hyprland session, also verify:

```sh
hyprmoncfg list
systemctl --user daemon-reload
systemctl --user status hyprmoncfgd
```
