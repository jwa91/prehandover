#!/usr/bin/env bash
# Codesign one Go binary with Developer ID. Called by goreleaser as a
# `builds.hooks.post` step with the build target and binary path. No-op on
# non-darwin or when MACOS_SIGN_IDENTITY is unset (so unsigned CI builds
# still pass — until CI has signing creds).
set -euo pipefail

target="${1:?usage: codesign.sh <target> <binary-path>}"
binary="${2:?usage: codesign.sh <target> <binary-path>}"

case "$target" in
  darwin_*)
    if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
      echo "codesign: MACOS_SIGN_IDENTITY unset — skipping (unsigned build)"
      exit 0
    fi
    codesign --force --options runtime --timestamp \
      --sign "$MACOS_SIGN_IDENTITY" "$binary"
    echo "codesign: signed $binary"
    ;;
  *)
    : # non-darwin, nothing to do
    ;;
esac
