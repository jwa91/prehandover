# Installation

prehandover publishes tagged GitHub Releases as the release source of truth. Homebrew, the standalone installer, and direct downloads consume those release artifacts. `go install` builds from the same module tag.

## Homebrew

```sh
brew install jwa91/tap/prehandover
prehandover --version
```

Update:

```sh
brew update
brew upgrade prehandover
```

Uninstall:

```sh
brew uninstall prehandover
```

## Standalone Installer

Install the latest release to `$HOME/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/jwa91/prehandover/main/scripts/install.sh | sh
```

Inspect the installer before running it:

```sh
curl -fsSLO https://raw.githubusercontent.com/jwa91/prehandover/main/scripts/install.sh
less install.sh
sh install.sh
```

Install a specific version:

```sh
PREHANDOVER_VERSION=0.1.0 sh install.sh
```

Install to a custom directory:

```sh
PREHANDOVER_INSTALL_DIR=/usr/local/bin sh install.sh
```

The installer downloads `checksums.txt` from the same GitHub Release and verifies the selected archive with SHA-256 before copying the binary. Checksums catch transfer or artifact mismatch; they do not replace trusting the GitHub Release publisher.

Update by rerunning the installer. Uninstall by removing the binary:

```sh
rm -f "$HOME/.local/bin/prehandover"
```

## Direct Download

Download the archive for your platform from the [latest release](https://github.com/jwa91/prehandover/releases/latest). Asset names use this shape:

- `prehandover_Darwin_arm64.tar.gz`
- `prehandover_Darwin_x86_64.tar.gz`
- `prehandover_Linux_arm64.tar.gz`
- `prehandover_Linux_x86_64.tar.gz`

Verify and extract:

```sh
asset=prehandover_Linux_x86_64.tar.gz
grep "  $asset$" checksums.txt > "$asset.sha256"
shasum -a 256 -c "$asset.sha256"
tar -xzf "$asset"
./prehandover --version
```

On Linux, use `sha256sum -c "$asset.sha256"` if `shasum` is not installed.

## Go

```sh
go install github.com/jwa91/prehandover/cmd/prehandover@latest
prehandover --version
```

For a pinned install:

```sh
go install github.com/jwa91/prehandover/cmd/prehandover@v0.1.0
```

The Go install path builds from the module tag instead of downloading the GitHub Release archive, so commit/date metadata may be less complete than Homebrew or direct-release installs. The version still resolves from Go module build metadata.
