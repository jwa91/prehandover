#!/usr/bin/env bash
# Submit each darwin Go binary to notarytool. Apple records the binary's
# CDHash; Gatekeeper online-check at `brew install` time then passes.
# The distribution .tar.gz archive is byte-identical before/after, so the
# Cask sha256 in the tap does not change — no re-upload.
#
# Requires:
#   - `xcrun notarytool` (Xcode CLT)
#   - A keychain profile named "notarytool" set up via:
#       xcrun notarytool store-credentials "notarytool" \
#         --apple-id <id> --team-id <team> --password <app-specific-pwd>
set -euo pipefail

project="${1:?usage: notarize-darwin.sh <project-name> <version>}"
version="${2:?usage: notarize-darwin.sh <project-name> <version>}"

command -v xcrun >/dev/null || {
  echo "xcrun not found — install Xcode Command Line Tools" >&2
  exit 1
}
xcrun notarytool --help >/dev/null 2>&1 || {
  echo "xcrun notarytool not available" >&2
  exit 1
}

for arch in amd64 arm64; do
  case "$arch" in
    amd64) dir="dist/${project}_darwin_${arch}_v1" ;;
    arm64) dir="dist/${project}_darwin_${arch}_v8.0" ;;
  esac
  bin="$dir/$project"
  if [ ! -x "$bin" ]; then
    echo "skipping $arch: $bin not found"
    continue
  fi
  zip="dist/${project}_${version}_darwin_${arch}.notarize.zip"
  # zip just the binary (no path prefix); notarytool only needs a Mach-O
  # to register the CDHash with Apple.
  ditto -c -k --sequesterRsrc "$bin" "$zip"
  echo "==> notarizing $bin via $zip"
  xcrun notarytool submit "$zip" --keychain-profile notarytool --wait
  rm -f "$zip"
done
echo "notarization complete — archive sha256s unchanged"
