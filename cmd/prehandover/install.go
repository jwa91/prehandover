package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const prehandoverCmd = "prehandover run --format=claude"

func cmdInstall(args []string) int {
	flags := flag.NewFlagSet("install", flag.ExitOnError)
	printOnly := flags.Bool("print", false, "print the merged settings without writing")
	flags.Parse(args)
	rest := flags.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: prehandover install [--print] <harness>\n\nsupported: claude")
		return 2
	}
	switch rest[0] {
	case "claude":
		return installClaudeAt(filepath.Join(".claude", "settings.json"), *printOnly)
	default:
		fmt.Fprintf(os.Stderr, "unknown harness: %q (supported: claude)\n", rest[0])
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

// mergeClaudeStopHook adds prehandover's Stop hook to the settings map.
// Returns true if a change was made, false if it was already present.
func mergeClaudeStopHook(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	stop, _ := hooks["Stop"].([]any)
	if hasPrehandoverHook(stop) {
		return false
	}
	entry := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": prehandoverCmd,
			},
		},
	}
	hooks["Stop"] = append(stop, entry)
	return true
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

func hasPrehandoverHook(stop []any) bool {
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
			if strings.HasPrefix(cmd, "prehandover ") || strings.Contains(cmd, "/prehandover ") {
				return true
			}
		}
	}
	return false
}
