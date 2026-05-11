package runner

import (
	"context"
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

func TestSplitEntry(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"true", []string{"true"}},
		{"go vet ./...", []string{"go", "vet", "./..."}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{`prettier --check`, []string{"prettier", "--check"}},
	}
	for _, tc := range cases {
		got, err := splitEntry(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q: got %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: [%d] got %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
