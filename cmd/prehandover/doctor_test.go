package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDoctorConfig(t *testing.T, dir string, adapters string) {
	t.Helper()
	body := `[manifest]
project = "test"
moments = ["agent_stop"]
adapters = [` + adapters + `]
required_prehandover = "0.1.0"

[[checks]]
id = "ok"
entry = "true"
always_run = true
`
	if err := os.WriteFile(filepath.Join(dir, "prehandover.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func withChdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCmdDoctor_AllManifestAdaptersInstalled(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	writeDoctorConfig(t, dir, `"claude", "codex", "cursor"`)

	if rc := installClaudeAt(filepath.Join(".claude", "settings.json"), false); rc != 0 {
		t.Fatalf("install claude rc = %d", rc)
	}
	if rc := installCodexAt(filepath.Join(".codex", "hooks.json"), filepath.Join(".codex", "config.toml"), false); rc != 0 {
		t.Fatalf("install codex rc = %d", rc)
	}
	if rc := installCursorAt(filepath.Join(".cursor", "hooks.json"), false); rc != 0 {
		t.Fatalf("install cursor rc = %d", rc)
	}

	if rc := cmdDoctor(nil); rc != 0 {
		t.Fatalf("doctor rc = %d, want 0", rc)
	}
}

func TestCmdDoctor_FailsWhenManifestAdapterIsMissing(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	writeDoctorConfig(t, dir, `"cursor"`)

	if rc := cmdDoctor(nil); rc == 0 {
		t.Fatal("doctor should fail when manifest adapter is not installed")
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("0.2.0", "0.1.9") <= 0 {
		t.Fatal("0.2.0 should be newer than 0.1.9")
	}
	if compareVersions("0.1.0", "0.1.0") != 0 {
		t.Fatal("same versions should compare equal")
	}
	if compareVersions("0.1.0", "0.2.0") >= 0 {
		t.Fatal("0.1.0 should be older than 0.2.0")
	}
}
