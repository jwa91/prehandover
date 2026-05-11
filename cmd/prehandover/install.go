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
		fmt.Fprintln(os.Stderr, "usage: prehandover install <harness>\n\nsupported: claude")
		return 2
	}
	switch rest[0] {
	case "claude":
		return installClaude(*printOnly)
	default:
		fmt.Fprintf(os.Stderr, "unknown harness: %q (supported: claude)\n", rest[0])
		return 2
	}
}

func installClaude(printOnly bool) int {
	path := filepath.Join(".claude", "settings.json")

	settings, err := readJSONMap(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	stop, _ := hooks["Stop"].([]any)
	if hasPrehandoverHook(stop) {
		fmt.Printf("already installed in %s\n", path)
		return 0
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
