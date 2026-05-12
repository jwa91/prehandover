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
	prehandoverBinEnv  = "PREHANDOVER_BIN"
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
	harness := rest[0]
	switch harness {
	case "claude", "codex", "cursor":
	default:
		fmt.Fprintf(os.Stderr, "unknown harness: %q (supported: claude, codex, cursor)\n", harness)
		return 2
	}
	command, err := installHookCommand(harness)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch harness {
	case "claude":
		return installClaudeAt(filepath.Join(".claude", "settings.json"), *printOnly, command)
	case "codex":
		return installCodexAt(filepath.Join(".codex", "hooks.json"), filepath.Join(".codex", "config.toml"), *printOnly, command)
	case "cursor":
		return installCursorAt(filepath.Join(".cursor", "hooks.json"), *printOnly, command)
	}
	return 2
}

func installHookCommand(harness string) (string, error) {
	if override := os.Getenv(prehandoverBinEnv); override != "" {
		bin, err := checkedExecutable(override)
		if err != nil {
			return "", fmt.Errorf("%s: %w", prehandoverBinEnv, err)
		}
		return agentStopCommand(bin, harness), nil
	}
	return hookCommandOrDefault(harness, nil), nil
}

func checkedExecutable(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve executable %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("resolve executable %q: %w", path, err)
	}
	if info.IsDir() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("resolve executable %q: not an executable file", path)
	}
	return abs, nil
}

func agentStopCommand(binary, harness string) string {
	return shellQuoteArg(binary) + " hook " + harness + " agent_stop"
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$&;()[]{}*?!<>|`") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func hookCommandOrDefault(harness string, commands []string) string {
	if len(commands) > 0 && commands[0] != "" {
		return commands[0]
	}
	return "prehandover hook " + harness + " agent_stop"
}

func installClaudeAt(path string, printOnly bool, command ...string) int {
	settings, err := readJSONMap(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	changed := mergeClaudeStopHook(settings, command...)

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
func mergeClaudeStopHook(settings map[string]any, command ...string) bool {
	want := hookCommandOrDefault("claude", command)
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	stop, _ := hooks["Stop"].([]any)
	normalized, found, changed := normalizeNestedStopHook(stop, "claude", want)
	hooks["Stop"] = normalized
	if found {
		return changed
	}
	entry := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": want,
			},
		},
	}
	hooks["Stop"] = append(stop, entry)
	return true
}

func installCodexAt(hooksPath, configPath string, printOnly bool, command ...string) int {
	hooksSettings, err := readJSONMap(hooksPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hooksChanged := mergeCodexStopHook(hooksSettings, command...)
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

func mergeCodexStopHook(settings map[string]any, command ...string) bool {
	want := hookCommandOrDefault("codex", command)
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	stop, _ := hooks["Stop"].([]any)
	normalized, found, changed := normalizeNestedStopHook(stop, "codex", want)
	hooks["Stop"] = normalized
	if found {
		return changed
	}
	entry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": want,
			},
		},
	}
	hooks["Stop"] = append(stop, entry)
	return true
}

func installCursorAt(path string, printOnly bool, command ...string) int {
	settings, err := readJSONMap(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	changed := mergeCursorStopHook(settings, command...)
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

func mergeCursorStopHook(settings map[string]any, command ...string) bool {
	want := hookCommandOrDefault("cursor", command)
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
	found, normalized := normalizeFlatStopHook(stop, "cursor", want)
	if normalized {
		changed = true
	}
	if found {
		hooks["stop"] = stop
		return changed
	}
	hooks["stop"] = append(stop, map[string]any{
		"command":    want,
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
	_, ok := findNestedCommand(stop, func(command string) bool { return command == want })
	return ok
}

func findNestedPrehandoverCommand(stop []any, harness string) (string, bool) {
	return findNestedCommand(stop, func(command string) bool {
		return isPrehandoverAgentStopCommand(command, harness)
	})
}

func findNestedCommand(stop []any, match func(string) bool) (string, bool) {
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
			if match(cmd) {
				return cmd, true
			}
		}
	}
	return "", false
}

func findFlatPrehandoverCommand(stop []any, harness string) (string, bool) {
	return findFlatCommand(stop, func(command string) bool {
		return isPrehandoverAgentStopCommand(command, harness)
	})
}

func findFlatCommand(stop []any, match func(string) bool) (string, bool) {
	for _, e := range stop {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := em["command"].(string)
		if match(cmd) {
			return cmd, true
		}
	}
	return "", false
}

func normalizeNestedStopHook(stop []any, harness, want string) ([]any, bool, bool) {
	found := false
	changed := false
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
				found = true
				continue
			}
			if isPrehandoverAgentStopCommand(cmd, harness) {
				hm["command"] = want
				found = true
				changed = true
			}
		}
	}
	return stop, found, changed
}

func normalizeFlatStopHook(stop []any, harness, want string) (bool, bool) {
	found := false
	changed := false
	for _, e := range stop {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := em["command"].(string)
		if cmd == want {
			found = true
			continue
		}
		if isPrehandoverAgentStopCommand(cmd, harness) {
			em["command"] = want
			found = true
			changed = true
		}
	}
	return found, changed
}

func isPrehandoverAgentStopCommand(command, harness string) bool {
	fields := shellFields(command)
	if len(fields) < 4 {
		return false
	}
	return filepath.Base(fields[0]) == "prehandover" &&
		fields[1] == "hook" &&
		fields[2] == harness &&
		fields[3] == "agent_stop"
}

func shellFields(s string) []string {
	var fields []string
	var cur strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		fields = append(fields, cur.String())
		cur.Reset()
	}
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return fields
}
