package main

import (
	"os"
	"path/filepath"
	"strings"
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

func writeFakePrehandover(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "prehandover")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCmdDoctor_AllManifestAdaptersInstalled(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	writeDoctorConfig(t, dir, `"claude", "codex", "cursor"`)
	bin := writeFakePrehandover(t, dir)

	if rc := installClaudeAt(filepath.Join(".claude", "settings.json"), false, agentStopCommand(bin, "claude")); rc != 0 {
		t.Fatalf("install claude rc = %d", rc)
	}
	if rc := installCodexAt(filepath.Join(".codex", "hooks.json"), filepath.Join(".codex", "config.toml"), false, agentStopCommand(bin, "codex")); rc != 0 {
		t.Fatalf("install codex rc = %d", rc)
	}
	if rc := installCursorAt(filepath.Join(".cursor", "hooks.json"), false, agentStopCommand(bin, "cursor")); rc != 0 {
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

func TestCmdDoctor_FailsWhenHookCommandCannotExecute(t *testing.T) {
	dir := t.TempDir()
	withChdir(t, dir)
	writeDoctorConfig(t, dir, `"codex"`)

	missing := filepath.Join(dir, "missing", "prehandover")
	if rc := installCodexAt(filepath.Join(".codex", "hooks.json"), filepath.Join(".codex", "config.toml"), false, agentStopCommand(missing, "codex")); rc != 0 {
		t.Fatalf("install codex rc = %d", rc)
	}
	if rc := cmdDoctor(nil); rc == 0 {
		t.Fatal("doctor should fail when the hook executable is missing")
	}
}

// The installed-hook matcher checks the *shape* of the command (any binary
// whose basename is "prehandover" + the right subcommand). So the "not
// installed" message must not pretend a specific literal string is required —
// otherwise a perfectly valid absolute-path install reads as a discrepancy.
func TestCheckInstalled_NotInstalledMessageIsGeneric(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T)
		check    func() doctorResult
		harness  string
		litToken string
	}{
		{
			name: "claude",
			setup: func(t *testing.T) {
				mustWriteJSON(t, ".claude/settings.json", "{}")
			},
			check:    checkClaudeInstalled,
			harness:  "claude",
			litToken: `"prehandover hook claude agent_stop"`,
		},
		{
			name: "codex",
			setup: func(t *testing.T) {
				mustWriteJSON(t, ".codex/hooks.json", "{}")
			},
			check:    checkCodexInstalled,
			harness:  "codex",
			litToken: `"prehandover hook codex agent_stop"`,
		},
		{
			name: "cursor",
			setup: func(t *testing.T) {
				mustWriteJSON(t, ".cursor/hooks.json", "{}")
			},
			check:    checkCursorInstalled,
			harness:  "cursor",
			litToken: `"prehandover hook cursor agent_stop"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			withChdir(t, dir)
			tc.setup(t)

			r := tc.check()
			if r.OK {
				t.Fatalf("expected not-OK when hook entry is absent, got %+v", r)
			}
			if strings.Contains(r.Message, tc.litToken) {
				t.Errorf("message references literal command %s, but matching is shape-based: %q", tc.litToken, r.Message)
			}
			if !strings.Contains(r.Message, "prehandover install "+tc.harness) {
				t.Errorf("message should suggest 'prehandover install %s': %q", tc.harness, r.Message)
			}
		})
	}
}

func mustWriteJSON(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
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
