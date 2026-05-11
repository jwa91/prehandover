package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jwa91/prehandover/internal/changeset"
	"github.com/jwa91/prehandover/internal/config"
	"github.com/jwa91/prehandover/internal/lifecycle"
	"github.com/jwa91/prehandover/internal/proof"
	"github.com/jwa91/prehandover/internal/report"
	"github.com/jwa91/prehandover/internal/runner"
)

const sampleConfig = `#:schema ./schema.json
# prehandover — unified agent-stop validation

budget = "5s"
parallelism = "auto"
on_unchanged = "skip"
fail_fast = false

exclude = { glob = ["node_modules/**", "dist/**", "build/**", ".git/**"] }

[manifest]
project = "your-repo"
moments = ["agent_stop"]
adapters = ["claude", "codex", "cursor"]
required_prehandover = "0.1.0"

# [[checks]]
# id = "typecheck"
# entry = "tsc --incremental --noEmit"
# files = { glob = "**/*.{ts,tsx}" }
# pass_filenames = false
# budget = "3s"
#
# [[checks]]
# id = "lint"
# entry = "eslint --cache"
# files = { glob = "**/*.{ts,tsx}" }
# budget = "1s"
`

func usage() {
	fmt.Fprintln(os.Stderr, `prehandover — unified agent-stop validation

Usage:
  prehandover run [--format=human|json] [--config=prehandover.toml]
  prehandover hook <harness> [agent_stop] [--config=prehandover.toml]
  prehandover doctor [--config=prehandover.toml]
  prehandover init [--path=prehandover.toml] [--force]
  prehandover validate [--config=prehandover.toml]
  prehandover install [--print] <harness>    (supported: claude, codex, cursor)

Run prehandover from the repo root; it reads prehandover.toml by default.
Exit codes: 0 pass, 1 fail, 2 config error, 3 budget exceeded with no fails.`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "hook":
		os.Exit(cmdHook(os.Args[2:]))
	case "doctor":
		os.Exit(cmdDoctor(os.Args[2:]))
	case "init":
		os.Exit(cmdInit(os.Args[2:]))
	case "validate":
		os.Exit(cmdValidate(os.Args[2:]))
	case "install":
		os.Exit(cmdInstall(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	format := fs.String("format", "human", "output format: human|json")
	cfgPath := fs.String("config", "prehandover.toml", "config file path")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	changed, err := changeset.Changed(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	run, err := runner.Execute(context.Background(), cfg, changed)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	switch *format {
	case "human":
		report.Human(os.Stdout, run)
	case "json":
		if err := report.JSON(os.Stdout, run); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown format: %s\n", *format)
		return 2
	}

	switch run.Status {
	case runner.StatusPass:
		return 0
	case runner.StatusTimeout:
		return 3
	default:
		return 1
	}
}

func cmdHook(args []string) int {
	parsed, err := parseHookArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "usage: prehandover hook <harness> [agent_stop] [--config=prehandover.toml]")
		return 2
	}
	adapter, ok := lifecycle.ForHarness(parsed.harness)
	if !ok {
		fmt.Fprintf(os.Stderr, "unsupported harness: %s\n", parsed.harness)
		return 2
	}
	if !adapter.Supports(parsed.moment) {
		fmt.Fprintf(os.Stderr, "unsupported moment %q for harness %s\n", parsed.moment, adapter.Name())
		return 2
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	inv, err := adapter.Decode(parsed.moment, input)
	if err != nil {
		inv = lifecycle.Invocation{Harness: adapter.Name(), Moment: parsed.moment}
		return encodeHookFailureWithProof(adapter, parsed.moment, inv, parsed.configPath, "hook_input_error", fmt.Errorf("prehandover could not parse hook input: %w", err))
	}
	if inv.CWD != "" {
		if err := os.Chdir(inv.CWD); err != nil {
			return encodeHookFailureWithProof(adapter, parsed.moment, inv, parsed.configPath, "execution_error", fmt.Errorf("prehandover could not use hook cwd %q: %w", inv.CWD, err))
		}
	}

	cfg, err := config.Load(parsed.configPath)
	if err != nil {
		return encodeHookFailureWithProof(adapter, parsed.moment, inv, parsed.configPath, "config_error", fmt.Errorf("prehandover configuration error: %w", err))
	}
	changed, err := changeset.Changed(".")
	if err != nil {
		return encodeHookFailureWithProof(adapter, parsed.moment, inv, parsed.configPath, "changeset_error", fmt.Errorf("prehandover changeset error: %w", err))
	}
	run, err := runner.Execute(context.Background(), cfg, changed)
	if err != nil {
		return encodeHookFailureWithProof(adapter, parsed.moment, inv, parsed.configPath, "execution_error", fmt.Errorf("prehandover execution error: %w", err))
	}
	outcome := lifecycle.OutcomeFromRun(run)
	if err := proof.WriteLatest(proof.FromRun(inv, parsed.configPath, changed, outcome)); err != nil {
		return encodeHookFailure(adapter, parsed.moment, fmt.Sprintf("prehandover proof error: %v", err))
	}
	if err := adapter.Encode(parsed.moment, outcome, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

type hookArgs struct {
	harness    string
	moment     lifecycle.Moment
	configPath string
}

func parseHookArgs(args []string) (hookArgs, error) {
	out := hookArgs{
		moment:     lifecycle.MomentAgentStop,
		configPath: "prehandover.toml",
	}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config" || arg == "-config":
			i++
			if i >= len(args) {
				return out, fmt.Errorf("%s requires a value", arg)
			}
			out.configPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			out.configPath = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-config="):
			out.configPath = strings.TrimPrefix(arg, "-config=")
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) < 1 || len(positional) > 2 {
		return out, fmt.Errorf("expected harness and optional moment")
	}
	out.harness = positional[0]
	if len(positional) == 2 {
		out.moment = lifecycle.Moment(positional[1])
	}
	return out, nil
}

func encodeHookFailure(adapter lifecycle.Adapter, moment lifecycle.Moment, message string) int {
	if err := adapter.Encode(moment, lifecycle.FailureOutcome(message), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func encodeHookFailureWithProof(adapter lifecycle.Adapter, moment lifecycle.Moment, inv lifecycle.Invocation, configPath string, category string, err error) int {
	if proofErr := proof.WriteLatest(proof.Failure(inv, configPath, category, err)); proofErr != nil {
		return encodeHookFailure(adapter, moment, fmt.Sprintf("%v\n\nAdditionally, prehandover could not write proof artifact: %v", err, proofErr))
	}
	return encodeHookFailure(adapter, moment, err.Error())
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	path := fs.String("path", "prehandover.toml", "where to write the config")
	force := fs.Bool("force", false, "overwrite if it exists")
	fs.Parse(args)
	if !*force {
		if _, err := os.Stat(*path); err == nil {
			fmt.Fprintf(os.Stderr, "%s already exists; pass --force to overwrite\n", *path)
			return 1
		}
	}
	if err := os.WriteFile(*path, []byte(sampleConfig), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("wrote %s\n", *path)
	return 0
}

func cmdValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	cfgPath := fs.String("config", "prehandover.toml", "config file path")
	fs.Parse(args)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("ok — %d checks, %s total budget\n", len(cfg.Checks), cfg.Budget.Duration)
	return 0
}
