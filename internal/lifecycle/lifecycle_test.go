package lifecycle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jwa91/prehandover/internal/runner"
)

func failingRun() *runner.Run {
	return &runner.Run{
		Status: runner.StatusFail,
		Budget: time.Second,
		Results: []runner.Result{
			{ID: "lint", Status: runner.StatusFail, Output: "bad\n"},
		},
	}
}

func TestDecisionAdaptersRenderFailure(t *testing.T) {
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			adapter, ok := ForHarness(harness)
			if !ok {
				t.Fatalf("missing adapter")
			}
			var out bytes.Buffer
			if err := adapter.Encode(MomentAgentStop, OutcomeFromRun(failingRun()), &out); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["decision"] != "block" {
				t.Fatalf("decision = %v, want block", got["decision"])
			}
			if !strings.Contains(got["reason"].(string), "[lint] fail") {
				t.Fatalf("reason missing check result: %q", got["reason"])
			}
		})
	}
}

func TestCursorAdapterRendersFailure(t *testing.T) {
	adapter, ok := ForHarness("cursor")
	if !ok {
		t.Fatal("missing adapter")
	}
	var out bytes.Buffer
	if err := adapter.Encode(MomentAgentStop, OutcomeFromRun(failingRun()), &out); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got["followup_message"].(string), "[lint] fail") {
		t.Fatalf("followup_message missing check result: %q", got["followup_message"])
	}
}

func TestAdaptersRenderPassAsEmptyOutput(t *testing.T) {
	pass := &runner.Run{Status: runner.StatusPass}
	for _, harness := range []string{"claude", "codex", "cursor"} {
		t.Run(harness, func(t *testing.T) {
			adapter, _ := ForHarness(harness)
			var out bytes.Buffer
			if err := adapter.Encode(MomentAgentStop, OutcomeFromRun(pass), &out); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Fatalf("pass output = %q, want empty", out.String())
			}
		})
	}
}

func TestDecisionAdapterDecode(t *testing.T) {
	adapter, _ := ForHarness("codex")
	inv, err := adapter.Decode(MomentAgentStop, []byte(`{
		"cwd": "/repo",
		"session_id": "s1",
		"turn_id": "t1",
		"transcript_path": "/tmp/transcript.jsonl",
		"last_assistant_message": "done"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Harness != "codex" || inv.CWD != "/repo" || inv.SessionID != "s1" || inv.TurnID != "t1" {
		t.Fatalf("unexpected invocation: %+v", inv)
	}
	if inv.TranscriptPath != "/tmp/transcript.jsonl" || inv.LastAssistantMessage != "done" {
		t.Fatalf("missing fields: %+v", inv)
	}
}

func TestCursorAdapterDecodeWorkspaceRoot(t *testing.T) {
	adapter, _ := ForHarness("cursor")
	inv, err := adapter.Decode(MomentAgentStop, []byte(`{
		"conversation_id": "c1",
		"generation_id": "g1",
		"workspace_roots": ["/repo"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if inv.Harness != "cursor" || inv.CWD != "/repo" || inv.SessionID != "c1" || inv.TurnID != "g1" {
		t.Fatalf("unexpected invocation: %+v", inv)
	}
}

func TestDecodeEmptyInputFallsBackToCurrentWorkingDirectory(t *testing.T) {
	wd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"claude", "codex", "cursor"} {
		t.Run(harness, func(t *testing.T) {
			adapter, _ := ForHarness(harness)
			inv, err := adapter.Decode(MomentAgentStop, nil)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Clean(inv.CWD) != filepath.Clean(want) {
				t.Fatalf("cwd = %q, want %q", inv.CWD, want)
			}
		})
	}
}
