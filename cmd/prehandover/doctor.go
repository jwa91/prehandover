package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jwa91/prehandover/internal/config"
	"github.com/jwa91/prehandover/internal/lifecycle"
	"github.com/jwa91/prehandover/internal/version"
)

type doctorResult struct {
	OK      bool
	Message string
}

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	cfgPath := fs.String("config", "prehandover.toml", "config file path")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stdout, "bad config: %v\n", err)
		return 1
	}

	var results []doctorResult
	results = append(results, checkVersion(cfg.Manifest.RequiredPrehandover))
	results = append(results, checkMomentManifest(cfg.Manifest.Moments)...)
	results = append(results, checkAdapterManifest(cfg.Manifest.Adapters)...)
	for _, adapter := range cfg.Manifest.Adapters {
		results = append(results, checkInstalledAdapter(adapter))
	}

	ok := true
	for _, res := range results {
		prefix := "ok "
		if !res.OK {
			prefix = "bad"
			ok = false
		}
		fmt.Fprintf(os.Stdout, "%s %s\n", prefix, res.Message)
	}
	if !ok {
		return 1
	}
	return 0
}

func checkVersion(required string) doctorResult {
	if compareVersions(version.Current, required) < 0 {
		return doctorResult{Message: fmt.Sprintf("prehandover %s is older than manifest.required_prehandover %s", version.Current, required)}
	}
	return doctorResult{OK: true, Message: fmt.Sprintf("prehandover %s satisfies required %s", version.Current, required)}
}

func checkMomentManifest(moments []string) []doctorResult {
	seenAgentStop := false
	var out []doctorResult
	for _, m := range moments {
		moment := lifecycle.Moment(m)
		if moment == lifecycle.MomentAgentStop {
			seenAgentStop = true
			out = append(out, doctorResult{OK: true, Message: "manifest declares moment agent_stop"})
			continue
		}
		out = append(out, doctorResult{Message: fmt.Sprintf("manifest declares unsupported moment %s", m)})
	}
	if !seenAgentStop {
		out = append(out, doctorResult{Message: "manifest must declare moment agent_stop"})
	}
	return out
}

func checkAdapterManifest(adapters []string) []doctorResult {
	var out []doctorResult
	for _, name := range adapters {
		adapter, ok := lifecycle.ForHarness(name)
		if !ok {
			out = append(out, doctorResult{Message: fmt.Sprintf("manifest declares unsupported adapter %s", name)})
			continue
		}
		if !adapter.Supports(lifecycle.MomentAgentStop) {
			out = append(out, doctorResult{Message: fmt.Sprintf("adapter %s does not support agent_stop", name)})
			continue
		}
		out = append(out, doctorResult{OK: true, Message: fmt.Sprintf("adapter %s supports agent_stop", name)})
	}
	return out
}

func checkInstalledAdapter(name string) doctorResult {
	switch strings.ToLower(name) {
	case "claude":
		return checkClaudeInstalled()
	case "codex":
		return checkCodexInstalled()
	case "cursor":
		return checkCursorInstalled()
	default:
		return doctorResult{Message: fmt.Sprintf("cannot check unsupported adapter %s", name)}
	}
}

func checkClaudeInstalled() doctorResult {
	path := ".claude/settings.json"
	if _, err := os.Stat(path); err != nil {
		return doctorResult{Message: fmt.Sprintf("Claude hook config missing at %s", path)}
	}
	settings, err := readJSONMap(path)
	if err != nil {
		return doctorResult{Message: err.Error()}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	command, ok := findNestedPrehandoverCommand(stop, "claude")
	if !ok {
		return doctorResult{Message: "Claude Stop hook not installed (run: prehandover install claude)"}
	}
	if err := validateHookCommandExecutable(command); err != nil {
		return doctorResult{Message: fmt.Sprintf("Claude Stop hook command is not executable: %v", err)}
	}
	return doctorResult{OK: true, Message: "Claude Stop hook is installed"}
}

func checkCodexInstalled() doctorResult {
	hooksPath := ".codex/hooks.json"
	if _, err := os.Stat(hooksPath); err != nil {
		return doctorResult{Message: fmt.Sprintf("Codex hook config missing at %s", hooksPath)}
	}
	settings, err := readJSONMap(hooksPath)
	if err != nil {
		return doctorResult{Message: err.Error()}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	command, ok := findNestedPrehandoverCommand(stop, "codex")
	if !ok {
		return doctorResult{Message: "Codex Stop hook not installed (run: prehandover install codex)"}
	}
	if err := validateHookCommandExecutable(command); err != nil {
		return doctorResult{Message: fmt.Sprintf("Codex Stop hook command is not executable: %v", err)}
	}

	configPath := ".codex/config.toml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return doctorResult{Message: fmt.Sprintf("Codex config missing at %s", configPath)}
	}
	if !codexHooksFeatureEnabled(string(data)) {
		return doctorResult{Message: "Codex config does not enable [features].codex_hooks = true"}
	}
	return doctorResult{OK: true, Message: "Codex Stop hook is installed and codex_hooks is enabled"}
}

func checkCursorInstalled() doctorResult {
	path := ".cursor/hooks.json"
	if _, err := os.Stat(path); err != nil {
		return doctorResult{Message: fmt.Sprintf("Cursor hook config missing at %s", path)}
	}
	settings, err := readJSONMap(path)
	if err != nil {
		return doctorResult{Message: err.Error()}
	}
	hooks, _ := settings["hooks"].(map[string]any)
	stop, _ := hooks["stop"].([]any)
	command, ok := findFlatPrehandoverCommand(stop, "cursor")
	if !ok {
		return doctorResult{Message: "Cursor stop hook not installed (run: prehandover install cursor)"}
	}
	if err := validateHookCommandExecutable(command); err != nil {
		return doctorResult{Message: fmt.Sprintf("Cursor stop hook command is not executable: %v", err)}
	}
	if !cursorLoopLimitIsNil(stop, command) {
		return doctorResult{Message: "Cursor stop hook must set loop_limit = null for continuous enforcement"}
	}
	return doctorResult{OK: true, Message: "Cursor stop hook is installed with loop_limit = null"}
}

func codexHooksFeatureEnabled(data string) bool {
	sectionRE := regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(#.*)?$`)
	trueRE := regexp.MustCompile(`^\s*codex_hooks\s*=\s*true\s*(#.*)?$`)
	inFeatures := false
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		if match := sectionRE.FindStringSubmatch(line); match != nil {
			inFeatures = strings.TrimSpace(match[1]) == "features"
			continue
		}
		if inFeatures && trueRE.MatchString(line) {
			return true
		}
	}
	return false
}

func validateHookCommandExecutable(command string) error {
	fields := shellFields(command)
	if len(fields) == 0 {
		return fmt.Errorf("empty command")
	}
	exe := fields[0]
	if strings.ContainsRune(exe, os.PathSeparator) {
		info, err := os.Stat(exe)
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&0111 == 0 {
			return fmt.Errorf("%s is not executable", exe)
		}
		return nil
	}
	if _, err := exec.LookPath(exe); err != nil {
		return err
	}
	return nil
}

func cursorLoopLimitIsNil(stop []any, want string) bool {
	for _, e := range stop {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := em["command"].(string)
		if cmd == want {
			_, present := em["loop_limit"]
			return present && em["loop_limit"] == nil
		}
	}
	return false
}

func compareVersions(current, required string) int {
	c := versionParts(current)
	r := versionParts(required)
	for i := 0; i < len(c) || i < len(r); i++ {
		cv, rv := 0, 0
		if i < len(c) {
			cv = c[i]
		}
		if i < len(r) {
			rv = r[i]
		}
		if cv > rv {
			return 1
		}
		if cv < rv {
			return -1
		}
	}
	return 0
}

func versionParts(v string) []int {
	var parts []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			parts = append(parts, 0)
			continue
		}
		parts = append(parts, n)
	}
	return parts
}
