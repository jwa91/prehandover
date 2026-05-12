# Repository hardening

This project is a distributed CLI, so release and CI configuration should be treated as sensitive code.

## GitHub settings

Enable these protections for `main` in GitHub repository settings:

- Require a pull request before merging.
- Require approvals.
- Require review from Code Owners.
- Require status checks to pass before merging.
- Require the `CI / Check` and `CI / Dependency Review` checks.
- Require branches to be up to date before merging.
- Block force pushes.
- Block deletions.

For repository actions settings:

- Set default workflow token permissions to read-only.
- Require approval for first-time external contributors.
- Prefer GitHub-hosted runners for public pull requests.

## Local expectations

Before opening a PR:

```sh
make test
```

Before merging broad changes:

```sh
make check
```

## Release expectations

Release automation should be tag-driven. A release tag like `v0.1.0` should:

- run the full check suite,
- build release artifacts from a clean checkout,
- publish checksums,
- update the Homebrew formula,
- produce release notes or a changelog entry.

Do not grant write permissions to normal CI jobs. Release jobs should request only the write scopes they need.

