package proof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jwa91/prehandover/internal/lifecycle"
	"github.com/jwa91/prehandover/internal/runner"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "prehandover.toml")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func sampleInvocation() lifecycle.Invocation {
	return lifecycle.Invocation{
		Harness:        "claude",
		Moment:         lifecycle.MomentAgentStop,
		CWD:            "/repo",
		SessionID:      "sess-1",
		TurnID:         "turn-2",
		TranscriptPath: "/tmp/transcript.json",
	}
}

func TestCategoryForRun(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  *runner.Run
		want string
	}{
		{
			name: "passed",
			run:  &runner.Run{Status: runner.StatusPass},
			want: "passed",
		},
		{
			name: "timeout maps to budget_exceeded",
			run:  &runner.Run{Status: runner.StatusTimeout},
			want: "budget_exceeded",
		},
		{
			name: "fail with no errors is validator_failed",
			run: &runner.Run{
				Status: runner.StatusFail,
				Results: []runner.Result{
					{ID: "a", Status: runner.StatusFail},
					{ID: "b", Status: runner.StatusPass},
				},
			},
			want: "validator_failed",
		},
		{
			name: "fail with an error result is validator_error",
			run: &runner.Run{
				Status: runner.StatusFail,
				Results: []runner.Result{
					{ID: "a", Status: runner.StatusFail},
					{ID: "b", Status: runner.StatusError},
				},
			},
			want: "validator_error",
		},
		{
			name: "unknown status falls back to execution_error",
			run:  &runner.Run{Status: runner.Status("weird")},
			want: "execution_error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CategoryForRun(tc.run)
			if got != tc.want {
				t.Errorf("CategoryForRun = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFromRun_PopulatesFields(t *testing.T) {
	cfgPath := writeConfig(t, "manifest = \"present\"\n")
	inv := sampleInvocation()
	run := &runner.Run{
		Status:   runner.StatusPass,
		Duration: 1200 * time.Millisecond,
		Budget:   5 * time.Second,
		Results: []runner.Result{
			{
				ID:       "lint",
				Status:   runner.StatusPass,
				Duration: 200 * time.Millisecond,
				Budget:   1 * time.Second,
			},
		},
	}
	outcome := lifecycle.Outcome{Allow: true, Run: run}

	art := FromRun(inv, cfgPath, []string{"a.go", "b.go"}, outcome)

	if art.Version != 1 {
		t.Errorf("Version = %d, want 1", art.Version)
	}
	if art.Moment != string(lifecycle.MomentAgentStop) {
		t.Errorf("Moment = %q", art.Moment)
	}
	if art.Harness != "claude" {
		t.Errorf("Harness = %q", art.Harness)
	}
	if art.CWD != "/repo" || art.SessionID != "sess-1" || art.TurnID != "turn-2" {
		t.Errorf("invocation fields not threaded through: %+v", art)
	}
	if art.Status != string(runner.StatusPass) || art.Category != "passed" {
		t.Errorf("Status/Category = %q/%q, want pass/passed", art.Status, art.Category)
	}
	if art.DurationMs != 1200 || art.BudgetMs != 5000 {
		t.Errorf("Duration/Budget = %d/%d", art.DurationMs, art.BudgetMs)
	}
	if len(art.Checks) != 1 || art.Checks[0].ID != "lint" {
		t.Errorf("Checks = %+v", art.Checks)
	}
	if _, err := time.Parse(time.RFC3339Nano, art.GeneratedAt); err != nil {
		t.Errorf("GeneratedAt %q is not RFC3339Nano: %v", art.GeneratedAt, err)
	}
	if want := expectedSHA(t, cfgPath); art.ConfigSHA256 != want {
		t.Errorf("ConfigSHA256 = %q, want %q", art.ConfigSHA256, want)
	}
}

func TestFromRun_FailingRunIncludesContinuationMessage(t *testing.T) {
	cfgPath := writeConfig(t, "x = 1")
	run := &runner.Run{
		Status: runner.StatusFail,
		Results: []runner.Result{
			{ID: "lint", Status: runner.StatusFail, Reason: "bad"},
		},
	}
	outcome := lifecycle.Outcome{Allow: false, ContinueMessage: "lint failed", Run: run}

	art := FromRun(sampleInvocation(), cfgPath, nil, outcome)
	if art.ContinuationMessage != "lint failed" {
		t.Errorf("ContinuationMessage = %q", art.ContinuationMessage)
	}
	if art.Category != "validator_failed" {
		t.Errorf("Category = %q, want validator_failed", art.Category)
	}
}

func TestFromRun_NilRunDefaultsToPassed(t *testing.T) {
	cfgPath := writeConfig(t, "x = 1")
	art := FromRun(sampleInvocation(), cfgPath, nil, lifecycle.Outcome{Allow: true})
	if art.Status != string(runner.StatusPass) {
		t.Errorf("Status = %q, want pass (nil run fallback)", art.Status)
	}
	if art.Category != "passed" {
		t.Errorf("Category = %q, want passed", art.Category)
	}
	if len(art.Checks) != 0 {
		t.Errorf("Checks = %+v, want none", art.Checks)
	}
}

func TestFromRun_ConfigSHA256MissingFileIsEmpty(t *testing.T) {
	art := FromRun(sampleInvocation(), "/does/not/exist.toml", nil, lifecycle.Outcome{Allow: true})
	if art.ConfigSHA256 != "" {
		t.Errorf("ConfigSHA256 = %q, want empty when config missing", art.ConfigSHA256)
	}
}

func TestFailure_ProducesErrorArtifact(t *testing.T) {
	cfgPath := writeConfig(t, "x = 1")
	art := Failure(sampleInvocation(), cfgPath, "config_error", errors.New("boom"))
	if art.Status != string(runner.StatusError) {
		t.Errorf("Status = %q, want error", art.Status)
	}
	if art.Category != "config_error" {
		t.Errorf("Category = %q", art.Category)
	}
	if art.Error != "boom" || art.ContinuationMessage != "boom" {
		t.Errorf("Error/ContinuationMessage = %q/%q", art.Error, art.ContinuationMessage)
	}
	if art.Harness != "claude" {
		t.Errorf("Harness = %q (invocation not threaded)", art.Harness)
	}
}

func TestFailure_NilErrorYieldsEmptyMessage(t *testing.T) {
	art := Failure(sampleInvocation(), "/missing.toml", "execution_error", nil)
	if art.Error != "" || art.ContinuationMessage != "" {
		t.Errorf("nil err should produce empty messages, got %q/%q", art.Error, art.ContinuationMessage)
	}
}

func TestWriteLatest_WritesJSONToExpectedPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	art := Artifact{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Moment:      "agent_stop",
		Harness:     "claude",
		Status:      "pass",
		Category:    "passed",
	}
	if err := WriteLatest(art); err != nil {
		t.Fatalf("WriteLatest: %v", err)
	}

	full := filepath.Join(dir, LatestPath)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read latest.json: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Errorf("artifact file should end with newline, got %q", data)
	}
	var roundTrip Artifact
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("artifact is not valid JSON: %v", err)
	}
	if roundTrip.Moment != art.Moment || roundTrip.Harness != art.Harness {
		t.Errorf("round-trip mismatch: %+v vs %+v", roundTrip, art)
	}
}

func TestWriteLatest_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := WriteLatest(Artifact{Version: 1, Status: "pass"}); err != nil {
		t.Fatalf("WriteLatest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".prehandover", "runs")); err != nil {
		t.Errorf("parent directory not created: %v", err)
	}
}

func expectedSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
