#!/usr/bin/env bash
# gofmt-strict: like `gofmt -l` but exits non-zero when any file needs formatting.
set -eu
out=$(gofmt -l "$@")
if [ -n "$out" ]; then
  printf 'needs gofmt:\n%s\n' "$out" >&2
  exit 1
fi
