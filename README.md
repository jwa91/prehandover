# prehandover

The unified hook for agent-to-human handovers.

Pre-commit and CI run at *commit* and *PR* boundaries. But an agent session crosses dozens of edit→test→edit cycles inside a single commit, and the quality contract for those micro-cycles is currently configured *per agent harness* — Claude Code hooks, Codex config, Cursor rules — instead of *per codebase*.

prehandover is one config in your repo that every harness can call at the agent-stop boundary. Same checks and same budget, with harness-specific hook responses handled by adapters.

## Install

```sh
go install github.com/jwa/prehandover/cmd/prehandover@latest
```

## Quickstart

```sh
cd your-repo
prehandover init           # writes prehandover.toml
prehandover validate       # parses + reports
prehandover run            # runs the checks
```

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

The installed hook calls `prehandover hook <harness> handover` at the agent-stop boundary. When a check fails, the adapter emits the harness-specific continuation response and the agent is prompted to fix before stopping.

## Config

`prehandover.toml` clones the [prek](https://prek.j178.dev) field surface — TOML, inline tables, regex or `{glob = "..."}` filters. Notable additions:

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

| `--format=` | Use for |
|---|---|
| `human` | terminal (default) |
| `json` | downstream tooling |

Exit codes: `0` pass, `1` fail, `2` config error, `3` budget exceeded with no fails.

## Hook adapters

`prehandover` names semantic agent-loop boundaries **moments**. The only implemented moment is currently `handover`, which means "the agent is about to hand control back to the human." Harness adapters map that moment onto each product's hook names and response schema.

Supported handover adapters:

| Harness | Installed event | Failure response |
|---|---|---|
| Claude Code | `Stop` | `{"decision":"block","reason":"..."}` |
| Codex | `Stop` | `{"decision":"block","reason":"..."}` |
| Cursor | `stop` | `{"followup_message":"..."}` |

Reserved future moments: `session_context`, `prompt_ingress`, `tool_preflight`, `tool_result`, `worker_handover`, `context_compaction`, and `session_end`. Not every harness exposes every moment, so future adapters should advertise capabilities instead of pretending the lifecycle is uniform everywhere.

## Changeset detection

`git diff --name-only HEAD` ∪ `git ls-files --others --exclude-standard`. Falls back to `git ls-files` on a fresh repo with no HEAD. If you're not in a git repo, every check runs as if `always_run = true`.

## Scope

prehandover sits at the **agent-to-human handover boundary** — the moment the agent stops and hands control back. That's the gap that current tooling doesn't fill.

It is **not**:

- a replacement for pre-commit / [prek](https://prek.j178.dev) — those already cover the commit boundary well.
- a replacement for CI — that already covers the PR / submit boundary well.

Higher-level loops have hooks that are perfectly fine. prehandover only addresses the lower-level loop that doesn't.

## Status

Early. `handover` works and self-hosts. Currently supports Claude Code, Codex, and Cursor. Pi / Amp / opencode adapters are on the roadmap when usable handover hooks exist.

## Credits

The `prehandover.toml` field surface — `id`, `entry`, `args`, `files`, `exclude`, `pass_filenames`, `env`, `priority`, the `{glob = "..."}` extension, and TOML shape — is borrowed from [**prek**](https://prek.j178.dev) (MIT-licensed by [@j178](https://github.com/j178)). prek is the Rust rewrite of pre-commit and is what I use as my commit-time hook manager; prehandover deliberately reuses its vocabulary so the two configs feel like siblings.

## License

[MIT](LICENSE).
