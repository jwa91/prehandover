package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jwa91/prehandover/internal/proof"
)

func TestCmdHookWritesProofArtifact(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	writeDoctorConfig(t, dir, `"claude"`)

	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	os.Stdin = stdin

	if rc := cmdHook([]string{"claude", "agent_stop"}); rc != 0 {
		t.Fatalf("hook rc = %d, want 0", rc)
	}

	data, err := os.ReadFile(filepath.Join(dir, proof.LatestPath))
	if err != nil {
		t.Fatal(err)
	}
	var artifact proof.Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Moment != "agent_stop" {
		t.Fatalf("moment = %q, want agent_stop", artifact.Moment)
	}
	if artifact.Harness != "claude" {
		t.Fatalf("harness = %q, want claude", artifact.Harness)
	}
	if artifact.Status != "pass" || artifact.Category != "passed" {
		t.Fatalf("status/category = %s/%s, want pass/passed", artifact.Status, artifact.Category)
	}
}

// A harness sending a relative "cwd" is contract-violating: silently os.Chdir-ing
// to it would resolve against whatever shell-inherited dir prehandover happened
// to start in, which can land outside the project. Reject before chdir.
func TestCmdHook_RejectsRelativeCwd(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	writeDoctorConfig(t, dir, `"claude"`)

	stdinPath := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(stdinPath, []byte(`{"cwd": "some/relative/path"}`), 0644); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	os.Stdin = stdin

	if rc := cmdHook([]string{"claude", "agent_stop"}); rc != 0 {
		t.Fatalf("hook rc = %d, want 0 (failure outcome encoded via adapter)", rc)
	}

	data, err := os.ReadFile(filepath.Join(dir, proof.LatestPath))
	if err != nil {
		t.Fatal(err)
	}
	var artifact proof.Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "error" {
		t.Errorf("status = %q, want error", artifact.Status)
	}
	if artifact.Category != "execution_error" {
		t.Errorf("category = %q, want execution_error", artifact.Category)
	}
	if !strings.Contains(artifact.ContinuationMessage, "absolute") {
		t.Errorf("message should explain absolute requirement: %q", artifact.ContinuationMessage)
	}
}
