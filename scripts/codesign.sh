#!/usr/bin/env bash
# Sign + notarize one Go binary. Called by goreleaser as a
# `builds.hooks.post` step with the build target and binary path.
# Non-darwin targets are ignored.
set -euo pipefail

target="${1:?usage: codesign.sh <target> <binary-path>}"
binary="${2:?usage: codesign.sh <target> <binary-path>}"
profile="${NOTARYTOOL_PROFILE:-notarytool}"

case "$target" in
  darwin_*)
    if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
      echo "codesign: MACOS_SIGN_IDENTITY unset — skipping signing/notarization"
      exit 0
    fi

    command -v xcrun >/dev/null || {
      echo "codesign: xcrun missing; install Xcode Command Line Tools" >&2
      exit 1
    }
    xcrun notarytool --help >/dev/null 2>&1 || {
      echo "codesign: xcrun notarytool unavailable" >&2
      exit 1
    }

    # Fail fast if the configured keychain profile is unavailable.
    xcrun notarytool history --keychain-profile "$profile" >/dev/null

    codesign --force --options runtime --timestamp \
      --sign "$MACOS_SIGN_IDENTITY" "$binary"
    echo "codesign: signed $binary"

    zip="$(mktemp -t prehandover-notarize.XXXXXX).zip"
    ditto -c -k --sequesterRsrc "$binary" "$zip"
    echo "notarytool: submitting $binary via $zip (profile: $profile)"
    xcrun notarytool submit "$zip" --keychain-profile "$profile" --wait
    rm -f "$zip"
    echo "notarytool: accepted $binary"
    ;;
  *)
    : # non-darwin, nothing to do
    ;;
esac
