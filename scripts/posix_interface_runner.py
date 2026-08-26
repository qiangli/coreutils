#!/usr/bin/env python3
"""Run declared POSIX interface evidence without proprietary suites.

The runner consumes docs/posix-required-command-interfaces.tsv, validates the
focused source-test references for owned Go and shell commands, and records a
resumable evidence ledger in a caller supplied state directory.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import os
import platform
import re
import subprocess
import sys
import tempfile
import time
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    import fcntl
except ImportError:  # pragma: no cover - this runner is POSIX-oriented.
    fcntl = None


ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/posix-required-command-interfaces.tsv"
LEDGER_NAME = "posix-interface-runner-ledger.json"
LOCK_NAME = "posix-interface-runner.lock"
SCHEMA_VERSION = "posix-interface-runner-v1"
OWNERS = frozenset({"go", "shell"})
TEST_ID = re.compile(r"^Test[A-Za-z0-9_]*$")
LOCAL_REF = re.compile(r"^(?P<path>[^:#]+\.go)#(?P<test>Test[A-Za-z0-9_]*)$")
PREFIXED_REF = re.compile(
    r"^(?P<repo>sh|bashy):(?P<path>[^:#]+\.go)#(?P<test>Test[A-Za-z0-9_]*)$"
)


class RunnerError(ValueError):
    pass


@dataclass(frozen=True, order=True)
class EvidenceRef:
    repo: str
    path: str
    test: str
    lane: str
    raw: str

    @property
    def package(self) -> str:
        package = Path(self.path).parent.as_posix()
        return "./" + package if package != "." else "."


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def json_dumps(data: Any) -> str:
    return json.dumps(data, indent=2, sort_keys=True)


def atomic_write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = (json_dumps(data) + "\n").encode()
    fd, name = tempfile.mkstemp(prefix=path.name + ".", suffix=".tmp", dir=path.parent)
    tmp = Path(name)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(payload)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(tmp, path)
        directory_fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        if tmp.exists():
            tmp.unlink()


def atomic_write_bytes(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=path.name + ".", suffix=".tmp", dir=path.parent)
    tmp = Path(name)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(data)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(tmp, path)
    finally:
        if tmp.exists():
            tmp.unlink()


def read_manifest(path: Path) -> list[dict[str, str]]:
    with path.open(newline="") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        fieldnames = set(reader.fieldnames or ())
        rows = list(reader)
    required = {"command", "effective_owner", "go_evidence", "shell_evidence", "shell_routing_evidence"}
    missing = required - fieldnames
    if missing:
        raise RunnerError(f"manifest is missing required field(s): {', '.join(sorted(missing))}")
    return rows


def inside(child: Path, parent: Path) -> bool:
    try:
        child.relative_to(parent)
        return True
    except ValueError:
        return False


def resolve_state_dir(path: Path, root: Path, *, create: bool = True) -> Path:
    state_dir = path.expanduser().resolve()
    if inside(state_dir, root.resolve()):
        raise RunnerError("state directory must be outside the worktree")
    if create:
        state_dir.mkdir(parents=True, exist_ok=True)
    return state_dir


def configured_roots(root: Path, env: dict[str, str]) -> dict[str, Path]:
    roots = {"coreutils": root.resolve()}
    if env.get("POSIX_SH_EVIDENCE_ROOT"):
        roots["sh"] = Path(env["POSIX_SH_EVIDENCE_ROOT"]).expanduser().resolve()
    if env.get("POSIX_BASHY_EVIDENCE_ROOT"):
        roots["bashy"] = Path(env["POSIX_BASHY_EVIDENCE_ROOT"]).expanduser().resolve()
    return roots


def validate_relative_path(command: str, ref: str, relative: str) -> Path:
    path = Path(relative)
    if path.is_absolute() or ".." in path.parts or path.name in {"", "."}:
        raise RunnerError(f"{command}: evidence path escapes its repository: {ref}")
    if not path.name.endswith("_test.go"):
        raise RunnerError(f"{command}: evidence path is not a focused Go test: {ref}")
    return path


def test_is_declared(path: Path, test: str) -> bool:
    source = path.read_text()
    return bool(re.search(rf"^func\s+{re.escape(test)}\s*\(", source, re.MULTILINE))


def parse_ref(command: str, lane: str, raw: str, roots: dict[str, Path]) -> EvidenceRef:
    if lane == "go_evidence":
        match = LOCAL_REF.fullmatch(raw)
        if not match:
            raise RunnerError(f"{command}: malformed go_evidence reference: {raw}")
        repo = "coreutils"
    else:
        match = PREFIXED_REF.fullmatch(raw)
        if not match:
            raise RunnerError(f"{command}: malformed {lane} reference: {raw}")
        repo = match.group("repo")
        if lane == "shell_evidence" and repo not in {"sh", "bashy"}:
            raise RunnerError(f"{command}: wrong-owner evidence root in {raw}")
        if lane == "shell_routing_evidence" and repo != "bashy":
            raise RunnerError(f"{command}: shell routing evidence must use bashy root: {raw}")

    path = validate_relative_path(command, raw, match.group("path"))
    test = match.group("test")
    if not TEST_ID.fullmatch(test):
        raise RunnerError(f"{command}: evidence identifier is not a Test ID: {raw}")
    if repo not in roots:
        raise RunnerError(f"{command}: unavailable evidence root {repo!r}; set its explicit root")
    full_path = roots[repo] / path
    if not full_path.is_file():
        raise RunnerError(f"{command}: evidence path is missing: {raw}")
    if not test_is_declared(full_path, test):
        raise RunnerError(f"{command}: evidence Test ID is missing: {raw}")
    return EvidenceRef(repo=repo, path=path.as_posix(), test=test, lane=lane, raw=raw)


def split_refs(raw: str) -> list[str]:
    if raw == "-":
        return []
    refs = raw.split(";")
    if any(not ref for ref in refs):
        raise RunnerError("empty evidence reference")
    duplicates = sorted(ref for ref in set(refs) if refs.count(ref) > 1)
    if duplicates:
        raise RunnerError(f"duplicate evidence reference(s): {', '.join(duplicates)}")
    return refs


def refs_for_row(row: dict[str, str], roots: dict[str, Path]) -> list[EvidenceRef]:
    command = row["command"]
    owner = row["effective_owner"]
    refs: list[EvidenceRef] = []
    if owner == "go":
        if row["shell_evidence"] != "-" or row["shell_routing_evidence"] != "-":
            raise RunnerError(f"{command}: wrong-owner shell evidence on Go-owned command")
        refs = [parse_ref(command, "go_evidence", raw, roots) for raw in split_refs(row["go_evidence"])]
    elif owner == "shell":
        if row["go_evidence"] != "-":
            raise RunnerError(f"{command}: wrong-owner Go evidence on shell-owned command")
        for lane in ("shell_evidence", "shell_routing_evidence"):
            refs.extend(parse_ref(command, lane, raw, roots) for raw in split_refs(row[lane]))
    else:
        raise RunnerError(f"{command}: unsupported owner for this runner: {owner}")
    if not refs:
        raise RunnerError(f"{command}: command lacks focused evidence")
    return sorted(refs)


def select_rows(
    rows: list[dict[str, str]], commands: list[str], owner: str | None, all_owned: bool
) -> list[dict[str, str]]:
    by_command = {row["command"]: row for row in rows if row["effective_owner"] in OWNERS}
    if len(by_command) != sum(row["effective_owner"] in OWNERS for row in rows):
        raise RunnerError("manifest contains duplicate command rows")
    modes = sum(bool(item) for item in (commands, owner, all_owned))
    if modes != 1:
        raise RunnerError("select exactly one of command list, --owner, or --all")
    if commands:
        unknown = [command for command in commands if command not in by_command]
        if unknown:
            raise RunnerError(f"unknown or unowned command(s): {', '.join(unknown)}")
        if len(commands) != len(set(commands)):
            raise RunnerError("duplicate command selected")
        selected = [by_command[command] for command in commands]
    elif owner:
        selected = [row for row in rows if row["effective_owner"] == owner]
    else:
        selected = [row for row in rows if row["effective_owner"] in OWNERS]
    selected = sorted(selected, key=lambda row: row["command"])
    if not selected:
        raise RunnerError("empty selection is not evidence")
    return selected


def run_checked(argv: list[str], cwd: Path, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        argv,
        cwd=str(cwd),
        env=env,
        capture_output=True,
        text=True,
        check=True,
    )


def git_revision(root: Path) -> str:
    return run_checked(["git", "rev-parse", "HEAD"], root).stdout.strip()


def go_version(root: Path) -> str:
    return run_checked(["go", "version"], root).stdout.strip()


def go_env(root: Path) -> dict[str, str]:
    result = run_checked(["go", "env", "GOOS", "GOARCH"], root).stdout.splitlines()
    if len(result) < 2 or not result[0] or not result[1]:
        raise RunnerError("go env did not return GOOS and GOARCH")
    return {"GOOS": result[0], "GOARCH": result[1]}


def grouped_invocations(refs: list[EvidenceRef], roots: dict[str, Path]) -> list[dict[str, Any]]:
    grouped: dict[tuple[str, str], list[str]] = defaultdict(list)
    for ref in refs:
        grouped[(ref.repo, ref.package)].append(ref.test)
    invocations = []
    for (repo, package), tests in sorted(grouped.items()):
        unique_tests = sorted(tests)
        run_expr = "^(?:" + "|".join(re.escape(test) for test in unique_tests) + ")$"
        argv = ["go", "test", "-v", "-run", run_expr, package]
        invocations.append({"repo": repo, "cwd": str(roots[repo]), "argv": argv, "tests": unique_tests})
    return invocations


def build_plan(rows: list[dict[str, str]], roots: dict[str, Path]) -> dict[str, Any]:
    commands: dict[str, Any] = {}
    for row in rows:
        refs = refs_for_row(row, roots)
        commands[row["command"]] = {
            "owner": row["effective_owner"],
            "refs": [ref.__dict__ for ref in refs],
            "invocations": grouped_invocations(refs, roots),
        }
    return commands


def build_contract(
    manifest: Path, selected: dict[str, Any], roots: dict[str, Path], root: Path
) -> tuple[str, dict[str, Any]]:
    used_roots = sorted({invocation["repo"] for command in selected.values() for invocation in command["invocations"]})
    revisions = {repo: git_revision(roots[repo]) for repo in used_roots}
    goversion = go_version(root)
    goenv = go_env(root)
    contract = {
        "schema_version": SCHEMA_VERSION,
        "manifest_sha256": sha256_file(manifest),
        "selection": {
            command: [ref["raw"] for ref in data["refs"]]
            for command, data in sorted(selected.items())
        },
        "git_revisions": revisions,
        "go_version": goversion,
        "go_env": goenv,
        "posixly_correct": "1",
    }
    contract_hash = sha256_bytes(json.dumps(contract, sort_keys=True).encode())
    return contract_hash, contract


def read_ledger(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {"runs": {}}
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError as exc:
        raise RunnerError(f"ledger is not valid JSON: {exc}") from exc
    if not isinstance(data, dict) or not isinstance(data.get("runs"), dict):
        raise RunnerError("ledger has an unsupported structure")
    return data


def current_run(ledger: dict[str, Any], contract_hash: str, contract: dict[str, Any]) -> dict[str, Any]:
    runs = ledger.setdefault("runs", {})
    run = runs.setdefault(
        contract_hash,
        {"schema_version": SCHEMA_VERSION, "contract_hash": contract_hash, "contract": contract, "commands": {}},
    )
    run["schema_version"] = SCHEMA_VERSION
    run["contract_hash"] = contract_hash
    run["contract"] = contract
    run.setdefault("commands", {})
    return run


def successful_terminal_attempt(attempts: list[dict[str, Any]]) -> bool:
    if not attempts:
        return False
    last = attempts[-1]
    return (
        last.get("terminal") == "pass"
        and last.get("exit_status") == 0
        and bool(last.get("end_timestamp"))
        and bool(last.get("invocations"))
    )


def attempt_status(attempts: list[dict[str, Any]]) -> str:
    if not attempts:
        return "new"
    last = attempts[-1]
    if last.get("terminal") in {"pass", "fail"} and last.get("end_timestamp"):
        return last["terminal"]
    return "interrupted"


def run_command(
    state_dir: Path,
    command: str,
    invocations: list[dict[str, Any]],
    run: dict[str, Any],
    ledger: dict[str, Any],
    ledger_path: Path,
    env: dict[str, str],
) -> dict[str, Any]:
    command_record = run["commands"].setdefault(command, {"attempts": []})
    attempts = command_record.setdefault("attempts", [])
    if successful_terminal_attempt(attempts):
        return {"command": command, "status": "skipped", "attempt": len(attempts)}

    attempt_no = len(attempts) + 1
    attempt_dir = state_dir / "outputs" / run["contract_hash"] / command / str(attempt_no)
    attempt: dict[str, Any] = {
        "attempt": attempt_no,
        "start_timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "terminal": "unknown",
        "invocations": [],
    }
    attempts.append(attempt)
    atomic_write_json(ledger_path, ledger)

    terminal = "pass"
    final_exit = 0
    for index, invocation in enumerate(invocations, start=1):
        child_env = env.copy()
        child_env["POSIXLY_CORRECT"] = "1"
        completed = subprocess.run(
            invocation["argv"],
            cwd=invocation["cwd"],
            env=child_env,
            capture_output=True,
            shell=False,
        )
        stdout_path = attempt_dir / f"{index:02d}-stdout.bin"
        stderr_path = attempt_dir / f"{index:02d}-stderr.bin"
        atomic_write_bytes(stdout_path, completed.stdout)
        atomic_write_bytes(stderr_path, completed.stderr)
        output_present = bool(completed.stdout or completed.stderr)
        invocation_record = {
            "repo": invocation["repo"],
            "cwd": invocation["cwd"],
            "argv": invocation["argv"],
            "exit_status": completed.returncode,
            "stdout_sha256": sha256_bytes(completed.stdout),
            "stderr_sha256": sha256_bytes(completed.stderr),
            "stdout_path": str(stdout_path),
            "stderr_path": str(stderr_path),
            "output_present": output_present,
        }
        attempt["invocations"].append(invocation_record)
        atomic_write_json(ledger_path, ledger)
        if completed.returncode != 0 or not output_present:
            terminal = "fail"
            final_exit = completed.returncode if completed.returncode != 0 else 1
            break

    attempt["end_timestamp"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    attempt["exit_status"] = final_exit
    attempt["terminal"] = terminal
    attempt["argv"] = [invocation["argv"] for invocation in invocations]
    atomic_write_json(ledger_path, ledger)
    return {"command": command, "status": terminal, "attempt": attempt_no, "exit_status": final_exit}


def emit_json(data: Any) -> None:
    print(json.dumps(data, sort_keys=True))


def emit_text(event: dict[str, Any]) -> None:
    status = event["status"]
    if status == "skipped":
        print(f"Skipping {event['command']} (prior success)")
    elif status == "pass":
        print(f"Passed {event['command']} (attempt {event['attempt']})")
    elif status == "fail":
        print(f"Failed {event['command']} (attempt {event['attempt']})")
    else:
        print(f"{event['command']}: {status}")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("commands", nargs="*", help="owned command names to run")
    parser.add_argument("--manifest", type=Path, default=MANIFEST)
    parser.add_argument("--state-dir", type=Path, required=True)
    parser.add_argument("--owner", choices=sorted(OWNERS))
    parser.add_argument("--all", action="store_true", dest="all_owned")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--json", action="store_true", dest="json_output")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    env = os.environ.copy()
    try:
        manifest = args.manifest.resolve()
        root = manifest.parents[1] if manifest.name == MANIFEST.name and manifest.parent.name == "docs" else ROOT
        roots = configured_roots(root, env)
        rows = read_manifest(manifest)
        selected_rows = select_rows(rows, args.commands, args.owner, args.all_owned)
        selected = build_plan(selected_rows, roots)
        contract_hash, contract = build_contract(manifest, selected, roots, root)
        state_dir = resolve_state_dir(args.state_dir, root, create=not args.dry_run)
        dry_run = {
            "contract_hash": contract_hash,
            "contract": contract,
            "commands": selected,
            "dry_run": True,
            "goos_goarch": f"{contract['go_env']['GOOS']}/{contract['go_env']['GOARCH']}",
            "python_platform": platform.platform(),
        }
        if args.dry_run:
            if args.json_output:
                emit_json(dry_run)
            else:
                for command, data in selected.items():
                    for invocation in data["invocations"]:
                        print(command + ": " + " ".join(invocation["argv"]))
            return 0

        lock_path = state_dir / LOCK_NAME
        ledger_path = state_dir / LEDGER_NAME
        events: list[dict[str, Any]] = []
        with lock_path.open("a+") as lock:
            if fcntl is None:
                raise RunnerError("fcntl locking is unavailable on this platform")
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
            ledger = read_ledger(ledger_path)
            run = current_run(ledger, contract_hash, contract)
            for command, data in selected.items():
                prior = attempt_status(run["commands"].get(command, {}).get("attempts", []))
                event = run_command(
                    state_dir, command, data["invocations"], run, ledger, ledger_path, env
                )
                if prior == "interrupted" and event["status"] != "skipped":
                    event["recovered_from"] = "interrupted"
                events.append(event)
                if not args.json_output:
                    emit_text(event)
            atomic_write_json(ledger_path, ledger)
            fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
        failed = [event for event in events if event["status"] != "skipped" and event["status"] != "pass"]
        if args.json_output:
            emit_json({"contract_hash": contract_hash, "events": events, "ok": not failed})
        return 1 if failed else 0
    except (OSError, subprocess.CalledProcessError, RunnerError) as exc:
        if args.json_output:
            emit_json({"ok": False, "error": str(exc)})
        else:
            print(f"posix-interface-runner: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
