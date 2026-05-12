# prehandover

Unified validation for the moment an agent is about to stop.

Pre-commit and CI run at _commit_ and _PR_ boundaries. But an agent session crosses dozens of edit→test→edit cycles inside a single commit, and the quality contract for those micro-cycles is currently configured _per agent harness_ — Claude Code hooks, Codex config, Cursor rules — instead of _per codebase_.

prehandover is one config in your repo that every harness can call at the `agent_stop` boundary. Same checks and same budget, with harness-specific hook responses handled by adapters.

## Install

```sh
go install github.com/jwa91/prehandover/cmd/prehandover@latest
```

## Quickstart

```sh
cd your-repo
prehandover init           # writes prehandover.toml
prehandover validate       # parses + reports
prehandover doctor         # verifies manifest and installed harness adapters
prehandover run            # runs the checks
```

## Development

This repo keeps project-specific Go tools pinned in `go.mod` and system-level
CLI tools in the dotfiles `Brewfile`.

```sh
make install     # install current checkout to GOPATH/bin for local hooks
make check       # fmt, vet, staticcheck, govulncheck, build, tests
make lint        # golangci-lint
make race        # race detector
```

If a GUI reports `hook exited with code 127`, it could not find the
`prehandover` binary. Run `make install` and make sure `$(go env GOPATH)/bin`
is loaded by your shell startup files.

Wire it into Claude Code:

```sh
prehandover install claude       # merges into .claude/settings.json (idempotent)
prehandover install --print claude   # dry-run
```

Or wire it into another supported harness:

```sh
prehandover install codex        # writes .codex/hooks.json and enables codex_hooks
prehandover install cursor       # writes .cursor/hooks.json
```

The installed hook calls `prehandover hook <harness> agent_stop` at the agent-stop boundary. When a check fails, the adapter emits the harness-specific continuation response and the agent is prompted to fix before stopping.

## Config

`prehandover.toml` starts with a repo-owned manifest that declares the active lifecycle moments and harness adapters:

```toml
budget = "5s"
parallelism = "auto"

[manifest]
project = "your-repo"
moments = ["agent_stop"]
adapters = ["claude", "codex", "cursor"]
required_prehandover = "0.1.0"
```

`prehandover.toml` clones the [prek](https://prek.j178.dev) check field surface — TOML, inline tables, regex or `{glob = "..."}` filters. Notable additions:

- **`budget`** — Go duration string per check and globally. Hard timeout, distinguished from `fail`.
- **`on_unchanged = "skip"`** — default. Skip checks whose `files` glob doesn't intersect the changeset.
- **Parallel by default** — `require_serial = true` to opt out.

```toml
budget = "5s"
parallelism = "auto"
on_unchanged = "skip"

exclude = { glob = ["node_modules/**", "dist/**", ".git/**"] }

[[checks]]
id = "typecheck"
entry = "tsc --incremental --noEmit"
files = { glob = "**/*.{ts,tsx}" }
pass_filenames = false
budget = "3s"

[[checks]]
id = "lint"
entry = "eslint --cache"
files = { glob = "**/*.{ts,tsx}" }
budget = "1s"

[[checks]]
id = "custom"
entry = "./scripts/no-todo-fixme.sh"
budget = "200ms"
```

## Output formats

| `--format=` | Use for            |
| ----------- | ------------------ |
| `human`     | terminal (default) |
| `json`      | downstream tooling |

Exit codes: `0` pass, `1` fail, `2` config error, `3` budget exceeded with no fails.

## Hook adapters

`prehandover` names semantic agent-loop boundaries **moments**. The only implemented moment is currently `agent_stop`, which means "the agent is about to stop." Harness adapters map that moment onto each product's hook names and response schema.

Supported `agent_stop` adapters:

| Harness     | Installed event | Failure response                      |
| ----------- | --------------- | ------------------------------------- |
| Claude Code | `Stop`          | `{"decision":"block","reason":"..."}` |
| Codex       | `Stop`          | `{"decision":"block","reason":"..."}` |
| Cursor      | `stop`          | `{"followup_message":"..."}`          |

Reserved future moments: `session_context`, `prompt_ingress`, `tool_preflight`, `tool_result`, `worker_stop`, `context_compaction`, and `session_end`. Not every harness exposes every moment, so future adapters should advertise capabilities instead of pretending the lifecycle is uniform everywhere.

## Proof artifacts

Every `agent_stop` hook run writes `.prehandover/runs/latest.json`. This file records the moment, harness, config hash, changed files, check results, timing, and rejection category. The path is gitignored because it is a local run artifact, but it gives the next human or agent a durable stop record.

## Changeset detection

`git diff --name-only HEAD` ∪ `git ls-files --others --exclude-standard`. Falls back to `git ls-files` on a fresh repo with no HEAD. If you're not in a git repo, every check runs as if `always_run = true`.

## Scope

prehandover sits at the **agent-stop boundary** — the moment an agent is about to stop before the next actor takes over. That's the gap that current tooling doesn't fill.

It is **not**:

- a replacement for pre-commit / [prek](https://prek.j178.dev) — those already cover the commit boundary well.
- a replacement for CI — that already covers the PR / submit boundary well.

Higher-level loops have hooks that are perfectly fine. prehandover only addresses the lower-level loop that doesn't.

## Status

Early. `agent_stop` works and self-hosts. Currently supports Claude Code, Codex, and Cursor. Pi / Amp / opencode adapters are on the roadmap when usable agent-stop hooks exist.

## Credits

The `prehandover.toml` field surface — `id`, `entry`, `args`, `files`, `exclude`, `pass_filenames`, `env`, `priority`, the `{glob = "..."}` extension, and TOML shape — is borrowed from [**prek**](https://prek.j178.dev) (MIT-licensed by [@j178](https://github.com/j178)). prek is the Rust rewrite of pre-commit and is what I use as my commit-time hook manager; prehandover deliberately reuses its vocabulary so the two configs feel like siblings.

## License

[MIT](LICENSE).
