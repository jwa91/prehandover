package runner

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jwa91/prehandover/internal/config"
)

func dur(d time.Duration) config.Duration { return config.Duration{Duration: d} }

func cfg(t *testing.T, total time.Duration, checks ...config.Check) *config.Config {
	t.Helper()
	return &config.Config{
		Budget:      dur(total),
		Parallelism: "auto",
		OnUnchanged: "skip",
		Checks:      checks,
	}
}

func find(t *testing.T, results []Result, id string) Result {
	t.Helper()
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no result with id %q in %+v", id, results)
	return Result{}
}

func TestExecute_AllPass(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{ID: "a", Entry: "true", AlwaysRun: true},
		config.Check{ID: "b", Entry: "true", AlwaysRun: true},
	)
	run, err := Execute(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusPass {
		t.Errorf("status = %s, want pass", run.Status)
	}
	if len(run.Results) != 2 {
		t.Errorf("got %d results", len(run.Results))
	}
}

func TestExecute_OneFail(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{ID: "ok", Entry: "true", AlwaysRun: true},
		config.Check{ID: "bad", Entry: "false", AlwaysRun: true},
	)
	run, err := Execute(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusFail {
		t.Errorf("status = %s, want fail", run.Status)
	}
	if find(t, run.Results, "bad").Status != StatusFail {
		t.Errorf("bad check should fail")
	}
}

func TestExecute_Timeout(t *testing.T) {
	c := cfg(t, 2*time.Second,
		config.Check{
			ID: "slow", Entry: "sleep", Args: []string{"5"},
			AlwaysRun: true,
			Budget:    dur(200 * time.Millisecond),
		},
	)
	start := time.Now()
	run, err := Execute(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		t.Errorf("timeout should kill quickly, took %s", elapsed)
	}
	if find(t, run.Results, "slow").Status != StatusTimeout {
		t.Errorf("status = %s, want timeout", run.Results[0].Status)
	}
	if run.Status != StatusTimeout {
		t.Errorf("aggregate = %s, want timeout", run.Status)
	}
}

func TestExecute_SkipWhenNoMatchingChanges(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{ID: "ts", Entry: "true", Files: config.Pattern{Globs: []string{"**/*.ts"}}},
	)
	run, err := Execute(context.Background(), c, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if find(t, run.Results, "ts").Status != StatusSkip {
		t.Errorf("status = %s, want skip", run.Results[0].Status)
	}
	if run.Status != StatusPass {
		t.Errorf("aggregate = %s, want pass (skip is not a failure)", run.Status)
	}
}

func TestExecute_AlwaysRunOverridesSkip(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{
			ID: "ts", Entry: "true", AlwaysRun: true,
			Files: config.Pattern{Globs: []string{"**/*.ts"}},
		},
	)
	run, err := Execute(context.Background(), c, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if find(t, run.Results, "ts").Status != StatusPass {
		t.Errorf("always_run should override skip, got %s", run.Results[0].Status)
	}
}

func TestExecute_FailFast(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{ID: "a", Entry: "false", AlwaysRun: true, RequireSerial: true, Priority: 0},
		config.Check{ID: "b", Entry: "false", AlwaysRun: true, RequireSerial: true, Priority: 1},
	)
	c.FailFast = true
	run, err := Execute(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Results) != 1 {
		t.Errorf("fail_fast should stop after first failure, got %d results", len(run.Results))
	}
}

func TestExecute_ShellMode(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{
			ID: "pipe", Entry: "echo hello | grep hello", AlwaysRun: true,
			Shell: "sh",
		},
	)
	run, err := Execute(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if find(t, run.Results, "pipe").Status != StatusPass {
		t.Errorf("shell entry should pass, got %s", run.Results[0].Status)
	}
}

func TestExecute_VerboseShowsOutputOnPass(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{
			ID: "loud", Entry: "echo hello", AlwaysRun: true, Verbose: true,
			Shell: "sh",
		},
	)
	run, err := Execute(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := find(t, run.Results, "loud")
	if r.Status != StatusPass {
		t.Fatalf("status = %s", r.Status)
	}
	if !strings.Contains(r.Output, "hello") {
		t.Errorf("verbose should preserve output, got %q", r.Output)
	}
}

func TestExecute_ConfigError(t *testing.T) {
	c := cfg(t, 5*time.Second,
		config.Check{ID: "", Entry: "true"},
	)
	if _, err := Execute(context.Background(), c, nil); err == nil {
		t.Error("expected error for missing id")
	}
}

func TestCommandEnv_AugmentsSparseHookPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	env := commandEnv(map[string]string{"PATH": "/usr/bin:/bin"})
	pathValue := envValue(env, "PATH")

	wantParts := []string{
		"/usr/bin",
		"/bin",
		filepath.Join(home, "go", "bin"),
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
	}
	for _, want := range wantParts {
		if !pathContains(pathValue, want) {
			t.Fatalf("PATH %q does not contain %q", pathValue, want)
		}
	}
}

func TestSplitEntry(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single token", "true", []string{"true"}},
		{"multi token", "go vet ./...", []string{"go", "vet", "./..."}},
		{"double quoted", `echo "hello world"`, []string{"echo", "hello world"}},
		{"flag", `prettier --check`, []string{"prettier", "--check"}},

		{"escaped space unquoted", `cmd a\ b`, []string{"cmd", "a b"}},
		{"escaped backslash unquoted", `cmd a\\b`, []string{"cmd", `a\b`}},
		{"escaped quote unquoted", `cmd a\"b`, []string{"cmd", `a"b`}},
		{"escaped quote in dquotes", `cmd "a\"b"`, []string{"cmd", `a"b`}},
		{"escaped backslash in dquotes", `cmd "a\\b"`, []string{"cmd", `a\b`}},
		{"single quotes preserve backslash", `cmd 'a\b'`, []string{"cmd", `a\b`}},
		{"single quotes preserve double-backslash", `cmd 'a\\b'`, []string{"cmd", `a\\b`}},
		{"adjacent quoted segments", `cmd "ab"'cd'`, []string{"cmd", "abcd"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitEntry(tc.in)
			if err != nil {
				t.Fatalf("splitEntry(%q): %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("splitEntry(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitEntry(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSplitEntry_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"only whitespace", "   \t"},
		{"unterminated double quote", `echo "hello`},
		{"unterminated single quote", `echo 'hello`},
		{"trailing backslash", `echo foo\`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := splitEntry(tc.in); err == nil {
				t.Errorf("splitEntry(%q): expected error", tc.in)
			}
		})
	}
}

func pathContains(pathValue, want string) bool {
	for _, part := range filepath.SplitList(pathValue) {
		if part == want {
			return true
		}
	}
	return false
}
