package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
