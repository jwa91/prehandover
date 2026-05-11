package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- pure merge logic ---

func TestMergeClaudeStopHook_EmptyMap(t *testing.T) {
	settings := map[string]any{}
	changed := mergeClaudeStopHook(settings)
	if !changed {
		t.Fatal("expected change=true on empty map")
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks key not a map: %T", settings["hooks"])
	}
	stop, ok := hooks["Stop"].([]any)
	if !ok || len(stop) != 1 {
		t.Fatalf("expected Stop with 1 entry, got %v", stop)
	}
}

func TestMergeClaudeStopHook_PreservesOtherKeys(t *testing.T) {
	settings := map[string]any{
		"model": "claude-opus-4-7",
		"theme": "dark",
	}
	mergeClaudeStopHook(settings)
	if settings["model"] != "claude-opus-4-7" {
		t.Errorf("model key lost: %v", settings["model"])
	}
	if settings["theme"] != "dark" {
		t.Errorf("theme key lost: %v", settings["theme"])
	}
}

func TestMergeClaudeStopHook_Idempotent(t *testing.T) {
	settings := map[string]any{}
	if !mergeClaudeStopHook(settings) {
		t.Fatal("first call should mutate")
	}
	if mergeClaudeStopHook(settings) {
		t.Error("second call should be no-op")
	}
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 1 {
		t.Errorf("expected 1 Stop entry after double-merge, got %d", len(stop))
	}
}

func TestMergeClaudeStopHook_AppendsAlongsideOtherStopHooks(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo other-tool"},
					},
				},
			},
		},
	}
	if !mergeClaudeStopHook(settings) {
		t.Fatal("expected change=true")
	}
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 2 {
		t.Errorf("expected 2 Stop entries (other + ours), got %d", len(stop))
	}
}

func TestMergeClaudeStopHook_EmittedCommandShape(t *testing.T) {
	settings := map[string]any{}
	mergeClaudeStopHook(settings)
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	entry := stop[0].(map[string]any)
	if entry["matcher"] != "" {
		t.Errorf(`matcher = %q, want ""`, entry["matcher"])
	}
	hooks := entry["hooks"].([]any)
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	hook := hooks[0].(map[string]any)
	if hook["type"] != "command" {
		t.Errorf("type = %v, want command", hook["type"])
	}
	if hook["command"] != claudeAgentStopCmd {
		t.Errorf("command = %v, want %s", hook["command"], claudeAgentStopCmd)
	}
}

func TestHasNestedCommand(t *testing.T) {
	cases := []struct {
		name string
		stop []any
		want bool
	}{
		{"empty", []any{}, false},
		{"unrelated_hook", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "echo hi"},
			}},
		}, false},
		{"matching_command", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": claudeAgentStopCmd},
			}},
		}, true},
		{"old_command_does_not_match", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "/opt/homebrew/bin/prehandover run"},
			}},
		}, false},
		{"command_just_mentions_prehandover", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "echo prehandover"},
			}},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasNestedCommand(tc.stop, claudeAgentStopCmd)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// --- file I/O integration ---

func TestInstallClaudeAt_CreatesFileAndDirectory(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".claude", "settings.json")
	rc := installClaudeAt(target, false)
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), claudeAgentStopCmd) {
		t.Errorf("file does not contain hook command:\n%s", data)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Errorf("written file is not valid JSON: %v", err)
	}
}

func TestInstallClaudeAt_MergeIntoExisting(t *testing.T) {
	target := filepath.Join(t.TempDir(), "settings.json")
	initial := `{"model": "claude-opus-4-7", "theme": "dark"}`
	if err := os.WriteFile(target, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	rc := installClaudeAt(target, false)
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	data, _ := os.ReadFile(target)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["model"] != "claude-opus-4-7" {
		t.Errorf("model key lost: %v", settings["model"])
	}
	if settings["theme"] != "dark" {
		t.Errorf("theme key lost: %v", settings["theme"])
	}
	if _, ok := settings["hooks"]; !ok {
		t.Error("hooks key not added")
	}
}

func TestInstallClaudeAt_PrintDoesNotWrite(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".claude", "settings.json")
	rc := installClaudeAt(target, true)
	if rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("--print should not write the file; got err = %v", err)
	}
}

func TestInstallClaudeAt_TwiceIsIdempotent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "settings.json")
	if rc := installClaudeAt(target, false); rc != 0 {
		t.Fatalf("first install rc = %d", rc)
	}
	first, _ := os.ReadFile(target)
	if rc := installClaudeAt(target, false); rc != 0 {
		t.Fatalf("second install rc = %d", rc)
	}
	second, _ := os.ReadFile(target)
	if string(first) != string(second) {
		t.Errorf("second install changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestMergeCodexStopHook_EmittedCommandShape(t *testing.T) {
	settings := map[string]any{}
	if !mergeCodexStopHook(settings) {
		t.Fatal("expected change=true")
	}
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	hooks := stop[0].(map[string]any)["hooks"].([]any)
	hook := hooks[0].(map[string]any)
	if hook["type"] != "command" {
		t.Errorf("type = %v, want command", hook["type"])
	}
	if hook["command"] != codexAgentStopCmd {
		t.Errorf("command = %v, want %s", hook["command"], codexAgentStopCmd)
	}
}

func TestMergeCodexStopHook_Idempotent(t *testing.T) {
	settings := map[string]any{}
	if !mergeCodexStopHook(settings) {
		t.Fatal("first call should mutate")
	}
	if mergeCodexStopHook(settings) {
		t.Fatal("second call should be no-op")
	}
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("expected 1 Stop entry, got %d", len(stop))
	}
}

func TestMergeCursorStopHook_EmittedCommandShape(t *testing.T) {
	settings := map[string]any{}
	if !mergeCursorStopHook(settings) {
		t.Fatal("expected change=true")
	}
	if settings["version"] != float64(1) {
		t.Fatalf("version = %v, want 1", settings["version"])
	}
	stop := settings["hooks"].(map[string]any)["stop"].([]any)
	hook := stop[0].(map[string]any)
	if hook["command"] != cursorAgentStopCmd {
		t.Errorf("command = %v, want %s", hook["command"], cursorAgentStopCmd)
	}
	if _, ok := hook["loop_limit"]; !ok {
		t.Error("loop_limit key missing")
	}
	if hook["loop_limit"] != nil {
		t.Errorf("loop_limit = %v, want nil", hook["loop_limit"])
	}
}

func TestMergeCursorStopHook_Idempotent(t *testing.T) {
	settings := map[string]any{}
	if !mergeCursorStopHook(settings) {
		t.Fatal("first call should mutate")
	}
	if mergeCursorStopHook(settings) {
		t.Fatal("second call should be no-op")
	}
	stop := settings["hooks"].(map[string]any)["stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("expected 1 stop entry, got %d", len(stop))
	}
}

func TestEnsureCodexHooksFeature_AddsSection(t *testing.T) {
	got, changed := ensureCodexHooksFeature("model = \"gpt\"\n")
	if !changed {
		t.Fatal("expected change=true")
	}
	if !strings.Contains(got, "[features]\ncodex_hooks = true\n") {
		t.Fatalf("missing features block:\n%s", got)
	}
	if !strings.Contains(got, "model = \"gpt\"") {
		t.Fatalf("lost existing setting:\n%s", got)
	}
}

func TestEnsureCodexHooksFeature_UpdatesExistingFalse(t *testing.T) {
	got, changed := ensureCodexHooksFeature("[features]\ncodex_hooks = false\n")
	if !changed {
		t.Fatal("expected change=true")
	}
	if got != "[features]\ncodex_hooks = true\n" {
		t.Fatalf("got:\n%s", got)
	}
}

func TestEnsureCodexHooksFeature_Idempotent(t *testing.T) {
	got, changed := ensureCodexHooksFeature("[features]\ncodex_hooks = true\n")
	if changed {
		t.Fatal("expected no change")
	}
	if got != "[features]\ncodex_hooks = true\n" {
		t.Fatalf("got:\n%s", got)
	}
}

func TestInstallCodexAt_TwiceIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, ".codex", "hooks.json")
	configPath := filepath.Join(dir, ".codex", "config.toml")
	if rc := installCodexAt(hooksPath, configPath, false); rc != 0 {
		t.Fatalf("first install rc = %d", rc)
	}
	firstHooks, _ := os.ReadFile(hooksPath)
	firstConfig, _ := os.ReadFile(configPath)
	if rc := installCodexAt(hooksPath, configPath, false); rc != 0 {
		t.Fatalf("second install rc = %d", rc)
	}
	secondHooks, _ := os.ReadFile(hooksPath)
	secondConfig, _ := os.ReadFile(configPath)
	if string(firstHooks) != string(secondHooks) {
		t.Errorf("hooks changed on second install")
	}
	if string(firstConfig) != string(secondConfig) {
		t.Errorf("config changed on second install")
	}
}

func TestInstallCursorAt_TwiceIsIdempotent(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".cursor", "hooks.json")
	if rc := installCursorAt(target, false); rc != 0 {
		t.Fatalf("first install rc = %d", rc)
	}
	first, _ := os.ReadFile(target)
	if rc := installCursorAt(target, false); rc != 0 {
		t.Fatalf("second install rc = %d", rc)
	}
	second, _ := os.ReadFile(target)
	if string(first) != string(second) {
		t.Errorf("second install changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}
