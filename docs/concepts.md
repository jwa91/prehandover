# Concepts

prehandover is built from five primitives. They compose in a fixed order during a single hook invocation: **harness → adapter → moment → changeset → checks → outcome → proof**. Read them in that order and the rest of the tool falls out as glue.

## The shape of one invocation

```
agent finishes turn
     │
     ▼
  harness fires its hook    (Claude "Stop", Codex "Stop", Cursor "stop")
     │
     ▼
  adapter.Decode()          harness-specific JSON → Invocation
     │
     ▼
  moment dispatch           today: only agent_stop
     │
     ▼
  changeset detection       git diff ∪ untracked
     │
     ▼
  checks run                filtered by changeset, bounded by budget
     │
     ▼
  Outcome                   {Allow, ContinueMessage}
     │
     ▼
  adapter.Encode()          Outcome → harness-specific response
     │
     ▼
  proof.WriteLatest()       .prehandover/runs/latest.json
```

The whole point of this diagram is that **only two arrows touch the harness** — the first (`Decode`) and the second-to-last (`Encode`). Everything in between is harness-agnostic. That is the design.

## Moment

A **moment** is a semantic point in an agent's lifecycle: "the agent is about to stop", "a tool is about to run", "context is about to compact". Moments are *prehandover's* vocabulary, not any harness's.

This matters because every harness names its hook events differently — Claude has `Stop`, Codex has `Stop` (different JSON shape), Cursor has `stop` (different response schema, lowercase). prehandover gives you one name (`agent_stop`) and pushes the per-harness translation into adapters.

Today only one moment is implemented: `agent_stop`. The reserved names — `session_context`, `prompt_ingress`, `tool_preflight`, `tool_result`, `worker_stop`, `context_compaction`, `session_end` — exist in the manifest enum so future configs are forward-compatible, but they don't fire.

**Why this is its own primitive:** because the agent lifecycle is *not* uniform across harnesses. Not every harness exposes every moment. Cursor doesn't have `tool_preflight`. Codex doesn't expose `context_compaction`. A clean moment abstraction lets adapters advertise capabilities rather than pretending the lifecycle is the same everywhere.

## Adapter

An **adapter** is the harness-shaped boundary on either side of the run. It does exactly two things:

1. **Decode** the harness's JSON payload (which carries `cwd`, `session_id`, transcript path, etc.) into a uniform `Invocation` struct.
2. **Encode** an `Outcome` (`{Allow, ContinueMessage}`) into the response shape the harness understands.

The response shapes diverge in revealing ways:

| Harness     | Hook event | Block response                          |
| ----------- | ---------- | --------------------------------------- |
| Claude Code | `Stop`     | `{"decision":"block","reason":"..."}`   |
| Codex       | `Stop`     | `{"decision":"block","reason":"..."}`   |
| Cursor      | `stop`     | `{"followup_message":"..."}`            |

Claude and Codex use a control-flow vocabulary (`decision: block`). Cursor uses a message-passing vocabulary (`followup_message`). Same semantic outcome, two different mental models. The adapter absorbs that difference so checks don't have to know.

An adapter must `Supports(moment)` before it can `Encode` — that's the capability check. A future Cursor adapter that gains `tool_preflight` will return `true` for that moment and `false` for the others it can't see.

## Check

A **check** is one command that returns 0 for pass and non-zero for fail. That's it.

The check surface is borrowed wholesale from [prek](https://prek.j178.dev) — `id`, `entry`, `files`, `pass_filenames`, `env`, `priority`, the `{glob = "..."}` extension. If you've written a `.pre-commit-config.yaml`, `prehandover.toml` is the same shape. The borrow is deliberate: a check at commit time and a check at agent-stop time are the same conceptual object — same command, different trigger.

Three rules govern how checks run:

- **Filter by changeset.** A check's `files` glob is intersected with the changeset. If the intersection is empty and `on_unchanged = "skip"` (default), the check skips. Set `always_run = true` to bypass.
- **Parallel by default.** Checks run concurrently up to `parallelism` (default: CPU count). Set `require_serial = true` on a check that contends for a shared resource (a port, a lockfile, a generated artifact).
- **Bounded by budget.** Each check has a per-check budget; the run has a global budget. Both are hard timeouts. A check that exceeds its budget is killed and reported as `timeout`, *not* `fail` — the proof artifact distinguishes the two and so does the exit code (`3` vs `1`).

The full field list is in the [config reference](./config-reference.md).

## Changeset

The changeset is `git diff --name-only HEAD ∪ git ls-files --others --exclude-standard` — i.e., **what's changed since HEAD, plus what's new**. On a fresh repo with no HEAD, it falls back to `git ls-files`. Outside a git repo, every check runs as if `always_run = true`.

This is the same definition prek uses and for the same reason: it's the smallest "what an agent has touched" set you can compute without reading the agent's transcript. Stage-vs-working-tree doesn't matter — both count. A file deleted, renamed, or untracked-but-existing all count.

**Why this is a primitive:** because "run only the checks that matter for the files the agent touched" is the whole reason the tool is fast enough to belong at the agent-stop boundary. A check that takes 3 seconds on the full repo can be 50 ms when it sees only the two files the agent edited. Without changeset detection, prehandover would be a slower CI; with it, prehandover is something you can afford to run on every stop.

## Budget

Two budgets, both hard:

- **Per-check** (`budget` on a `[[checks]]` entry) — defaults to the global budget if absent.
- **Global** (`budget` at the top level, default `5s`) — total wall-clock for the whole run.

Budget is *distinct from fail*. A check that fails is reported with status `fail` and category `validator_failed`. A check that exceeds its budget is reported with status `timeout` and category `budget_exceeded`. The hook still blocks the agent in both cases, but the proof artifact tells you which happened, and `prehandover run` exits with a different code (`1` for fail, `3` for budget).

Why split them: a flaky 30-second check that occasionally finishes is a different problem from a check that says "I found a bug." You want the agent to fix the bug; you want *you* to fix the flake. Same surface for both is a bug, not a feature.

## Outcome and proof

The `Outcome` is the small struct that flows out of the runner: `{Allow bool, ContinueMessage string}`. If `Allow` is true, the adapter encodes nothing (silent pass). If false, the adapter encodes the harness-specific block response and includes the `ContinueMessage` — which by default is the concatenated output of every failing check.

After encode, prehandover writes `.prehandover/runs/latest.json`. The artifact captures:

- The moment, harness, cwd, session/turn IDs (when the harness provides them).
- The config path and its SHA-256 — so you can tell if the config drifted between the run that blocked and the run you're debugging.
- The changed files seen.
- Per-check: id, status, duration, budget, reason, output.
- A **category**: one of `passed`, `validator_failed`, `validator_error`, `budget_exceeded`, `config_error`, `changeset_error`, `execution_error`, `hook_input_error`.

The category is the most useful field for triage. "Why did my agent stop?" gets answered by reading one string.

## What prehandover is not

prehandover sits at exactly one boundary: **agent-stop**, the moment between turns when an agent is about to hand control back. That gap is currently configured *per agent harness* — Claude Code hooks, Codex config, Cursor rules — instead of *per codebase*. prehandover replaces those per-harness configurations with one config the codebase owns.

It is **not** a replacement for prek / pre-commit — those own the commit boundary, which is a different actor (human commits) and a different cadence (per commit, not per agent turn). It is **not** a replacement for CI — that owns the PR boundary, which has a longer budget and different inputs.

Pre-commit and CI are fine. The agent-stop boundary is the one that didn't have shared infrastructure. prehandover only addresses that.
