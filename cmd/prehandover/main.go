package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jwa/prehandover/internal/changeset"
	"github.com/jwa/prehandover/internal/config"
	"github.com/jwa/prehandover/internal/report"
	"github.com/jwa/prehandover/internal/runner"
)

const sampleConfig = `#:schema https://prehandover.dev/schema.json
# prehandover — unified hook for agent-to-human handovers

budget = "5s"
parallelism = "auto"
on_unchanged = "skip"
fail_fast = false

exclude = { glob = ["node_modules/**", "dist/**", "build/**", ".git/**"] }

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
	fmt.Fprintln(os.Stderr, `prehandover — unified hook for agent-to-human handovers

Usage:
  prehandover run [--format=human|json|claude] [--config=prehandover.toml]
  prehandover init [--path=prehandover.toml] [--force]
  prehandover validate [--config=prehandover.toml]
  prehandover install [--print] <harness>    (supported: claude)

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
	format := fs.String("format", "human", "output format: human|json|claude")
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
	case "claude":
		if err := report.Claude(os.Stdout, run); err != nil {
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
