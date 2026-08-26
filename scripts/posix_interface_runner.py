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
import selectors
import shutil
import signal
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
SCHEMA_VERSION = "posix-interface-runner-v2"
DEFAULT_TIMEOUT_SECONDS = 300.0
DEFAULT_MAX_OUTPUT_BYTES = 16 * 1024 * 1024
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
        directory_fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
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


def resolve_state_dir(path: Path, roots: dict[str, Path], *, create: bool = True) -> Path:
    state_dir = path.expanduser().resolve()
    for name, root in roots.items():
        if inside(state_dir, root.resolve()):
            raise RunnerError(f"state directory must be outside every evidence root (inside {name})")
    if create:
        state_dir.mkdir(parents=True, exist_ok=True)
    return state_dir


def configured_roots(root: Path, env: dict[str, str]) -> dict[str, Path]:
    roots = {"coreutils": root.resolve()}
    if env.get("POSIX_SH_EVIDENCE_ROOT"):
        roots["sh"] = Path(env["POSIX_SH_EVIDENCE_ROOT"]).expanduser().resolve()
    if env.get("POSIX_BASHY_EVIDENCE_ROOT"):
        roots["bashy"] = Path(env["POSIX_BASHY_EVIDENCE_ROOT"]).expanduser().resolve()
    inverse: dict[Path, str] = {}
    for name, resolved in roots.items():
        if resolved in inverse:
            raise RunnerError(f"evidence roots {inverse[resolved]!r} and {name!r} resolve to the same path")
        inverse[resolved] = name
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
        bashy_sh_entrypoint = command == "sh" and repo == "bashy"
        if lane == "shell_evidence" and repo != "sh" and not bashy_sh_entrypoint:
            raise RunnerError(f"{command}: shell evidence must use sh root: {raw}")
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
            lane_refs = [parse_ref(command, lane, raw, roots) for raw in split_refs(row[lane])]
            if not lane_refs:
                raise RunnerError(f"{command}: shell-owned command lacks {lane}")
            refs.extend(lane_refs)
    else:
        raise RunnerError(f"{command}: unsupported owner for this runner: {owner}")
    if not refs:
        raise RunnerError(f"{command}: command lacks focused evidence")
    canonical = [(ref.repo, ref.path, ref.test) for ref in refs]
    duplicates = sorted({item for item in canonical if canonical.count(item) > 1})
    if duplicates:
        rendered = ", ".join(f"{repo}:{path}#{test}" for repo, path, test in duplicates)
        raise RunnerError(f"{command}: duplicate evidence reference(s) across lanes: {rendered}")
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


def git_root(root: Path) -> Path:
    resolved = Path(run_checked(["git", "rev-parse", "--show-toplevel"], root).stdout.strip()).resolve()
    if resolved != root.resolve():
        raise RunnerError(f"configured evidence root is not the Git top level: {root}")
    return resolved


def git_status(root: Path) -> str:
    return run_checked(["git", "status", "--porcelain=v1", "--untracked-files=all"], root).stdout


def git_dirty_fingerprint(root: Path, status: str) -> str:
    diff = run_checked(["git", "diff", "--no-ext-diff", "--binary", "HEAD", "--"], root).stdout
    untracked_output = run_checked(
        ["git", "ls-files", "--others", "--exclude-standard"], root
    ).stdout
    digest = hashlib.sha256()
    digest.update(status.encode())
    digest.update(diff.encode())
    for relative in sorted(filter(None, untracked_output.splitlines())):
        relative_path = Path(relative)
        if relative_path.is_absolute() or ".." in relative_path.parts:
            raise RunnerError(f"untracked path escapes evidence root: {relative}")
        path = root / relative_path
        digest.update(relative.encode())
        if path.is_symlink():
            digest.update(b"symlink\0" + os.readlink(path).encode())
        elif path.is_file():
            digest.update(sha256_file(path).encode())
        else:
            raise RunnerError(f"unsupported untracked evidence-root entry: {relative}")
    return digest.hexdigest()


def resolve_go(env: dict[str, str]) -> Path:
    found = shutil.which("go", path=env.get("PATH"))
    if not found:
        raise RunnerError("go executable was not found on PATH")
    resolved = Path(found).resolve()
    if not resolved.is_file():
        raise RunnerError(f"resolved go executable is not a file: {resolved}")
    return resolved


def go_version(root: Path, go_binary: Path) -> str:
    return run_checked([str(go_binary), "version"], root).stdout.strip()


def go_env(root: Path, go_binary: Path) -> dict[str, str]:
    result = run_checked([str(go_binary), "env", "GOOS", "GOARCH"], root).stdout.splitlines()
    if len(result) < 2 or not result[0] or not result[1]:
        raise RunnerError("go env did not return GOOS and GOARCH")
    return {"GOOS": result[0], "GOARCH": result[1]}


def grouped_invocations(
    refs: list[EvidenceRef], roots: dict[str, Path], go_binary: Path
) -> list[dict[str, Any]]:
    grouped: dict[tuple[str, str], list[str]] = defaultdict(list)
    for ref in refs:
        grouped[(ref.repo, ref.package)].append(ref.test)
    invocations = []
    for (repo, package), tests in sorted(grouped.items()):
        unique_tests = sorted(tests)
        run_expr = "^(?:" + "|".join(re.escape(test) for test in unique_tests) + ")$"
        argv = [str(go_binary), "test", "-count=1", "-json", "-run", run_expr, package]
        invocations.append({"repo": repo, "cwd": str(roots[repo]), "argv": argv, "tests": unique_tests})
    return invocations


def build_plan(
    rows: list[dict[str, str]], roots: dict[str, Path], go_binary: Path
) -> dict[str, Any]:
    commands: dict[str, Any] = {}
    for row in rows:
        refs = refs_for_row(row, roots)
        commands[row["command"]] = {
            "owner": row["effective_owner"],
            "refs": [ref.__dict__ for ref in refs],
            "invocations": grouped_invocations(refs, roots, go_binary),
        }
    return commands


def build_contract(
    manifest: Path,
    selected: dict[str, Any],
    roots: dict[str, Path],
    root: Path,
    go_binary: Path,
    timeout_seconds: float,
    max_output_bytes: int,
) -> tuple[str, dict[str, Any]]:
    used_roots = sorted({invocation["repo"] for command in selected.values() for invocation in command["invocations"]})
    root_contract: dict[str, Any] = {}
    for repo in used_roots:
        resolved = git_root(roots[repo])
        status = git_status(resolved)
        root_contract[repo] = {
            "path": str(resolved),
            "git_revision": git_revision(resolved),
            "dirty": bool(status),
            "git_status_sha256": sha256_bytes(status.encode()),
            "dirty_content_sha256": git_dirty_fingerprint(resolved, status),
        }
    evidence_files = {
        f"{ref['repo']}:{ref['path']}": sha256_file(roots[ref["repo"]] / ref["path"])
        for data in selected.values()
        for ref in data["refs"]
    }
    goversion = go_version(root, go_binary)
    goenv = go_env(root, go_binary)
    contract = {
        "schema_version": SCHEMA_VERSION,
        "manifest_sha256": sha256_file(manifest),
        "selection": {
            command: [ref["raw"] for ref in data["refs"]]
            for command, data in sorted(selected.items())
        },
        "evidence_roots": root_contract,
        "evidence_file_sha256": dict(sorted(evidence_files.items())),
        "go": {
            "path": str(go_binary),
            "sha256": sha256_file(go_binary),
            "version": goversion,
            "env": goenv,
        },
        "limits": {
            "timeout_seconds": timeout_seconds,
            "max_output_bytes": max_output_bytes,
        },
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
    if contract_hash not in runs:
        runs[contract_hash] = {
            "schema_version": SCHEMA_VERSION,
            "contract_hash": contract_hash,
            "contract": contract,
            "commands": {},
        }
    run = runs[contract_hash]
    if (
        not isinstance(run, dict)
        or run.get("schema_version") != SCHEMA_VERSION
        or run.get("contract_hash") != contract_hash
        or run.get("contract") != contract
        or not isinstance(run.get("commands"), dict)
    ):
        raise RunnerError(f"ledger contract record is invalid for {contract_hash}")
    return run


def validate_attempts(attempts: Any) -> list[dict[str, Any]]:
    if not isinstance(attempts, list):
        raise RunnerError("ledger command attempts are not a list")
    for number, attempt in enumerate(attempts, start=1):
        if not isinstance(attempt, dict) or attempt.get("attempt") != number:
            raise RunnerError("ledger command attempts are not an append-only sequence")
    return attempts


def expected_artifact_path(
    state_dir: Path, contract_hash: str, command: str, attempt_no: int, index: int, stream: str
) -> Path:
    return state_dir / "outputs" / contract_hash / command / str(attempt_no) / f"{index:02d}-{stream}.bin"


def secure_attempt_dir(state_dir: Path, attempt_dir: Path) -> None:
    state = state_dir.resolve()
    if not inside(attempt_dir, state):
        raise RunnerError("attempt artifact directory escapes the state directory")
    relative = attempt_dir.relative_to(state)
    current = state
    for part in relative.parts:
        current = current / part
        if current.exists() or current.is_symlink():
            if current.is_symlink() or not current.is_dir():
                raise RunnerError(f"unsafe artifact directory component: {current}")
        else:
            current.mkdir()


def terminate_process_group(process: subprocess.Popen[bytes]) -> None:
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=0.5)
    except subprocess.TimeoutExpired:
        pass
    # The group can retain descendants after the direct child exits.
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=1)
    except subprocess.TimeoutExpired as exc:
        raise RunnerError("evidence process did not terminate after SIGKILL") from exc


def run_bounded(
    argv: list[str], cwd: str, env: dict[str, str], timeout_seconds: float, max_output_bytes: int
) -> dict[str, Any]:
    process = subprocess.Popen(
        argv,
        cwd=cwd,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        shell=False,
        start_new_session=True,
    )
    assert process.stdout is not None and process.stderr is not None
    os.set_blocking(process.stdout.fileno(), False)
    os.set_blocking(process.stderr.fileno(), False)
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ, "stdout")
    selector.register(process.stderr, selectors.EVENT_READ, "stderr")
    chunks: dict[str, list[bytes]] = {"stdout": [], "stderr": []}
    captured = 0
    deadline = time.monotonic() + timeout_seconds
    failure: str | None = None
    try:
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0 and failure is None:
                failure = "timeout"
                terminate_process_group(process)
            events = selector.select(max(0.0, min(0.1, remaining)) if failure is None else 0.1)
            for key, _ in events:
                try:
                    data = os.read(key.fd, 64 * 1024)
                except BlockingIOError:
                    continue
                if not data:
                    selector.unregister(key.fileobj)
                    continue
                room = max_output_bytes - captured
                if room > 0:
                    kept = data[:room]
                    chunks[key.data].append(kept)
                    captured += len(kept)
                if len(data) > room and failure is None:
                    failure = "output_limit"
                    terminate_process_group(process)
            if process.poll() is not None and not events:
                # Pipes normally become readable at EOF; this guards unusual selector behavior.
                for key in list(selector.get_map().values()):
                    try:
                        data = os.read(key.fd, 64 * 1024)
                    except BlockingIOError:
                        continue
                    if data:
                        room = max_output_bytes - captured
                        if room > 0:
                            kept = data[:room]
                            chunks[key.data].append(kept)
                            captured += len(kept)
                        if len(data) > room and failure is None:
                            failure = "output_limit"
                    else:
                        selector.unregister(key.fileobj)
        if process.poll() is None:
            process.wait(timeout=max(0.1, deadline - time.monotonic()))
    finally:
        selector.close()
        process.stdout.close()
        process.stderr.close()
        if process.poll() is None:
            terminate_process_group(process)
    return {
        "returncode": process.returncode,
        "stdout": b"".join(chunks["stdout"]),
        "stderr": b"".join(chunks["stderr"]),
        "failure": failure,
        "captured_bytes": captured,
    }


def prove_test_ids(stdout: bytes, planned_tests: list[str]) -> tuple[bool, dict[str, str], str | None]:
    states = {test: "missing" for test in planned_tests}
    try:
        lines = stdout.decode("utf-8").splitlines()
    except UnicodeDecodeError:
        return False, states, "go test -json emitted non-UTF-8 output"
    for line in lines:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            return False, states, "go test -json emitted malformed JSON"
        if not isinstance(event, dict):
            return False, states, "go test -json emitted a non-object event"
        test = event.get("Test")
        action = event.get("Action")
        if test not in states:
            continue
        if action == "run":
            states[test] = "run"
        elif action in {"skip", "fail"}:
            states[test] = action
        elif action == "pass" and states[test] == "run":
            states[test] = "pass"
    bad = {test: state for test, state in states.items() if state != "pass"}
    if bad:
        detail = ", ".join(f"{test}={state}" for test, state in sorted(bad.items()))
        return False, states, f"missing exact TestID pass proof: {detail}"
    return True, states, None


def successful_terminal_attempt(
    attempts: list[dict[str, Any]],
    planned_invocations: list[dict[str, Any]],
    state_dir: Path,
    contract_hash: str,
    command: str,
) -> bool:
    if not attempts:
        return False
    last = attempts[-1]
    records = last.get("invocations")
    if (
        last.get("terminal") != "pass"
        or last.get("exit_status") != 0
        or not last.get("end_timestamp")
        or not isinstance(records, list)
        or len(records) != len(planned_invocations)
    ):
        return False
    attempt_no = last.get("attempt")
    for index, (record, planned) in enumerate(zip(records, planned_invocations), start=1):
        if (
            not isinstance(record, dict)
            or record.get("repo") != planned["repo"]
            or record.get("cwd") != planned["cwd"]
            or record.get("argv") != planned["argv"]
            or record.get("exit_status") != 0
            or record.get("output_present") is not True
            or record.get("test_results") != {test: "pass" for test in planned["tests"]}
        ):
            return False
        for stream in ("stdout", "stderr"):
            expected = record.get(f"{stream}_sha256")
            output_path = record.get(f"{stream}_path")
            if not isinstance(expected, str) or not re.fullmatch(r"[0-9a-f]{64}", expected):
                return False
            if not isinstance(output_path, str):
                return False
            expected_path = expected_artifact_path(
                state_dir, contract_hash, command, attempt_no, index, stream
            )
            try:
                if Path(output_path).resolve(strict=True) != expected_path.resolve(strict=True):
                    return False
                if not inside(expected_path.resolve(strict=True), state_dir.resolve()):
                    return False
            except OSError:
                return False
            try:
                if sha256_file(Path(output_path)) != expected:
                    return False
            except OSError:
                return False
    return True


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
    timeout_seconds: float,
    max_output_bytes: int,
) -> dict[str, Any]:
    command_record = run["commands"].setdefault(command, {"attempts": []})
    if not isinstance(command_record, dict):
        raise RunnerError(f"ledger command record is invalid for {command}")
    attempts = validate_attempts(command_record.setdefault("attempts", []))
    if successful_terminal_attempt(
        attempts, invocations, state_dir, run["contract_hash"], command
    ):
        return {"command": command, "status": "skipped", "attempt": len(attempts)}

    attempt_no = len(attempts) + 1
    attempt_dir = state_dir / "outputs" / run["contract_hash"] / command / str(attempt_no)
    if attempt_dir.exists() or attempt_dir.is_symlink():
        raise RunnerError(f"refusing to overwrite existing attempt artifacts: {attempt_dir}")
    secure_attempt_dir(state_dir, attempt_dir)
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
        go_contract = run["contract"].get("go", {})
        invocation_go = Path(invocation["argv"][0])
        if (
            str(invocation_go.resolve()) != go_contract.get("path")
            or sha256_file(invocation_go) != go_contract.get("sha256")
        ):
            raise RunnerError("Go executable changed after the evidence contract was built")
        completed = run_bounded(
            invocation["argv"],
            invocation["cwd"],
            child_env,
            timeout_seconds,
            max_output_bytes,
        )
        stdout = completed["stdout"]
        stderr = completed["stderr"]
        stdout_path = expected_artifact_path(
            state_dir, run["contract_hash"], command, attempt_no, index, "stdout"
        )
        stderr_path = expected_artifact_path(
            state_dir, run["contract_hash"], command, attempt_no, index, "stderr"
        )
        atomic_write_bytes(stdout_path, stdout)
        atomic_write_bytes(stderr_path, stderr)
        output_present = bool(stdout or stderr)
        proof, test_results, proof_error = prove_test_ids(stdout, invocation["tests"])
        invocation_record = {
            "repo": invocation["repo"],
            "cwd": invocation["cwd"],
            "argv": invocation["argv"],
            "exit_status": completed["returncode"],
            "stdout_sha256": sha256_bytes(stdout),
            "stderr_sha256": sha256_bytes(stderr),
            "stdout_path": str(stdout_path),
            "stderr_path": str(stderr_path),
            "output_present": output_present,
            "captured_bytes": completed["captured_bytes"],
            "failure_reason": completed["failure"] or proof_error,
            "test_results": test_results,
        }
        attempt["invocations"].append(invocation_record)
        atomic_write_json(ledger_path, ledger)
        if completed["returncode"] != 0 or completed["failure"] is not None or not proof:
            terminal = "fail"
            final_exit = completed["returncode"] if completed["returncode"] != 0 else 1
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
    parser.add_argument("--timeout-seconds", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    parser.add_argument("--max-output-bytes", type=int, default=DEFAULT_MAX_OUTPUT_BYTES)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    env = os.environ.copy()
    try:
        if args.timeout_seconds <= 0:
            raise RunnerError("--timeout-seconds must be positive")
        if args.max_output_bytes <= 0:
            raise RunnerError("--max-output-bytes must be positive")
        manifest = args.manifest.resolve()
        root = manifest.parents[1] if manifest.name == MANIFEST.name and manifest.parent.name == "docs" else ROOT
        roots = configured_roots(root, env)
        rows = read_manifest(manifest)
        selected_rows = select_rows(rows, args.commands, args.owner, args.all_owned)
        go_binary = resolve_go(env)
        selected = build_plan(selected_rows, roots, go_binary)
        contract_hash, contract = build_contract(
            manifest,
            selected,
            roots,
            root,
            go_binary,
            args.timeout_seconds,
            args.max_output_bytes,
        )
        state_dir = resolve_state_dir(args.state_dir, roots, create=not args.dry_run)
        dry_run = {
            "contract_hash": contract_hash,
            "contract": contract,
            "commands": selected,
            "dry_run": True,
            "goos_goarch": f"{contract['go']['env']['GOOS']}/{contract['go']['env']['GOARCH']}",
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
                prior_record = run["commands"].get(command, {})
                if not isinstance(prior_record, dict):
                    raise RunnerError(f"ledger command record is invalid for {command}")
                prior = attempt_status(validate_attempts(prior_record.get("attempts", [])))
                event = run_command(
                    state_dir,
                    command,
                    data["invocations"],
                    run,
                    ledger,
                    ledger_path,
                    env,
                    args.timeout_seconds,
                    args.max_output_bytes,
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
