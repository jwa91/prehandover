package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "prehandover.toml")
	manifest := `
[manifest]
project = "test"
moments = ["agent_stop"]
adapters = ["claude"]
required_prehandover = "0.1.0"
`
	full := body + manifest
	if idx := strings.Index(body, "[[checks]]"); idx >= 0 {
		full = body[:idx] + manifest + body[idx:]
	}
	if err := os.WriteFile(p, []byte(full), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_Defaults(t *testing.T) {
	p := write(t, `
[[checks]]
id = "x"
entry = "true"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Budget.Duration != 5*time.Second {
		t.Errorf("default budget = %s, want 5s", cfg.Budget.Duration)
	}
	if cfg.OnUnchanged != "skip" {
		t.Errorf("default on_unchanged = %q, want skip", cfg.OnUnchanged)
	}
	if cfg.Parallelism != "auto" {
		t.Errorf("default parallelism = %q, want auto", cfg.Parallelism)
	}
	if cfg.Checks[0].Budget.Duration != 5*time.Second {
		t.Errorf("check budget should inherit global, got %s", cfg.Checks[0].Budget.Duration)
	}
	if cfg.Manifest.Project != "test" {
		t.Errorf("manifest project = %q", cfg.Manifest.Project)
	}
}

func TestLoad_PatternShapes(t *testing.T) {
	p := write(t, `
[[checks]]
id = "a"
entry = "true"
files = "\\.go$"

[[checks]]
id = "b"
entry = "true"
files = { glob = "**/*.ts" }

[[checks]]
id = "c"
entry = "true"
files = { glob = ["**/*.ts", "**/*.tsx"] }

[[checks]]
id = "d"
entry = "true"
exclude = { regex = "vendor/" }
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Checks[0].Files.Regex != "\\.go$" {
		t.Errorf("check[0] regex = %q", cfg.Checks[0].Files.Regex)
	}
	if got := cfg.Checks[1].Files.Globs; len(got) != 1 || got[0] != "**/*.ts" {
		t.Errorf("check[1] globs = %v", got)
	}
	if got := cfg.Checks[2].Files.Globs; len(got) != 2 {
		t.Errorf("check[2] globs = %v", got)
	}
	if cfg.Checks[3].Exclude.Regex != "vendor/" {
		t.Errorf("check[3] exclude regex = %q", cfg.Checks[3].Exclude.Regex)
	}
}

func TestLoad_PassFilenames(t *testing.T) {
	p := write(t, `
[[checks]]
id = "default"
entry = "true"

[[checks]]
id = "explicit_false"
entry = "true"
pass_filenames = false

[[checks]]
id = "explicit_true"
entry = "true"
pass_filenames = true

[[checks]]
id = "limit"
entry = "true"
pass_filenames = 5
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		idx  int
		want bool
	}{
		{0, true},  // default
		{1, false}, // explicit false
		{2, true},  // explicit true
		{3, true},  // int → enabled
	}
	for _, c := range cases {
		got := cfg.Checks[c.idx].PassFilenames.Effective()
		if got != c.want {
			t.Errorf("check[%d] effective pass_filenames = %v, want %v", c.idx, got, c.want)
		}
	}
	if cfg.Checks[3].PassFilenames.Limit != 5 {
		t.Errorf("check[3] limit = %d, want 5", cfg.Checks[3].PassFilenames.Limit)
	}
}

func TestLoad_BudgetParsing(t *testing.T) {
	p := write(t, `
budget = "10s"

[[checks]]
id = "fast"
entry = "true"
budget = "250ms"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Budget.Duration != 10*time.Second {
		t.Errorf("global budget = %s", cfg.Budget.Duration)
	}
	if cfg.Checks[0].Budget.Duration != 250*time.Millisecond {
		t.Errorf("check budget = %s", cfg.Checks[0].Budget.Duration)
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	p := write(t, `
budget = "not-a-duration"

[[checks]]
id = "x"
entry = "true"
`)
	if _, err := Load(p); err == nil {
		t.Error("expected error on bad duration")
	}
}

func TestLoad_InvalidPatternShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "pattern as int",
			body: `
[[checks]]
id = "x"
entry = "true"
files = 5
`,
		},
		{
			name: "pattern as bool",
			body: `
[[checks]]
id = "x"
entry = "true"
files = true
`,
		},
		{
			name: "glob as int",
			body: `
[[checks]]
id = "x"
entry = "true"
files = { glob = 7 }
`,
		},
		{
			name: "glob list with non-string element",
			body: `
[[checks]]
id = "x"
entry = "true"
files = { glob = ["**/*.ts", 3] }
`,
		},
		{
			name: "regex as int",
			body: `
[[checks]]
id = "x"
entry = "true"
files = { regex = 9 }
`,
		},
		{
			name: "unknown pattern key",
			body: `
[[checks]]
id = "x"
entry = "true"
files = { pattern = "**/*.go" }
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, tc.body)
			if _, err := Load(p); err == nil {
				t.Errorf("expected error for invalid pattern shape")
			}
		})
	}
}

func TestLoad_InvalidPassFilenamesShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "string value",
			body: `
[[checks]]
id = "x"
entry = "true"
pass_filenames = "yes"
`,
		},
		{
			name: "zero",
			body: `
[[checks]]
id = "x"
entry = "true"
pass_filenames = 0
`,
		},
		{
			name: "negative int",
			body: `
[[checks]]
id = "x"
entry = "true"
pass_filenames = -1
`,
		},
		{
			name: "table value",
			body: `
[[checks]]
id = "x"
entry = "true"
pass_filenames = { limit = 5 }
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, tc.body)
			if _, err := Load(p); err == nil {
				t.Errorf("expected error for invalid pass_filenames shape")
			}
		})
	}
}

func TestLoad_ParallelismValid(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "auto",
			body: `
parallelism = "auto"

[[checks]]
id = "x"
entry = "true"
`,
			want: "auto",
		},
		{
			name: "positive int string",
			body: `
parallelism = "4"

[[checks]]
id = "x"
entry = "true"
`,
			want: "4",
		},
		{
			name: "default when empty",
			body: `
[[checks]]
id = "x"
entry = "true"
`,
			want: "auto",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, tc.body)
			cfg, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Parallelism != tc.want {
				t.Errorf("parallelism = %q, want %q", cfg.Parallelism, tc.want)
			}
		})
	}
}

func TestLoad_ParallelismInvalid(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "non-numeric",
			body: `
parallelism = "many"

[[checks]]
id = "x"
entry = "true"
`,
		},
		{
			name: "zero",
			body: `
parallelism = "0"

[[checks]]
id = "x"
entry = "true"
`,
		},
		{
			name: "negative",
			body: `
parallelism = "-2"

[[checks]]
id = "x"
entry = "true"
`,
		},
		{
			name: "float string",
			body: `
parallelism = "1.5"

[[checks]]
id = "x"
entry = "true"
`,
		},
		{
			name: "mixed",
			body: `
parallelism = "4cores"

[[checks]]
id = "x"
entry = "true"
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, tc.body)
			if _, err := Load(p); err == nil {
				t.Errorf("expected error for invalid parallelism")
			}
		})
	}
}

func TestLoad_ManifestRequired(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prehandover.toml")
	if err := os.WriteFile(p, []byte(`
[[checks]]
id = "x"
entry = "true"
`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Error("expected error when manifest is missing")
	}
}
