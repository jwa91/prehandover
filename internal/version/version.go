package version

import (
	"runtime/debug"
	"strings"
)

// These values are set by release builds with -ldflags. Source and go install
// builds fall back to Go build metadata where available.
var Current = "dev"
var Commit = "unknown"
var Date = "unknown"

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func Info() BuildInfo {
	build, ok := debug.ReadBuildInfo()
	return BuildInfo{
		Version: resolveVersion(build, ok),
		Commit:  resolveCommit(build, ok),
		Date:    resolveDate(build, ok),
	}
}

func Version() string {
	return Info().Version
}

func resolveVersion(build *debug.BuildInfo, ok bool) string {
	if v := cleanVersion(Current); v != "" {
		return v
	}
	if ok {
		if v := cleanVersion(build.Main.Version); v != "" {
			return v
		}
	}
	return Current
}

func resolveCommit(build *debug.BuildInfo, ok bool) string {
	if !unknown(Commit) {
		return Commit
	}
	if ok {
		for _, setting := range build.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				if len(setting.Value) > 12 {
					return setting.Value[:12]
				}
				return setting.Value
			}
		}
	}
	return Commit
}

func resolveDate(build *debug.BuildInfo, ok bool) string {
	if !unknown(Date) {
		return Date
	}
	if ok {
		for _, setting := range build.Settings {
			if setting.Key == "vcs.time" && setting.Value != "" {
				return setting.Value
			}
		}
	}
	return Date
}

func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" || v == "(devel)" {
		return ""
	}
	return strings.TrimPrefix(v, "v")
}

func unknown(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == "unknown"
}
