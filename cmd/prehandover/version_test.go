package main

import (
	"testing"

	"github.com/jwa91/prehandover/internal/version"
)

func TestVersionLine(t *testing.T) {
	oldCurrent := version.Current
	oldCommit := version.Commit
	oldDate := version.Date
	t.Cleanup(func() {
		version.Current = oldCurrent
		version.Commit = oldCommit
		version.Date = oldDate
	})

	version.Current = "1.2.3"
	version.Commit = "abcdef0"
	version.Date = "2026-05-12T10:11:12Z"

	got := versionLine()
	want := "prehandover 1.2.3 (abcdef0, 2026-05-12T10:11:12Z)"
	if got != want {
		t.Fatalf("versionLine() = %q, want %q", got, want)
	}
}

func TestCompareVersionsAllowsDevelopmentBuilds(t *testing.T) {
	if compareVersions("dev", "99.0.0") <= 0 {
		t.Fatal("development builds should satisfy manifest version checks")
	}
}

func TestCompareVersionsNormalizesTagsAndPrereleases(t *testing.T) {
	if compareVersions("v0.2.0", "0.1.0") <= 0 {
		t.Fatal("v-prefixed versions should compare by numeric version")
	}
	if compareVersions("0.2.0-rc.1", "0.2.0") != 0 {
		t.Fatal("prerelease suffix should not break numeric comparison")
	}
}
