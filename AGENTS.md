# AGENTS.md

`prehandover` is a Go CLI that provides one repo-owned hook configuration for multiple agent harnesses. It currently targets the `agent_stop` moment: the point where an agent is about to stop and hand control back.

## Project Shape

- `cmd/prehandover/` contains the CLI commands (`run`, `hook`, `install`, `doctor`, `validate`, `init`).
- `internal/config/` owns `prehandover.toml` parsing and schema-shaped defaults.
- `internal/lifecycle/` defines semantic agent-loop moments; only `agent_stop` is implemented today.
- `internal/runner/` executes checks, applies budgets, and produces outcomes.
- `internal/changeset/` detects changed files from git diff plus untracked files.
- `internal/filter/` handles regex/glob file matching.
- `internal/report/` formats human/JSON output and exit categories.
- `internal/proof/` writes `.prehandover/runs/latest.json`.
- `schema.json` and `docs/config-reference.md` are the public config contract.

## Design Rules

- This project is still early; do not preserve backwards compatibility when a better structural choice is needed.
- If a structural change is chosen, apply it consistently across the codebase instead of keeping compatibility shims or mixed patterns.
- Code simplicity and consistency are core pillars.
- Keep harness-specific behavior in adapters: decode hook input and encode the harness response.
- Everything between those steps should stay harness-agnostic.
- Treat moments as prehandover vocabulary, not product hook names.
- Preserve the prek-like check surface in `prehandover.toml`; prefer compatible additions over new concepts.
- Budgets are hard timeouts and distinct from check failures.
- Changeset filtering is core to agent-stop latency; do not bypass it casually.
- Proof artifacts are local run records and should remain gitignored.

## Development

- Use `make test` for focused verification and `make check` before broad changes.
- Keep tests near the package being changed.
- Run `./scripts/gofmt-strict.sh` or `make fmt` after Go edits.
