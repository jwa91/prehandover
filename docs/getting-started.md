# Getting started

Five minutes from zero to a Claude Code session that blocks itself when a check fails.

## 1. Install the binary

```sh
go install github.com/jwa91/prehandover/cmd/prehandover@latest
prehandover --help
```

`go install` puts the binary in `$GOBIN` (or `$GOPATH/bin`). If `prehandover --help` is "command not found", add that directory to your `PATH`.

## 2. Initialize the config

From the root of the repo you want to protect:

```sh
prehandover init
```

This writes `prehandover.toml`. Open it. The interesting part is the manifest:

```toml
[manifest]
project = "your-repo"
moments = ["agent_stop"]
adapters = ["claude", "codex", "cursor"]
required_prehandover = "0.1.0"
```

The manifest is a **declaration** — "this repo participates in the `agent_stop` lifecycle moment and supports these three harnesses." Everything else in the file (checks, budget, file filters) is the contract that gets enforced when that moment fires.

## 3. Add your first check

Replace the commented-out sample with one real check. A trivial one to start:

```toml
[[checks]]
id = "no-todo-in-staged"
entry = "./scripts/no-todo.sh"
files = { glob = "**/*.{ts,tsx,go,py}" }
budget = "200ms"
```

Then validate it parses:

```sh
prehandover validate
# ok — 1 checks, 5s total budget
```

And dry-run it without any hook wiring:

```sh
prehandover run
```

You should see a green pass (no TODO files were touched) or a red fail with the offending file. **This is the same code path the hook will execute** — `prehandover run` and `prehandover hook` share their runner. If `run` passes, the hook will pass.

## 4. Wire it into your harness

Pick one. The install command is idempotent — running it twice is a no-op.

**Claude Code:**
```sh
prehandover install claude
# installed Stop hook in .claude/settings.json
```

**Codex:**
```sh
prehandover install codex
```

**Cursor:**
```sh
prehandover install cursor
```

Dry-run any of them by passing `--print`:

```sh
prehandover install --print claude
```

That prints the settings file as it would be written, so you can review the merge before it touches disk.

## 5. Verify

Run the doctor:

```sh
prehandover doctor
```

It checks that the manifest is well-formed, that every adapter listed in the manifest is installed, and that the installed hook actually calls `prehandover hook <harness> agent_stop`. Green doctor = the wiring is real.

## 6. Watch it fire

Open your agent (e.g., Claude Code) inside the repo. When the agent is about to stop, the harness invokes the installed hook, which runs `prehandover hook claude agent_stop`. Two paths:

- **All checks pass.** The hook exits silently, the agent stops normally. Nothing visible.
- **A check fails.** The adapter emits `{"decision":"block","reason":"..."}` and the agent is prompted to fix before stopping. The reason includes the failing check's output verbatim.

To make this happen on purpose, introduce a TODO into a file the agent edited and let it try to stop. You should see the block.

## 7. Read the proof artifact

After every hook invocation — pass or fail — prehandover writes `.prehandover/runs/latest.json`. This is the durable record of what happened at the boundary. It contains the moment, the harness, the config hash, the changed files, each check's status, and the rejection category if anything blocked.

```sh
cat .prehandover/runs/latest.json
```

The path is in `.gitignore` by default. It's a local artifact, not a commit-time concern — but it's the first place to look when "the agent stopped weirdly and I don't know why."

## Where to go from here

- **[Concepts](./concepts.md)** — the five primitives (moments, adapters, checks, changeset, budget) and how they compose in one invocation. Read this before you write checks beyond the first.
- **[Config reference](./config-reference.md)** — every field in `prehandover.toml`, mirrored against `schema.json`.

## If something went wrong

- `prehandover validate` errors → fix the TOML; the error message names the field.
- `prehandover doctor` reports a missing adapter → re-run `prehandover install <harness>`.
- Hook seems to never fire → check that the installed command in your harness's settings file actually points at `prehandover hook ...` (the binary must be on the harness's `PATH` too, which may differ from your shell's).
- Hook fires but always passes → likely `on_unchanged = "skip"` skipping every check because your changeset is empty; commit or stage some files and try again.
