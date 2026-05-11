# prehandover

The unified hook for agent-to-human handovers.

Pre-commit and CI run at *commit* and *PR* boundaries. But an agent session crosses dozens of edit→test→edit cycles inside a single commit, and the quality contract for those micro-cycles is currently configured *per agent harness* — Claude Code hooks, Codex config, Cursor rules — instead of *per codebase*.

prehandover is one config in your repo that every harness can call at the agent-stop boundary. Same checks, same budget, same output shape, no matter which agent made the edit.

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

```json
// .claude/settings.json
{
  "hooks": {
    "Stop": [
      { "matcher": "", "hooks": [{
        "type": "command",
        "command": "prehandover run --format=claude"
      }]}
    ]
  }
}
```

When a check fails, Claude Code sees `{"decision":"block","reason":"..."}` and the agent is prompted to fix before stopping.

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
| `claude` | Claude Code Stop hook — emits `{decision, reason}` |

Exit codes: `0` pass, `1` fail, `2` config error, `3` budget exceeded with no fails.

## Changeset detection

`git diff --name-only HEAD` ∪ `git ls-files --others --exclude-standard`. Falls back to `git ls-files` on a fresh repo with no HEAD. If you're not in a git repo, every check runs as if `always_run = true`.

## Status

Early. The `on-stop` phase is the only one implemented. Adding more phases (`on-edit`, `on-commit`, `on-submit`) is the obvious next move; probabilistic checks with precision tracking come after.
