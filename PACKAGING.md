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

Current status as of 2026-09-03:

| Channel | Status | Notes |
|---|---|---|
| Arch AUR | Published | Stable [`hyprmoncfg`](https://aur.archlinux.org/packages/hyprmoncfg) and binary [`hyprmoncfg-bin`](https://aur.archlinux.org/packages/hyprmoncfg-bin) are published at 1.17.1. VCS [`hyprmoncfg-git`](https://aur.archlinux.org/packages/hyprmoncfg-git) continues to track `main`. |
| Fedora COPR | Staged | [`paolino/hyprmoncfg`](https://copr.fedorainfracloud.org/coprs/paolino/hyprmoncfg/) build [10939280](https://copr.fedorainfracloud.org/coprs/build/10939280) publishes 1.17.0 for Fedora 44, 45, and rawhide on `x86_64` and `aarch64`. The 1.17.1 spec is ready, but this machine has no COPR credentials or RPM build environment. |
| Nixpkgs | Open PR | The source lives at `pkgs/by-name/hy/hyprmoncfg`; the package is available as `pkgs.hyprmoncfg` and `nixpkgs#hyprmoncfg`. The [1.17.1 update](https://github.com/NixOS/nixpkgs/pull/552223) is under review with all CI checks passing. |
| Gentoo GURU | Published | [`gui-apps/hyprmoncfg` 1.17.1](https://github.com/gentoo/guru/tree/dev/gui-apps/hyprmoncfg) is published on the required GURU `dev` branch in hardware-OpenPGP-signed, DCO-compliant commit `4e9e9cba`. |
| Void Linux official | Blocked | A local `hyprmoncfg` template exists, but official submission is not useful while Hyprland is not in Void. Multiple Hyprland package requests and PRs have been closed upstream, and the current Void maintainer stance is that Hyprland is not planned. |
| Void Blackhole-vl | Open PR | [Blackhole-vl](https://github.com/Event-Horizon-VL/blackhole-vl) publishes `hyprland` and `hyprmoncfg` packages outside official Void. Version 1.17.0 merged in [PR 283](https://github.com/Event-Horizon-VL/blackhole-vl/pull/283); [PR 284](https://github.com/Event-Horizon-VL/blackhole-vl/pull/284) updates it to 1.17.1 with all package-build checks passing. |
| Alpine aports | Open MR | [`alpine/aports!103051`](https://gitlab.alpinelinux.org/alpine/aports/-/merge_requests/103051) targets 1.17.1 with a successful pipeline; no package is in the Alpine package index yet. |
| Debian and Ubuntu | Sponsor-ready | The 1.17.1 [`debian/sid` branch and signed upstream tag are published on Salsa](https://salsa.debian.org/crmne/hyprmoncfg), and the full Go test suite passes from the release payload. Official inclusion still needs Debian policy review and the sponsor/upload flow. |
| openSUSE OBS | Staged | [`home:paolino/hyprmoncfg`](https://build.opensuse.org/package/show/home:paolino/hyprmoncfg) revision 3 publishes 1.15.1 for openSUSE Tumbleweed on `x86_64`; the validated 1.17.1 payload is staged, but this machine has no `osc` credentials. |
| SlackBuilds.org | Staged | The 1.17.1 SlackBuild payload is ready for manual submission after validation on a fully patched Slackware 15.0 system. |

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
