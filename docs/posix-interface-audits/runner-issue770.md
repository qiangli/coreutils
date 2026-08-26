# POSIX interface evidence runner (issue 770)

`scripts/posix_interface_runner.py` is a Python 3 stdlib-only runner for the
source-interface evidence references declared in
`docs/posix-required-command-interfaces.tsv`. It covers the 100 implementation
owned commands in the canonical manifest: 78 `go` rows and 22 `shell` rows.
References are routed by their strict manifest repo prefix. Shell source tests
use only the explicit `sh` root, and shell routing tests use only the explicit
`bashy` root, without command-specific exceptions. Cross-lane duplicate
validation runs before lane-root validation so malformed shared evidence is
reported rather than silently collapsed or obscured by a later wrong-root
diagnostic. The canonical `sh` row now keeps three focused interpreter tests in
the semantic lane and the argv0/selection tests solely in the routing lane.

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
invocations without executing them or creating state. Evidence runs include
`-count=1` to bypass the Go test result cache and `-json` so that every declared
TestID must have exact `run` and `pass` events; a package-only pass, skipped
test, missing test, or malformed event stream fails closed. Use `--json` for
machine-readable output.

`--timeout-seconds` (default 300) and `--max-output-bytes` (default 16777216
combined stdout/stderr bytes per invocation) bound execution. Each test runs in
a new process group. Timeout or output-limit handling terminates and reaps the
group, retains bounded artifacts, and records a failed invocation, including
when a hanging process closes both output pipes before the deadline.

The resolved state directory must be outside every configured evidence root.
Artifact directories and saved resume paths are containment-checked, including
against symlink escape. The ledger is guarded by an advisory lock and atomic
writes. Attempts form a validated, monotonically numbered sequence; failed,
interrupted, and unknown attempts remain in the ledger and are retried.

A successful command is skipped on a later run only when the full contract
hash matches: runner schema version, manifest SHA-256, selected
command/reference set, resolved path/revision/dirty-state hash for every used
evidence root, referenced-test file SHA-256 values, resolved Go executable path
and SHA-256, Go version, `GOOS`/`GOARCH`, limits, and `POSIXLY_CORRECT=1`. The
same absolute, hashed Go executable is used for every evidence invocation.

Each command attempt records timestamps, exact argv, exit status, terminal
pass/fail, stdout/stderr SHA-256 values, and paths to captured stdout/stderr
files under the state directory. Empty output, missing evidence, malformed or
cross-lane duplicate references, unavailable sibling roots, non-`Test...`
identifiers, and failed `go test` invocations are terminal failures.

Limitation: this runner executes source-interface implemented evidence only. It
does not execute or copy VSC-PCTS material, and it must not be used to claim
Profile C/D verified evidence.
