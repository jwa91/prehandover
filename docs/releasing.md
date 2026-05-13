# Releasing

Releases are driven by annotated `v*` tags. GitHub Releases are the changelog source of truth; this repo does not keep a `CHANGELOG.md`.

## One-Time Setup

1. Make the `jwa91/prehandover` repository public before the first public tag.
2. Create a fine-grained GitHub PAT with `Contents: read and write` on `jwa91/homebrew-tap` only.
3. Add that PAT to `jwa91/prehandover` as the repository secret `HOMEBREW_TAP_GITHUB_TOKEN`.

This project ships through GoReleaser `homebrew_casks`, matching the rest of the `jwa-*` CLI family. Homebrew still installs it with `brew install jwa91/tap/prehandover`; the tap artifact lives at `Casks/prehandover.rb`.

Prerelease tags such as `v0.1.0-rc.1` publish a GitHub prerelease but do not update the Homebrew tap.

## Pre-Tag Checks

```sh
make check
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

Inspect a snapshot binary:

```sh
tmp="$(mktemp -d)"
tar -xzf dist/prehandover_Darwin_arm64.tar.gz -C "$tmp"
"$tmp/prehandover" --version
```

Use the matching `dist/prehandover_*.tar.gz` archive for your local platform.

## Tag Release

1. Confirm `main` is green.
2. Choose the version using semver.
3. Create and push the annotated tag:

```sh
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

4. Watch `.github/workflows/release.yml`.
5. Confirm the GitHub Release published with four archives and `checksums.txt`.
6. Confirm `Casks/prehandover.rb` landed in `jwa91/homebrew-tap`.
7. Confirm the smoke job passed.

Final sanity on a clean machine:

```sh
brew update
brew install jwa91/tap/prehandover
prehandover --version
```

Also verify the standalone installer:

```sh
curl -fsSL https://raw.githubusercontent.com/jwa91/prehandover/main/scripts/install.sh | sh
prehandover --version
```

And the Go module path:

```sh
go install github.com/jwa91/prehandover/cmd/prehandover@vX.Y.Z
prehandover --version
```

If the release artifact is wrong, the smoke job fails, or the Homebrew token is misconfigured, do not reuse the same tag for a public release. Fix forward and publish the next patch tag.
