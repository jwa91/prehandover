# Changelog

All notable changes to this project will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com); versions
follow [SemVer](https://semver.org).

## [Unreleased]

## [0.1.1] — 2026-05-13

### Changed

- **Migrate from `brews:` (Formula) to `homebrew_casks:` (Cask)** per
  [ADR 0008 in jwa91/homebrew-tap](https://github.com/jwa91/homebrew-tap/blob/main/docs/adr/0008-one-binary-per-repo-and-homebrew-casks.md).
  GoReleaser deprecated `brews:` in v2.10; the modern shape is
  `homebrew_casks:` with `binaries: […]`. End-user `brew install
  jwa91/tap/prehandover` is unchanged (modern brew auto-detects).
- **Codesign + notarize darwin binaries** via Developer ID Application.
  Required because macOS Tahoe Gatekeeper now blocks unsigned binaries
  delivered through Casks. Local release flow:
  `scripts/codesign.sh` (invoked as goreleaser `builds.hooks.post`) and
  `scripts/notarize-darwin.sh` (invoked by the Makefile `release`
  target) submit each codesigned binary to `xcrun notarytool`. The
  archive sha256 is byte-identical before/after notarization — Apple
  records the binary's CDHash so the Gatekeeper online check passes at
  install time.
- **CI release workflow** switched to `workflow_dispatch`-only — tag
  pushes no longer trigger automatic releases (CI cannot codesign
  without secrets). Canonical release path is `make release
  VERSION=X.Y.Z` locally.
- **`.env.template`** now uses `HOMEBREW_TAP_GITHUB_TOKEN` for the Cask
  commit only; `GITHUB_TOKEN` for the source-repo release is injected
  from `gh auth token` at release time. Adds `MACOS_SIGN_IDENTITY` for
  the codesign step.

## [0.1.0] — earlier

Initial release. See git history.

[Unreleased]: https://github.com/jwa91/prehandover/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/jwa91/prehandover/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/jwa91/prehandover/releases/tag/v0.1.0
