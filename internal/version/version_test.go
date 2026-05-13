package version

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		build   *debug.BuildInfo
		ok      bool
		want    string
	}{
		{
			name:    "ldflags value wins",
			current: "0.2.0",
			build:   &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			ok:      true,
			want:    "0.2.0",
		},
		{
			name:    "go install module version fallback",
			current: "dev",
			build:   &debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}},
			ok:      true,
			want:    "0.3.0",
		},
		{
			name:    "source build remains dev",
			current: "dev",
			build:   &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			ok:      true,
			want:    "dev",
		},
	}

	oldCurrent := Current
	t.Cleanup(func() { Current = oldCurrent })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = tt.current
			if got := resolveVersion(tt.build, tt.ok); got != tt.want {
				t.Fatalf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveBuildMetadata(t *testing.T) {
	build := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "0123456789abcdef"},
		{Key: "vcs.time", Value: "2026-05-12T10:11:12Z"},
	}}

	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Commit = oldCommit
		Date = oldDate
	})
	Commit = "unknown"
	Date = "unknown"

	if got := resolveCommit(build, true); got != "0123456789ab" {
		t.Fatalf("resolveCommit() = %q, want short revision", got)
	}
	if got := resolveDate(build, true); got != "2026-05-12T10:11:12Z" {
		t.Fatalf("resolveDate() = %q, want vcs time", got)
	}
}
