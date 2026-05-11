package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	claudeAgentStopCmd = "prehandover hook claude agent_stop"
	codexAgentStopCmd  = "prehandover hook codex agent_stop"
	cursorAgentStopCmd = "prehandover hook cursor agent_stop"
)

func cmdInstall(args []string) int {
	flags := flag.NewFlagSet("install", flag.ExitOnError)
	printOnly := flags.Bool("print", false, "print the merged settings without writing")
	flags.Parse(args)
	rest := flags.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: prehandover install [--print] <harness>\n\nsupported: claude, codex, cursor")
		return 2
	}
	switch rest[0] {
	case "claude":
		return installClaudeAt(filepath.Join(".claude", "settings.json"), *printOnly)
	case "codex":
		return installCodexAt(filepath.Join(".codex", "hooks.json"), filepath.Join(".codex", "config.toml"), *printOnly)
	case "cursor":
		return installCursorAt(filepath.Join(".cursor", "hooks.json"), *printOnly)
	default:
		fmt.Fprintf(os.Stderr, "unknown harness: %q (supported: claude, codex, cursor)\n", rest[0])
		return 2
	}
}

func installClaudeAt(path string, printOnly bool) int {
	settings, err := readJSONMap(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	changed := mergeClaudeStopHook(settings)

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out = append(out, '\n')

	if printOnly {
		fmt.Println(string(out))
		return 0
	}
	if !changed {
		fmt.Printf("already installed in %s\n", path)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("installed Stop hook in %s\n", path)
	return 0
}

// mergeClaudeStopHook adds prehandover's agent_stop hook to the settings map.
// Returns true if a change was made, false if it was already present.
func mergeClaudeStopHook(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	stop, _ := hooks["Stop"].([]any)
	if hasNestedCommand(stop, claudeAgentStopCmd) {
		return false
	}
	entry := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": claudeAgentStopCmd,
			},
		},
	}
	hooks["Stop"] = append(stop, entry)
	return true
}

func installCodexAt(hooksPath, configPath string, printOnly bool) int {
	hooksSettings, err := readJSONMap(hooksPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hooksChanged := mergeCodexStopHook(hooksSettings)
	hooksOut, err := json.MarshalIndent(hooksSettings, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hooksOut = append(hooksOut, '\n')

	configData, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	configOut, configChanged := ensureCodexHooksFeature(string(configData))

	if printOnly {
		fmt.Printf("# %s\n%s", hooksPath, hooksOut)
		fmt.Printf("# %s\n%s", configPath, configOut)
		return 0
	}
	if hooksChanged {
		if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(hooksPath, hooksOut, 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if configChanged {
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(configPath, []byte(configOut), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if !hooksChanged && !configChanged {
		fmt.Printf("already installed in %s and %s\n", hooksPath, configPath)
		return 0
	}
	fmt.Printf("installed Stop hook in %s and enabled codex_hooks in %s\n", hooksPath, configPath)
	return 0
}

func mergeCodexStopHook(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	stop, _ := hooks["Stop"].([]any)
	if hasNestedCommand(stop, codexAgentStopCmd) {
		return false
	}
	entry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": codexAgentStopCmd,
			},
		},
	}
	hooks["Stop"] = append(stop, entry)
	return true
}

func installCursorAt(path string, printOnly bool) int {
	settings, err := readJSONMap(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	changed := mergeCursorStopHook(settings)
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out = append(out, '\n')
	if printOnly {
		fmt.Println(string(out))
		return 0
	}
	if !changed {
		fmt.Printf("already installed in %s\n", path)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("installed stop hook in %s\n", path)
	return 0
}

func mergeCursorStopHook(settings map[string]any) bool {
	changed := false
	if _, ok := settings["version"]; !ok {
		settings["version"] = float64(1)
		changed = true
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
		changed = true
	}
	stop, _ := hooks["stop"].([]any)
	if hasFlatCommand(stop, cursorAgentStopCmd) {
		return changed
	}
	hooks["stop"] = append(stop, map[string]any{
		"command":    cursorAgentStopCmd,
		"loop_limit": nil,
	})
	return true
}

func ensureCodexHooksFeature(data string) (string, bool) {
	normalized := strings.ReplaceAll(data, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	sectionRE := regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(#.*)?$`)
	codexHooksRE := regexp.MustCompile(`^\s*codex_hooks\s*=`)
	trueRE := regexp.MustCompile(`^\s*codex_hooks\s*=\s*true\s*(#.*)?$`)

	inFeatures := false
	featuresStart := -1
	insertAt := -1
	for i, line := range lines {
		if match := sectionRE.FindStringSubmatch(line); match != nil {
			if inFeatures && insertAt == -1 {
				insertAt = i
			}
			inFeatures = strings.TrimSpace(match[1]) == "features"
			if inFeatures {
				featuresStart = i
				insertAt = len(lines)
			}
			continue
		}
		if inFeatures && codexHooksRE.MatchString(line) {
			if trueRE.MatchString(line) {
				return normalizedWithFinalNewline(lines), false
			}
			lines[i] = "codex_hooks = true"
			return normalizedWithFinalNewline(lines), true
		}
	}

	if featuresStart >= 0 {
		if insertAt < 0 {
			insertAt = len(lines)
		}
		lines = append(lines[:insertAt], append([]string{"codex_hooks = true"}, lines[insertAt:]...)...)
		return normalizedWithFinalNewline(lines), true
	}

	if len(lines) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, "[features]", "codex_hooks = true")
	return normalizedWithFinalNewline(lines), true
}

func normalizedWithFinalNewline(lines []string) string {
	return strings.Join(lines, "\n") + "\n"
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func hasNestedCommand(stop []any, want string) bool {
	for _, e := range stop {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		hs, _ := em["hooks"].([]any)
		for _, h := range hs {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hm["command"].(string)
			if cmd == want {
				return true
			}
		}
	}
	return false
}

func hasFlatCommand(stop []any, want string) bool {
	for _, e := range stop {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := em["command"].(string)
		if cmd == want {
			return true
		}
	}
	return false
}
