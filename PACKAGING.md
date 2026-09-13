# Packaging hyprmoncfg

This repository owns the application, shared installation assets, distribution
recipes, and release automation. Edit packaging here and export the generated
recipes to the distribution's publishing repository.

The [native-packages](https://rubygems.org/gems/native-packages) gem builds binary
packages and handles downstream repositories from `native-packages.yaml`.
GoReleaser builds the Linux archives and offline Go dependency archive.
Hyprmoncfg keeps the Go/Nix source-recipe generator in `scripts/package_sources.rb`;
it uses the same gem's release helpers. No packaging Gemfile or wrapper is needed.

## Layout

| Path | Purpose |
|---|---|
| `.goreleaser.yml` | Linux binary and offline dependency archives |
| `packaging/applications`, `icons`, `systemd` | Shared installation assets |
| `packaging/arch` | Shared AUR build/install logic for stable, binary, and Git packages |
| `packaging/debian` | Debian/Ubuntu source packaging, including changelog history |
| `packaging/rpm` | One source RPM spec for Fedora COPR and openSUSE OBS |
| `packaging/alpine`, `void`, `slackware` | Native source recipes |
| `packaging/gentoo` | Source and binary ebuilds and maintainer metadata |
| `packaging/nix` | Nix source package |
| `native-packages.yaml` | Tool versions, DEB/RPM targets and contents, downstream repositories |
| `scripts/package_sources.rb` | App-specific Go/Nix source-recipe generation |

Files ending in `.in` are templates. Release versions, commits, Go requirements,
and checksums come from the requested tag and its published assets. There is no
second version number to maintain. AUR variants share their metadata and build
logic; the generator emits standalone `PKGBUILD` and `.SRCINFO` files.

Binary packages go under `dist/packages/<version>/`; use
`native-packages publish --from dist/packages/<version> --to github` to attach a
verified build to its release. Local archive inputs under `dist/` can be packaged
with `native-packages build --version VERSION`. Generated source recipes go under
ignored `dist/packaging/<version>/`; source-recipe downloads are
cached under `.cache/packaging/<version>/`. Managed downstream checkouts live in
`.cache/packaging/repos/`, with pending update records in `.cache/packaging/state/`.
These directories are ignored by Git.

## Updating every package

After the GitHub release has finished publishing:

```sh
git fetch origin --tags
gem install native-packages --version 0.2.0
native-packages validate
native-packages build --release v1.18.3
ruby scripts/package_sources.rb prepare 1.18.3
```

Requirements: Ruby 3.2+, native-packages 0.2.0, nFPM 2.47.0, Git, curl, `bsdtar`, `readelf`, Go at least
as new as the release's `go.mod`, and Nix (`nix hash path`, without a Nix daemon). Arch's `makepkg` is
optional locally and adds native `.SRCINFO` validation. The Packaging GitHub
Actions workflow provides the required tools if you prefer to run this in CI.
Install the test dependency and run the application's source-recipe tests with:

```sh
gem install minitest --version 6.0.6
ruby scripts/packages_test.rb
```

The shared repository runs the repository-client tests, using temporary local
Git repositories and fake API clients without publishing to real destinations.

The source-recipe command downloads the source, both binary archives, and offline Go modules;
verifies the release assets against `checksums.txt`; computes distro checksums
and Gentoo manifests; calculates Nix's vendor hash from an offline `go mod vendor`;
and generates every recipe. GitHub's automatic source archive is downloaded over
HTTPS and hashed separately because it is not listed in the release checksums.
The release commit and dates come from the local tag. Fetch tags before use.

All files are prepared and checked in a temporary directory before the result
appears. Existing output directories are refused, so use `--output` to compare
another rendering after editing templates:

```sh
ruby scripts/package_sources.rb prepare 1.18.3 --output dist/packaging-review
ruby scripts/package_sources.rb check dist/packaging-review
```

The output includes `release.json` with all versions, URLs, and hashes. Recipes
reference the published release assets and can be used without the sibling
packaging workspaces. Nix's `default.nix` can be evaluated with
`pkgs.callPackage ./default.nix { }` using a Nixpkgs version with a sufficiently
new Go compiler.

## Tracking downstream repositories

[`native-packages.yaml`](native-packages.yaml) is the registry of
publishing destinations. It records the public upstream URL, authenticated push
URL, fork where applicable, target branch, package paths, version extraction, and
publishing method. Credentials stay in your SSH agent or environment.

Each destination keeps its own Git history in an ignored checkout. Inside that
checkout, `upstream` points to the distribution repository and `origin` points to
your fork or the directly writable package repository. The application repository
does not need extra remotes or submodules. Existing sibling checkouts are untouched.

```sh
native-packages repositories
native-packages status
native-packages status aur
native-packages status --json
native-packages status --offline
```

Live status fetches upstream recipe versions and lists open package PRs/MRs,
alongside locally prepared or pushed updates. It reports failed reads as unknown
and exits unsuccessfully if a remote or API cannot be checked. Offline status uses
cached Git refs and omits live request queries. A recipe in Git does not establish
that a distribution has built or published a binary package. Service destinations
such as COPR and OBS are listed with their URLs and remaining steps; their build
status is not queried.

GitHub request discovery needs the `gh` CLI authenticated for read access. Alpine
request discovery uses GitLab's public API. Initial Git checkouts are shallow and
sparse to keep large repositories manageable; fetch more history or disable sparse
checkout in the managed directory when native tooling needs the full tree.

## Staging and publishing

Generate recipes once, then choose one target, the `aur` group, or `all`:

```sh
ruby scripts/package_sources.rb prepare 1.18.3
native-packages stage all dist/packaging/1.18.3
native-packages diff aur
native-packages diff nixpkgs
```

`stage` fetches the destination, creates a local working branch, and stages the
mapped files. It makes no commits or remote writes. Manual destinations print
their remaining steps. An existing open request from the configured fork is reused
so its branch and review history survive a release update. If several requests
match, set `proposal_branch` in the registry to the one you intend to update.

Review the diff and run the destination's native checks. Changes from downstream
maintainers may need to be carried into the central templates or reconciled in
the staged recipe. You can edit files inside `.cache/packaging/repos/<target>/`
and use `git add` there before publishing. New Gentoo ebuilds and DIST entries are
added while older ebuilds, Manifest entries, and unrelated files are retained.

AUR publication uses your Git identity and AUR SSH credentials:

```sh
native-packages publish aur
```

This commits and pushes changed recipes to the three independent AUR repositories.
Use `aur-source`, `aur-bin`, or `aur-git` to target just one package. The compatibility
command `native-packages publish-aur dist/packaging/1.18.3` performs both
staging and publication for release CI.

For Nixpkgs, Alpine, or Blackhole, staging writes a submission draft under
`.cache/packaging/submissions/`. Review it, update the native validation results,
and pass it explicitly:

```sh
native-packages publish nixpkgs --body-file .cache/packaging/submissions/nixpkgs.md
native-packages publish alpine --body-file .cache/packaging/submissions/alpine.md
native-packages publish blackhole --body-file .cache/packaging/submissions/blackhole.md
```

GitHub publishing needs `gh` authentication and SSH access to the configured fork.
Alpine publishing needs SSH access to `crmne/aports` and an `ALPINE_GITLAB_TOKEN`
environment variable with API access to that fork. These commands push the branch,
then create or update its request; repeat runs reuse the existing request. Submit
each PR/MR target individually with its own description. The draft includes the
previous request body or repository template, which must be checked for stale
claims. Follow the destination's contribution instructions linked in the registry,
including [Nixpkgs' contribution rules](https://github.com/NixOS/nixpkgs/blob/master/CONTRIBUTING.md).

GURU publishes directly to `dev`, with signed commits, signoff, and signed pushes:

```sh
native-packages diff guru
# Run native package validation before publishing.
native-packages publish guru
```

Configure your GURU identity, SSH access, and OpenPGP signing key first. Follow the
[GURU contributor instructions](https://wiki.gentoo.org/wiki/Project:GURU/Information_for_Contributors)
and run `pkgcheck` in an appropriate Gentoo environment. Status reads GURU's
`master` branch, so an update pushed to `dev` can still be pending there.

Publication refuses unstaged changes, untracked files, changes outside the staged
package paths, and unexpected remote advances. It never force-pushes. If someone
updates the destination after staging, commit your reviewed changes in the managed
checkout, fetch `origin`, and rebase onto the destination branch reported by the
error before retrying. Sign your manual commits when the destination requires it.
Conflicts need review. Already pushed targets are safe to retry if a later push or
request fails; publication across several repositories is not atomic.

Staging the same pending release again preserves local work. A different pending
release or edits after publication must be resolved before staging another update.
Keep managed checkouts and their staging records together; they contain any
unpublished work.

### Destinations with native upload workflows

The registry also tracks destinations whose upload process needs additional tools:

| Destination | Generated payload | Remaining publishing step |
|---|---|---|
| Fedora COPR / openSUSE OBS | `rpm/hyprmoncfg.spec` plus cached source/deps archives | Build an SRPM with `rpmbuild -bs` and submit with authenticated `copr-cli`, or upload the spec and sources with `osc` |
| Debian Salsa / mentors | `debian/` plus source/deps archives | Import the upstream tag with `git-buildpackage`, build/sign the native source package, and follow the sponsorship/upload process |
| Void Linux official | `void/template` | Resolve the Hyprland dependency requirement; Blackhole is the active submission target |
| SlackBuilds.org | `slackware/` | Validate on supported Slackware and submit the payload |

For automated Git destinations, native validation still includes `abuild` for
Alpine, `xbps-src` for Blackhole, `pkgcheck` and a package build for GURU, and the
Nixpkgs package build and contribution checks. The generator's syntax and checksum
checks do not replace these builds.

The previous `hyprmoncfg-packaging`, `distro-submissions`, AUR, and distribution
checkouts were the migration inputs. They can remain as historical workspaces;
future recipe edits and destination configuration belong here.

## CI

Push a stable `vX.Y.Z` tag through the normal release process. GoReleaser publishes
Linux archives, offline dependencies and `checksums.txt`. The Packaging workflow
then invokes the pinned native-packages workflow for amd64/arm64 DEB/RPM files
and prepares the source recipes with the Go/Nix generator. A following release
job publishes the verified binary packages and the source-recipe archive.

`packaging-checksums.txt` covers the binary packages;
`source-packaging-checksums.txt` covers `hyprmoncfg-<version>-packaging.tar.xz`.
The original `checksums.txt` is preserved. No follow-up version commit or AI
session is needed for packaging updates.

Packaging changes also run the application generator tests, validate native shell
recipes, compare AUR metadata with `makepkg`, and build/inspect snapshot Debian
and RPM packages in CI. The Packaging workflow can be dispatched with a published
version to regenerate recipes or retry generation independently of releasing.
These checks do not replace each distribution's native package build and review.

To publish AUR packages automatically after releases, configure repository secrets
`AUR_SSH_KEY` and `AUR_KNOWN_HOSTS`
(the verified AUR host-key entry), then set repository variable `PUBLISH_AUR=true`.
Generation and binary package publication work without those credentials.

## Upstream Release Assets

Each tagged release publishes:

- `hyprmoncfg_<version>_linux_amd64.tar.gz`
- `hyprmoncfg_<version>_linux_arm64.tar.gz`
- `hyprmoncfg-<version>-deps.tar.xz`
- Native `.deb` and `.rpm` packages for amd64/x86_64 and arm64/aarch64
- `checksums.txt`
- `hyprmoncfg-<version>-packaging.tar.xz`, `packaging-checksums.txt` and `source-packaging-checksums.txt` after stable release packaging succeeds
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
