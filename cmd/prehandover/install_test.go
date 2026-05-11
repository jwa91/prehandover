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
	if hook["command"] != prehandoverCmd {
		t.Errorf("command = %v, want %s", hook["command"], prehandoverCmd)
	}
}

func TestHasPrehandoverHook(t *testing.T) {
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
		{"prehandover_on_path", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "prehandover run --format=claude"},
			}},
		}, true},
		{"prehandover_full_path", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "/opt/homebrew/bin/prehandover run"},
			}},
		}, true},
		{"command_just_mentions_prehandover", []any{
			map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "echo prehandover"},
			}},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasPrehandoverHook(tc.stop)
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
	if !strings.Contains(string(data), prehandoverCmd) {
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
