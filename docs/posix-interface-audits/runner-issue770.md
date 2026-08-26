# POSIX interface evidence runner (issue 770)

`scripts/posix_interface_runner.py` is a Python 3 stdlib-only runner for the
source-interface evidence references declared in
`docs/posix-required-command-interfaces.tsv`. It covers the 100 implementation
owned commands in the canonical manifest: 78 `go` rows and 22 `shell` rows.

Intended invocation:

```sh
POSIX_SH_EVIDENCE_ROOT=/path/to/sh \
POSIX_BASHY_EVIDENCE_ROOT=/path/to/bashy \
python3 scripts/posix_interface_runner.py --all --state-dir /path/outside/worktree
```

Focused selections are deterministic:

```sh
python3 scripts/posix_interface_runner.py at --state-dir /path/outside/worktree
python3 scripts/posix_interface_runner.py at awk cat --state-dir /path/outside/worktree
python3 scripts/posix_interface_runner.py --owner go --state-dir /path/outside/worktree
python3 scripts/posix_interface_runner.py --owner shell --state-dir /path/outside/worktree
```

Use `--dry-run` to validate references and print the package-scoped `go test`
invocations without executing them. Use `--json` for machine-readable output.

The state directory must be outside the worktree. The ledger is guarded by an
advisory lock and atomic writes. A successful command is skipped on a later run
only when the full contract hash matches: runner schema version, manifest
SHA-256, selected command/reference set, git revisions for every evidence root,
Go version, `GOOS`/`GOARCH`, and `POSIXLY_CORRECT=1`. Failed, interrupted, and
unknown attempts remain in the ledger and are retried.

Each command attempt records timestamps, exact argv, exit status, terminal
pass/fail, stdout/stderr SHA-256 values, and paths to captured stdout/stderr
files under the state directory. Empty output, missing evidence, malformed
references, unavailable sibling roots, non-`Test...` identifiers, and failed
`go test` invocations are terminal failures.

Limitation: this runner executes source-interface implemented evidence only. It
does not execute or copy VSC-PCTS material, and it must not be used to claim
Profile C/D verified evidence.
