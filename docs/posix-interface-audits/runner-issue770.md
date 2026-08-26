# POSIX Interface Evidence Runner (Issue 770)

## Intended Invocation
The POSIX interface runner validates that exact focused evidence tests successfully run for all `go` and `shell` owned commands specified in the manifest.

### Example Usage:
```sh
# Run tests for a specific command
./scripts/posix_interface_runner.py at awk --state-dir /tmp/posix_ledger

# Run tests for all go-owned commands
./scripts/posix_interface_runner.py --owner go --state-dir /tmp/posix_ledger

# Run all owned commands
./scripts/posix_interface_runner.py --all --state-dir /tmp/posix_ledger
```
The state directory holds a resumable ledger bound to the manifest, selected commands, and Exact Git Revisions.

## Limitations
This runner executes **source-interface implemented evidence only**. It is strictly for executing native testing coverage declared in `go_evidence`, `shell_evidence`, and `shell_routing_evidence`.

It must **never** be used to run or report **Profile C/D verified evidence** or to execute VSC-PCTS material.
