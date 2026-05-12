package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jwa91/prehandover/internal/runner"
)

func sampleRun() *runner.Run {
	return &runner.Run{
		Status:   runner.StatusFail,
		Duration: 1500 * time.Millisecond,
		Budget:   5 * time.Second,
		Results: []runner.Result{
			{
				ID:       "lint",
				Status:   runner.StatusPass,
				Duration: 100 * time.Millisecond,
				Budget:   1 * time.Second,
			},
			{
				ID:       "vet",
				Status:   runner.StatusFail,
				Duration: 200 * time.Millisecond,
				Budget:   1 * time.Second,
				Reason:   "exit 1",
				Output:   "./foo.go:1: trouble\n./bar.go:2: more trouble\n",
			},
			{
				ID:     "tsc",
				Status: runner.StatusSkip,
				Budget: 3 * time.Second,
				Reason: "no matching files",
			},
			{
				ID:       "slow",
				Status:   runner.StatusTimeout,
				Duration: 1 * time.Second,
				Budget:   500 * time.Millisecond,
			},
		},
	}
}

func TestHuman_SummaryCountsAllStatuses(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, sampleRun())
	out := buf.String()

	want := "1 passed, 1 failed, 1 skipped, 1 timed out"
	if !strings.Contains(out, want) {
		t.Errorf("missing summary %q in:\n%s", want, out)
	}
	if !strings.Contains(out, "1500ms / 5000ms budget") {
		t.Errorf("missing duration/budget line in:\n%s", out)
	}
}

func TestHuman_ErrorStatusCountsAsFail(t *testing.T) {
	run := &runner.Run{
		Status: runner.StatusFail,
		Results: []runner.Result{
			{ID: "broken", Status: runner.StatusError, Reason: "exec failed"},
		},
	}
	var buf bytes.Buffer
	Human(&buf, run)
	if !strings.Contains(buf.String(), "0 passed, 1 failed") {
		t.Errorf("error status should roll into failed count; got:\n%s", buf.String())
	}
}

func TestHuman_ShowsReasonAndOutputOnlyForFailures(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, sampleRun())
	out := buf.String()

	if !strings.Contains(out, "exit 1") {
		t.Errorf("failing check reason missing: %s", out)
	}
	if !strings.Contains(out, "  | ./foo.go:1: trouble") {
		t.Errorf("failing check output should be indented with '  | ' prefix: %s", out)
	}
	if !strings.Contains(out, "no matching files") {
		t.Errorf("skip reason missing: %s", out)
	}

	passRun := &runner.Run{
		Status: runner.StatusPass,
		Results: []runner.Result{
			{
				ID:       "ok",
				Status:   runner.StatusPass,
				Duration: 50 * time.Millisecond,
				Output:   "verbose pass output",
			},
		},
	}
	var passBuf bytes.Buffer
	Human(&passBuf, passRun)
	if strings.Contains(passBuf.String(), "verbose pass output") {
		t.Errorf("passing check output should not appear in human output: %s", passBuf.String())
	}
}

func TestHuman_ZeroDurationRendersDash(t *testing.T) {
	run := &runner.Run{
		Status: runner.StatusPass,
		Results: []runner.Result{
			{ID: "instant", Status: runner.StatusSkip, Duration: 0},
		},
	}
	var buf bytes.Buffer
	Human(&buf, run)
	if !strings.Contains(buf.String(), "—") {
		t.Errorf("zero-duration row should render an em dash: %s", buf.String())
	}
}

func TestHuman_HandlesNoResults(t *testing.T) {
	var buf bytes.Buffer
	Human(&buf, &runner.Run{Status: runner.StatusPass, Budget: 5 * time.Second})
	if !strings.Contains(buf.String(), "0 passed, 0 failed, 0 skipped, 0 timed out") {
		t.Errorf("empty run should still produce a summary: %s", buf.String())
	}
}

func TestJSON_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleRun()); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got jsonRun
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if got.Status != string(runner.StatusFail) {
		t.Errorf("Status = %q", got.Status)
	}
	if got.DurationMs != 1500 || got.BudgetMs != 5000 {
		t.Errorf("Duration/Budget = %d/%d", got.DurationMs, got.BudgetMs)
	}
	if len(got.Checks) != 4 {
		t.Fatalf("Checks len = %d, want 4", len(got.Checks))
	}

	byID := map[string]jsonResult{}
	for _, c := range got.Checks {
		byID[c.ID] = c
	}
	if byID["vet"].Output == "" || byID["vet"].Reason == "" {
		t.Errorf("vet should carry output+reason, got %+v", byID["vet"])
	}
	if byID["lint"].Output != "" || byID["lint"].Reason != "" {
		t.Errorf("lint had no output/reason but JSON included them: %+v", byID["lint"])
	}
}

func TestJSON_IsIndented(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, &runner.Run{Status: runner.StatusPass}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "\n  \"status\"") {
		t.Errorf("expected two-space indentation, got:\n%s", s)
	}
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("encoder should produce a trailing newline, got %q", s)
	}
}

func TestJSON_OmitsEmptyOutputAndReason(t *testing.T) {
	run := &runner.Run{
		Status: runner.StatusPass,
		Results: []runner.Result{
			{ID: "clean", Status: runner.StatusPass, Duration: 10 * time.Millisecond, Budget: 1 * time.Second},
		},
	}
	var buf bytes.Buffer
	if err := JSON(&buf, run); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Contains(s, "\"output\"") {
		t.Errorf("empty output should be omitted: %s", s)
	}
	if strings.Contains(s, "\"reason\"") {
		t.Errorf("empty reason should be omitted: %s", s)
	}
}
