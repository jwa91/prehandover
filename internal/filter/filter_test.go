package filter

import (
	"testing"

	"github.com/jwa/prehandover/internal/config"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		name    string
		include config.Pattern
		exclude config.Pattern
		path    string
		want    bool
	}{
		{"no_filter_matches_all", config.Pattern{}, config.Pattern{}, "any/path.txt", true},
		{"include_glob_match", config.Pattern{Globs: []string{"**/*.ts"}}, config.Pattern{}, "src/foo.ts", true},
		{"include_glob_miss", config.Pattern{Globs: []string{"**/*.ts"}}, config.Pattern{}, "src/foo.go", false},
		{"include_regex_match", config.Pattern{Regex: "\\.go$"}, config.Pattern{}, "main.go", true},
		{"include_regex_miss", config.Pattern{Regex: "\\.go$"}, config.Pattern{}, "main.ts", false},
		{"exclude_glob", config.Pattern{}, config.Pattern{Globs: []string{"vendor/**"}}, "vendor/foo.go", false},
		{"exclude_glob_pass", config.Pattern{}, config.Pattern{Globs: []string{"vendor/**"}}, "src/foo.go", true},
		{"include_and_exclude", config.Pattern{Globs: []string{"**/*.go"}}, config.Pattern{Globs: []string{"vendor/**"}}, "vendor/foo.go", false},
		{"multiple_includes_any_match", config.Pattern{Globs: []string{"**/*.ts", "**/*.tsx"}}, config.Pattern{}, "src/foo.tsx", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(tc.include, tc.exclude)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.Match(tc.path); got != tc.want {
				t.Errorf("Match(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	m, err := New(config.Pattern{Globs: []string{"**/*.go"}}, config.Pattern{Globs: []string{"vendor/**"}})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"a.go", "b.ts", "vendor/c.go", "internal/d.go"}
	got := m.Filter(paths)
	want := []string{"a.go", "internal/d.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, p := range want {
		if got[i] != p {
			t.Errorf("[%d] got %q, want %q", i, got[i], p)
		}
	}
}

func TestInvalidRegex(t *testing.T) {
	if _, err := New(config.Pattern{Regex: "[invalid"}, config.Pattern{}); err == nil {
		t.Error("expected error on invalid regex")
	}
}
