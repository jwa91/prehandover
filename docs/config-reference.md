# Config reference

Every field accepted in `prehandover.toml`, in the same order they appear in [`schema.json`](../schema.json). The schema is the source of truth; this page is the prose mirror.

```
prehandover.toml
├── manifest        (required)
├── budget
├── parallelism
├── on_unchanged
├── fail_fast
├── files
├── exclude
└── checks          [[checks]] array
```

## Top-level

### `manifest` *(required)*

The repo-owned operating contract. Without it, prehandover refuses to load the config.

```toml
[manifest]
project = "your-repo"
moments = ["agent_stop"]
adapters = ["claude", "codex", "cursor"]
required_prehandover = "0.1.0"
```

See [`[manifest]`](#manifest-table) below for the field list.

### `budget`

**Type:** Go duration string (`"500ms"`, `"3s"`, `"1m"`). **Default:** `"5s"`.

Total wall-clock budget for the whole run. Hard timeout. A check that's still running when the global budget expires is killed and reported as `timeout`. Distinguished from `fail` in both the proof artifact (`category: "budget_exceeded"`) and the exit code (`3` vs `1`).

A per-check `budget` overrides this; if absent on a check, the global budget is inherited.

### `parallelism`

**Type:** string. **Default:** `"auto"`.

`"auto"` uses the CPU count. Otherwise a stringified integer (`"1"`, `"4"`). Set to `"1"` to force serial execution of all checks; this is useful when running in CI alongside other workloads that contend for the same cores.

Per-check `require_serial = true` opts a single check out of parallel scheduling without forcing the rest of the run serial.

### `on_unchanged`

**Type:** enum `"skip"` | `"run"`. **Default:** `"skip"`.

What to do for a check whose `files` glob doesn't intersect the changeset. `"skip"` is the default and the reason prehandover is fast enough to live at agent-stop — most checks have no work to do on most invocations.

Set to `"run"` if you want every check to fire every time regardless of changeset. Usually you want `always_run = true` on a single check instead.

### `fail_fast`

**Type:** boolean. **Default:** `false`.

When `true`, stop running serial checks after the first failure. Has no effect on checks running in parallel — they've already started. Set `require_serial = true` plus `fail_fast = true` to get strict short-circuit behavior.

### `files`

**Type:** [pattern](#pattern). **Default:** none.

Repo-wide include filter applied to every check before per-check `files` is evaluated. Rarely used; most repos prefer per-check filters.

### `exclude`

**Type:** [pattern](#pattern). **Default:** none.

Repo-wide exclude filter. Common values: `{ glob = ["node_modules/**", "dist/**", ".git/**"] }`. Per-check `exclude` is applied additionally, not as a replacement.

### `checks`

**Type:** array of [check tables](#check-table).

The work prehandover actually does. Each entry is a `[[checks]]` block.

---

## `[manifest]` table

### `project` *(required)*

**Type:** non-empty string.

Repo or project name. Stored verbatim in the proof artifact; useful when one machine accumulates `latest.json` files from several repos.

### `moments` *(required)*

**Type:** array of enum, min length 1, unique.

Lifecycle moments this repo participates in. Today only `"agent_stop"` is implemented. Future-reserved values accepted by the parser: `"session_context"`, `"prompt_ingress"`, `"tool_preflight"`, `"tool_result"`, `"worker_stop"`, `"context_compaction"`, `"session_end"`.

Including a reserved moment in `moments` does nothing today but won't break — your config stays forward-compatible.

### `adapters` *(required)*

**Type:** array of enum, min length 1, unique. Allowed: `"claude"`, `"codex"`, `"cursor"`.

Harness adapters this repo expects to be wired up. `prehandover doctor` cross-checks this list against the installed hooks and reports drift.

### `required_prehandover` *(required)*

**Type:** semver string matching `^\d+\.\d+\.\d+$`, e.g. `"0.1.0"`.

Minimum binary version. A binary older than this refuses to run. Pin this to the version you tested against so a stale `prehandover` on a contributor's machine doesn't silently skip new check fields.

---

## `[[checks]]` table

### `id` *(required)*

**Type:** non-empty string.

Stable identifier. Used in the proof artifact, in `--format=json` output, and in failure messages. Treat it as a public name — renaming breaks downstream tooling that grep the artifact.

### `name`

**Type:** string. **Default:** falls back to `id`.

Human-readable label shown in `--format=human` output.

### `entry` *(required)*

**Type:** non-empty string.

The command to run. Whitespace-split into argv unless `shell` is set (see below). The first token is resolved against `PATH`; relative paths like `./scripts/check.sh` are resolved against the repo root.

### `args`

**Type:** array of string. **Default:** `[]`.

Extra arguments appended after the `entry` split and after any filenames passed via `pass_filenames`.

### `files`

**Type:** [pattern](#pattern). **Default:** matches all.

Filter applied to the changeset. Only files matching this pattern are passed to `entry` (when `pass_filenames` is true) and only files matching this pattern count for the `on_unchanged` skip decision.

### `exclude`

**Type:** [pattern](#pattern). **Default:** none.

Subtracted from `files`. Both repo-level `exclude` and per-check `exclude` apply.

### `pass_filenames`

**Type:** boolean *or* positive integer. **Default:** `true`.

- `true` — pass all matching files as positional args to `entry` (after `args`).
- `false` — pass no filenames. Use for whole-repo tools like `go vet ./...`, `tsc --noEmit`, `golangci-lint run ./...`.
- *N* (integer) — pass at most N files per invocation; if there are more, prehandover invokes `entry` multiple times. Default behavior matches prek.

### `always_run`

**Type:** boolean. **Default:** `false`.

If `true`, the check runs even when the changeset doesn't intersect `files`. Bypasses `on_unchanged`. Use sparingly — defeating the changeset filter is what makes prehandover slow.

### `require_serial`

**Type:** boolean. **Default:** `false`.

Opt this check out of parallel scheduling. Use when the check contends for a shared resource (a TCP port, a build cache, a generated artifact in the working tree).

### `verbose`

**Type:** boolean. **Default:** `false`.

Show check output even on pass. Default is silent-on-pass.

### `env`

**Type:** map of string → string. **Default:** `{}`.

Extra environment variables for this check. Merged on top of the inherited environment; per-check `env` wins over the parent.

### `priority`

**Type:** integer. **Default:** `0`.

Lower values run first within serial execution. Has no effect on parallel checks beyond influencing the launch order. Use to put a fast lint *before* a slow type-check when `fail_fast = true`.

### `budget`

**Type:** Go duration string. **Default:** inherits global `budget`.

Per-check hard timeout. Killed-and-reported behavior identical to the global budget (see [`budget`](#budget) at top level).

### `description`

**Type:** string. **Default:** none.

Free-form documentation. Surfaced in `prehandover doctor` and in the JSON report.

### `shell`

**Type:** enum `"sh"` | `"bash"`. **Default:** none.

When set, `entry` is run as `<shell> -c "<entry>"`. Required for pipes, redirects, `&&`, glob expansion in the command itself. When unset, `entry` is whitespace-split and executed directly — this is faster and safer; prefer it.

---

## Pattern

Three accepted forms, in order of preference:

```toml
files = "regex string"                  # regex
files = { glob = "**/*.{ts,tsx}" }      # glob (one)
files = { glob = ["**/*.go", "go.mod"] } # glob (many)
files = { regex = "^cmd/.*\\.go$" }     # regex in inline-table form
```

`{glob = "..."}` is the prek-borrowed extension and the form you should reach for first — globs are clearer to read and easier for an agent to author correctly. Reach for regex only when the glob can't express the predicate.

## Duration

Go `time.ParseDuration` format. Valid units: `ns`, `us`/`µs`, `ms`, `s`, `m`, `h`. Compound forms work: `"1m30s"`, `"500ms"`, `"2h45m"`. The schema regex enforces this; an unparseable duration fails `prehandover validate`.

## Defaults summary

| Field            | Default      |
| ---------------- | ------------ |
| `budget`         | `"5s"`       |
| `parallelism`    | `"auto"`     |
| `on_unchanged`   | `"skip"`     |
| `fail_fast`      | `false`      |
| check `budget`   | inherits global |
| `pass_filenames` | `true`       |
| `always_run`     | `false`      |
| `require_serial` | `false`      |
| `verbose`        | `false`      |
| `priority`       | `0`          |

## Editor support

The shipped config starts with `#:schema ./schema.json` so editors that understand the [Taplo](https://taplo.tamasfe.dev/) schema directive (VS Code's Even Better TOML, Helix, Zed) get autocomplete and inline validation. Keep that comment when you edit by hand.
