#!/usr/bin/env python3

from __future__ import annotations

import csv
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("posix_interface_runner.py").resolve()
FIELDS = (
    "command",
    "effective_owner",
    "go_evidence",
    "shell_evidence",
    "shell_routing_evidence",
)


class PosixInterfaceRunnerTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp)
        self.repo = self.tmp / "coreutils"
        self.sh_repo = self.tmp / "sh"
        self.bashy_repo = self.tmp / "bashy"
        self.state = self.tmp / "state"
        for root in (self.repo, self.sh_repo, self.bashy_repo):
            (root / "docs").mkdir(parents=True)
        self.manifest = self.repo / "docs" / "posix-required-command-interfaces.tsv"
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        self.log = self.tmp / "go-log.jsonl"
        self.write_fake_tools()
        self.env = os.environ.copy()
        self.env.update(
            {
                "PATH": str(self.bin) + os.pathsep + self.env.get("PATH", ""),
                "POSIX_SH_EVIDENCE_ROOT": str(self.sh_repo),
                "POSIX_BASHY_EVIDENCE_ROOT": str(self.bashy_repo),
                "RUNNER_LOG": str(self.log),
            }
        )

    def write_fake_tools(self) -> None:
        git = self.bin / "git"
        git.write_text(
            "#!/usr/bin/env python3\n"
            "import os, pathlib, sys\n"
            "if sys.argv[1:] == ['rev-parse', 'HEAD']:\n"
            "    print('rev-' + pathlib.Path.cwd().name + os.environ.get('RUNNER_REV_SUFFIX', ''))\n"
            "    raise SystemExit(0)\n"
            "if sys.argv[1:] == ['rev-parse', '--show-toplevel']:\n"
            "    print(pathlib.Path.cwd().resolve())\n"
            "    raise SystemExit(0)\n"
            "if sys.argv[1:] == ['status', '--porcelain=v1', '--untracked-files=all']:\n"
            "    print(os.environ.get('RUNNER_DIRTY', ''), end='')\n"
            "    raise SystemExit(0)\n"
            "if sys.argv[1:] == ['diff', '--no-ext-diff', '--binary', 'HEAD', '--']:\n"
            "    print(os.environ.get('RUNNER_DIRTY_CONTENT', ''), end='')\n"
            "    raise SystemExit(0)\n"
            "if sys.argv[1:] == ['ls-files', '--others', '--exclude-standard']:\n"
            "    raise SystemExit(0)\n"
            "raise SystemExit(1)\n"
        )
        git.chmod(0o755)
        go = self.bin / "go"
        go.write_text(
            "#!/usr/bin/env python3\n"
            "import json, os, pathlib, re, subprocess, sys, time\n"
            "if sys.argv[1:] == ['version']:\n"
            "    print('go version go1.26.4 test/arch')\n"
            "    raise SystemExit(0)\n"
            "if sys.argv[1:] == ['env', 'GOOS', 'GOARCH']:\n"
            "    print('testos')\n"
            "    print('testarch')\n"
            "    raise SystemExit(0)\n"
            "if len(sys.argv) > 1 and sys.argv[1] == 'test':\n"
            "    entry = {\n"
            "        'argv': sys.argv[1:],\n"
            "        'cwd': str(pathlib.Path.cwd()),\n"
            "        'posixly_correct': os.environ.get('POSIXLY_CORRECT'),\n"
            "    }\n"
            "    with open(os.environ['RUNNER_LOG'], 'a') as handle:\n"
            "        handle.write(json.dumps(entry, sort_keys=True) + '\\n')\n"
            "    if os.environ.get('RUNNER_HANG_CHILD') == '1':\n"
            "        child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)'])\n"
            "        pathlib.Path(os.environ['RUNNER_CHILD_PID']).write_text(str(child.pid))\n"
            "        time.sleep(60)\n"
            "    if os.environ.get('RUNNER_OUTPUT_BYTES'):\n"
            "        os.write(1, b'x' * int(os.environ['RUNNER_OUTPUT_BYTES']))\n"
            "        time.sleep(60)\n"
            "    time.sleep(float(os.environ.get('RUNNER_SLEEP', '0')))\n"
            "    if os.environ.get('RUNNER_EMPTY') == '1':\n"
            "        raise SystemExit(0)\n"
            "    tests = re.findall(r'Test[A-Za-z0-9_]+', sys.argv[sys.argv.index('-run') + 1])\n"
            "    package = sys.argv[-1]\n"
            "    if os.environ.get('RUNNER_PACKAGE_ONLY') == '1':\n"
            "        print(json.dumps({'Action': 'pass', 'Package': package}))\n"
            "        raise SystemExit(0)\n"
            "    if os.environ.get('RUNNER_ZERO_TEST') == '1':\n"
            "        print(json.dumps({'Action': 'output', 'Package': package, 'Output': 'testing: warning: no tests to run\\n'}))\n"
            "        print(json.dumps({'Action': 'pass', 'Package': package}))\n"
            "        raise SystemExit(0)\n"
            "    for test in tests:\n"
            "        print(json.dumps({'Action': 'run', 'Package': package, 'Test': test}))\n"
            "        action = 'skip' if os.environ.get('RUNNER_SKIP') == '1' else ('fail' if os.environ.get('RUNNER_FAIL') == '1' else 'pass')\n"
            "        print(json.dumps({'Action': action, 'Package': package, 'Test': test}))\n"
            "    raise SystemExit(1 if os.environ.get('RUNNER_FAIL') == '1' else 0)\n"
            "raise SystemExit(1)\n"
        )
        go.chmod(0o755)

    def add_test(self, root: Path, relative: str, *tests: str) -> None:
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("package p\n\n" + "\n".join(f"func {test}() {{}}\n" for test in tests))

    def write_manifest(self, rows: list[dict[str, str]]) -> None:
        with self.manifest.open("w", newline="") as handle:
            writer = csv.DictWriter(handle, fieldnames=FIELDS, delimiter="\t")
            writer.writeheader()
            for row in rows:
                complete = {field: "-" for field in FIELDS}
                complete.update(row)
                writer.writerow(complete)

    def run_runner(
        self,
        *args: str,
        env: dict[str, str] | None = None,
        check: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        command = [
            sys.executable,
            str(SCRIPT),
            "--manifest",
            str(self.manifest),
            "--state-dir",
            str(self.state),
            *args,
        ]
        return subprocess.run(
            command,
            cwd=self.repo,
            env=self.env if env is None else env,
            capture_output=True,
            text=True,
            check=check,
        )

    def go_events(self) -> list[dict[str, str]]:
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def ledger(self) -> dict[str, object]:
        return json.loads((self.state / "posix-interface-runner-ledger.json").read_text())

    def test_exact_selection_and_anchored_package_invocation(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne", "TestAtTwo")
        self.add_test(self.repo, "cmds/cat/cat_test.go", "TestCatOne")
        self.write_manifest(
            [
                {
                    "command": "at",
                    "effective_owner": "go",
                    "go_evidence": "cmds/at/at_test.go#TestAtTwo;cmds/at/at_test.go#TestAtOne",
                },
                {
                    "command": "cat",
                    "effective_owner": "go",
                    "go_evidence": "cmds/cat/cat_test.go#TestCatOne",
                },
            ]
        )
        result = self.run_runner("at")
        self.assertEqual(result.returncode, 0, result.stderr)
        events = self.go_events()
        self.assertEqual(len(events), 1)
        self.assertEqual(events[0]["cwd"], str(self.repo.resolve()))
        self.assertEqual(
            events[0]["argv"],
            ["test", "-count=1", "-json", "-run", "^(?:TestAtOne|TestAtTwo)$", "./cmds/at"],
        )

    def test_owner_routing_uses_shell_and_bashy_roots(self) -> None:
        self.add_test(self.sh_repo, "interp/issue7_test.go", "TestEchoIssue7Interface")
        self.add_test(self.bashy_repo, "internal/cli/profile_b_test.go", "TestProfileBRouteEcho")
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [
                {
                    "command": "at",
                    "effective_owner": "go",
                    "go_evidence": "cmds/at/at_test.go#TestAtOne",
                },
                {
                    "command": "echo",
                    "effective_owner": "shell",
                    "shell_evidence": "sh:interp/issue7_test.go#TestEchoIssue7Interface",
                    "shell_routing_evidence": "bashy:internal/cli/profile_b_test.go#TestProfileBRouteEcho",
                },
            ]
        )
        result = self.run_runner("--owner", "shell")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            sorted(event["cwd"] for event in self.go_events()),
            sorted([str(self.sh_repo.resolve()), str(self.bashy_repo.resolve())]),
        )

    def test_dry_run_json_does_not_execute_or_write_ledger(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.state.mkdir()
        sentinel = self.state / "sentinel"
        sentinel.write_bytes(b"unchanged")
        before = (sentinel.read_bytes(), sentinel.stat().st_mtime_ns, sorted(self.state.iterdir()))
        result = self.run_runner("--dry-run", "--json", "at")
        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertTrue(payload["dry_run"])
        self.assertEqual(self.go_events(), [])
        self.assertFalse((self.state / "posix-interface-runner-ledger.json").exists())
        after = (sentinel.read_bytes(), sentinel.stat().st_mtime_ns, sorted(self.state.iterdir()))
        self.assertEqual(after, before)

    def test_successful_resume_skips_same_contract(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(len(self.go_events()), 1)

    def test_missing_saved_output_invalidates_success_and_reruns(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        first = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"][0]
        Path(first["invocations"][0]["stdout_path"]).unlink()
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(len(self.go_events()), 2)
        attempts = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"]
        self.assertEqual([attempt["terminal"] for attempt in attempts], ["pass", "pass"])

    def test_manifest_contract_change_reruns_success(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        with self.manifest.open("a") as handle:
            handle.write("# comment that changes the manifest hash\n")
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(len(self.go_events()), 2)

    def test_git_revision_contract_change_reruns_success(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        changed_env = self.env.copy()
        changed_env["RUNNER_REV_SUFFIX"] = "-changed"
        self.assertEqual(self.run_runner("at", env=changed_env).returncode, 0)
        self.assertEqual(len(self.go_events()), 2)
        revisions = {
            run["contract"]["evidence_roots"]["coreutils"]["git_revision"]
            for run in self.ledger()["runs"].values()
        }
        self.assertEqual(revisions, {"rev-coreutils", "rev-coreutils-changed"})

    def test_failure_is_retained_and_retried(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        failing_env = self.env.copy()
        failing_env["RUNNER_FAIL"] = "1"
        self.assertNotEqual(self.run_runner("at", env=failing_env).returncode, 0)
        self.assertEqual(self.run_runner("at").returncode, 0)
        attempts = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"]
        self.assertEqual([attempt["terminal"] for attempt in attempts], ["fail", "pass"])

    def test_interrupted_unknown_attempt_is_detected_and_rerun(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        dry = json.loads(self.run_runner("--dry-run", "--json", "at").stdout)
        self.state.mkdir(exist_ok=True)
        ledger = {
            "runs": {
                dry["contract_hash"]: {
                    "schema_version": "posix-interface-runner-v2",
                    "contract_hash": dry["contract_hash"],
                    "contract": dry["contract"],
                    "commands": {
                        "at": {"attempts": [{"attempt": 1, "start_timestamp": "2026-01-01T00:00:00Z", "terminal": "unknown"}]}
                    },
                }
            }
        }
        (self.state / "posix-interface-runner-ledger.json").write_text(json.dumps(ledger))
        result = self.run_runner("--json", "at")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["events"][0]["recovered_from"], "interrupted")
        attempts = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"]
        self.assertEqual(len(attempts), 2)

    def test_empty_malformed_non_test_missing_and_duplicate_refs_are_rejected(self) -> None:
        cases = [
            ("empty", "-", "lacks focused evidence"),
            ("malformed", "cmds/x/x_test.go", "malformed"),
            ("nontest", "cmds/x/x_test.go#BenchmarkX", "malformed"),
            ("missingpath", "cmds/x/missing_test.go#TestX", "evidence path is missing"),
            ("missingid", "cmds/x/x_test.go#TestY", "Test ID is missing"),
            ("duplicate", "cmds/x/x_test.go#TestX;cmds/x/x_test.go#TestX", "duplicate evidence"),
        ]
        self.add_test(self.repo, "cmds/x/x_test.go", "TestX")
        for command, ref, message in cases:
            with self.subTest(command=command):
                self.write_manifest([{"command": command, "effective_owner": "go", "go_evidence": ref}])
                result = self.run_runner(command)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(message, result.stderr)

    def test_wrong_owner_and_sibling_root_requirements_are_rejected(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [
                {
                    "command": "echo",
                    "effective_owner": "shell",
                    "go_evidence": "cmds/at/at_test.go#TestAtOne",
                    "shell_evidence": "sh:interp/issue7_test.go#TestEchoIssue7Interface",
                    "shell_routing_evidence": "bashy:internal/cli/profile_b_test.go#TestProfileBRouteEcho",
                }
            ]
        )
        result = self.run_runner("echo")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("wrong-owner", result.stderr)

        env = self.env.copy()
        env.pop("POSIX_SH_EVIDENCE_ROOT")
        self.write_manifest(
            [
                {
                    "command": "echo",
                    "effective_owner": "shell",
                    "shell_evidence": "sh:interp/issue7_test.go#TestEchoIssue7Interface",
                    "shell_routing_evidence": "bashy:internal/cli/profile_b_test.go#TestProfileBRouteEcho",
                }
            ]
        )
        result = self.run_runner("echo", env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unavailable evidence root 'sh'", result.stderr)

        self.add_test(self.bashy_repo, "interp/issue7_test.go", "TestEchoIssue7Interface")
        self.write_manifest(
            [
                {
                    "command": "echo",
                    "effective_owner": "shell",
                    "shell_evidence": "bashy:interp/issue7_test.go#TestEchoIssue7Interface",
                    "shell_routing_evidence": "bashy:internal/cli/profile_b_test.go#TestProfileBRouteEcho",
                }
            ]
        )
        result = self.run_runner("echo")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("shell evidence must use sh root", result.stderr)

    def test_state_dir_must_be_outside_worktree_and_empty_selection_fails(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        inside_result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--manifest",
                str(self.manifest),
                "--state-dir",
                str(self.repo / "state"),
                "at",
            ],
            cwd=self.repo,
            env=self.env,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(inside_result.returncode, 0)
        self.assertIn("state directory must be outside every evidence root", inside_result.stderr)

        self.write_manifest([])
        empty_result = subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--manifest",
                str(self.manifest),
                "--state-dir",
                str(self.repo / "state"),
                "--all",
            ],
            cwd=self.repo,
            env=self.env,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(empty_result.returncode, 0)
        self.assertIn("empty selection", empty_result.stderr)

    def test_lock_serializes_runs_and_intermediate_ledger_is_atomic(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        slow_env = self.env.copy()
        slow_env["RUNNER_SLEEP"] = "0.5"
        argv = [
            sys.executable,
            str(SCRIPT),
            "--manifest",
            str(self.manifest),
            "--state-dir",
            str(self.state),
            "at",
        ]
        first = subprocess.Popen(
            argv,
            cwd=self.repo,
            env=slow_env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        deadline = time.monotonic() + 3
        while not self.log.exists() and first.poll() is None and time.monotonic() < deadline:
            time.sleep(0.01)
        self.assertTrue(self.log.exists(), "first runner did not begin its evidence invocation")
        in_progress = self.ledger()
        attempt = next(iter(in_progress["runs"].values()))["commands"]["at"]["attempts"][0]
        self.assertEqual(attempt["terminal"], "unknown")

        second = self.run_runner("--json", "at")
        first_stdout, first_stderr = first.communicate(timeout=3)
        self.assertEqual(first.returncode, 0, first_stderr or first_stdout)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(json.loads(second.stdout)["events"][0]["status"], "skipped")
        self.assertEqual(len(self.go_events()), 1)
        self.assertEqual(list(self.state.glob("*.tmp")), [])

    def test_posixly_correct_and_output_hashes(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        result = self.run_runner("at")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.state / "posix-interface-runner.lock").exists())
        self.assertEqual(list(self.state.glob("*.tmp")), [])
        self.assertEqual(self.go_events()[0]["posixly_correct"], "1")
        attempt = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"][0]
        invocation = attempt["invocations"][0]
        stdout = Path(invocation["stdout_path"]).read_bytes()
        stderr = Path(invocation["stderr_path"]).read_bytes()
        self.assertEqual(invocation["stdout_sha256"], hashlib.sha256(stdout).hexdigest())
        self.assertEqual(invocation["stderr_sha256"], hashlib.sha256(stderr).hexdigest())
        self.assertTrue(invocation["output_present"])

    def test_absent_output_never_passes(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        env = self.env.copy()
        env["RUNNER_EMPTY"] = "1"
        result = self.run_runner("at", env=env)
        self.assertNotEqual(result.returncode, 0)
        attempt = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"][0]
        self.assertEqual(attempt["terminal"], "fail")

    def test_exact_testid_proof_rejects_zero_skip_and_package_only(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        for variable, expected in (
            ("RUNNER_ZERO_TEST", "missing"),
            ("RUNNER_SKIP", "skip"),
            ("RUNNER_PACKAGE_ONLY", "missing"),
        ):
            with self.subTest(variable=variable):
                env = self.env.copy()
                env[variable] = "1"
                result = self.run_runner("at", env=env)
                self.assertNotEqual(result.returncode, 0)
                attempts = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"]
                invocation = attempts[-1]["invocations"][0]
                self.assertEqual(invocation["test_results"]["TestAtOne"], expected)
                self.assertIn("missing exact TestID pass proof", invocation["failure_reason"])

    def test_timeout_kills_descendant_process_group(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        pid_path = self.tmp / "child.pid"
        env = self.env.copy()
        env.update({"RUNNER_HANG_CHILD": "1", "RUNNER_CHILD_PID": str(pid_path)})
        result = self.run_runner("--timeout-seconds", "0.2", "at", env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(pid_path.exists())
        child_pid = int(pid_path.read_text())
        deadline = time.monotonic() + 3
        alive = True
        while time.monotonic() < deadline:
            try:
                os.kill(child_pid, 0)
            except ProcessLookupError:
                alive = False
                break
            time.sleep(0.02)
        if alive:
            os.kill(child_pid, 9)
        self.assertFalse(alive, "timed-out evidence descendant survived process-group cleanup")
        attempt = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"][0]
        self.assertEqual(attempt["invocations"][0]["failure_reason"], "timeout")

    def test_output_cap_kills_invocation_and_bounds_artifacts(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        env = self.env.copy()
        env["RUNNER_OUTPUT_BYTES"] = "65536"
        result = self.run_runner("--max-output-bytes", "1024", "at", env=env)
        self.assertNotEqual(result.returncode, 0)
        attempt = next(iter(self.ledger()["runs"].values()))["commands"]["at"]["attempts"][0]
        invocation = attempt["invocations"][0]
        self.assertEqual(invocation["failure_reason"], "output_limit")
        self.assertLessEqual(Path(invocation["stdout_path"]).stat().st_size, 1024)
        self.assertLessEqual(
            Path(invocation["stdout_path"]).stat().st_size
            + Path(invocation["stderr_path"]).stat().st_size,
            1024,
        )

    def test_contract_binds_reference_dirty_state_and_go_binary(self) -> None:
        relative = "cmds/at/at_test.go"
        self.add_test(self.repo, relative, "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": relative + "#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        (self.repo / relative).write_text("package p\n\nfunc TestAtOne() {}\n// changed\n")
        self.assertEqual(self.run_runner("at").returncode, 0)
        dirty_env = self.env.copy()
        dirty_env["RUNNER_DIRTY"] = " M unrelated.go\n"
        dirty_env["RUNNER_DIRTY_CONTENT"] = "first dirty contents"
        self.assertEqual(self.run_runner("at", env=dirty_env).returncode, 0)
        changed_dirty_env = dirty_env.copy()
        changed_dirty_env["RUNNER_DIRTY_CONTENT"] = "different dirty contents"
        self.assertEqual(self.run_runner("at", env=changed_dirty_env).returncode, 0)
        with (self.bin / "go").open("a") as handle:
            handle.write("# changed executable bytes\n")
        self.assertEqual(self.run_runner("at", env=changed_dirty_env).returncode, 0)
        second_bin = self.tmp / "bin-second"
        second_bin.mkdir()
        shutil.copy2(self.bin / "go", second_bin / "go")
        path_env = changed_dirty_env.copy()
        path_env["PATH"] = str(second_bin) + os.pathsep + path_env["PATH"]
        self.assertEqual(self.run_runner("at", env=path_env).returncode, 0)
        self.assertEqual(len(self.go_events()), 6)
        contracts = [run["contract"] for run in self.ledger()["runs"].values()]
        self.assertEqual(len(contracts), 6)
        self.assertEqual(
            {contract["evidence_roots"]["coreutils"]["path"] for contract in contracts},
            {str(self.repo.resolve())},
        )
        self.assertEqual(len({contract["go"]["path"] for contract in contracts}), 2)

    def test_state_rejected_under_each_root_and_symlink_alias(self) -> None:
        self.add_test(self.sh_repo, "interp/x_test.go", "TestX")
        self.add_test(self.bashy_repo, "route/x_test.go", "TestRoute")
        self.write_manifest(
            [{
                "command": "x",
                "effective_owner": "shell",
                "shell_evidence": "sh:interp/x_test.go#TestX",
                "shell_routing_evidence": "bashy:route/x_test.go#TestRoute",
            }]
        )
        for root in (self.repo, self.sh_repo, self.bashy_repo):
            with self.subTest(root=root.name):
                self.state = root / "runner-state"
                result = self.run_runner("x")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("outside every evidence root", result.stderr)
        alias = self.tmp / "sh-alias"
        alias.symlink_to(self.sh_repo, target_is_directory=True)
        self.state = alias / "runner-state"
        result = self.run_runner("x")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("outside every evidence root", result.stderr)

    def test_cross_lane_duplicate_is_rejected(self) -> None:
        self.add_test(self.bashy_repo, "route/sh_test.go", "TestRoute")
        ref = "bashy:route/sh_test.go#TestRoute"
        self.write_manifest(
            [{
                "command": "sh",
                "effective_owner": "shell",
                "shell_evidence": ref,
                "shell_routing_evidence": ref,
            }]
        )
        result = self.run_runner("sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("duplicate evidence reference(s) across lanes", result.stderr)

    def test_escaped_and_symlinked_saved_artifacts_never_resume(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        ledger = self.ledger()
        first = next(iter(ledger["runs"].values()))["commands"]["at"]["attempts"][0]
        invocation = first["invocations"][0]
        escaped = self.tmp / "escaped.bin"
        escaped.write_bytes(Path(invocation["stdout_path"]).read_bytes())
        invocation["stdout_path"] = str(escaped)
        (self.state / "posix-interface-runner-ledger.json").write_text(json.dumps(ledger))
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(len(self.go_events()), 2)

        ledger = self.ledger()
        attempts = next(iter(ledger["runs"].values()))["commands"]["at"]["attempts"]
        latest = Path(attempts[-1]["invocations"][0]["stdout_path"])
        saved = self.tmp / "saved.bin"
        saved.write_bytes(latest.read_bytes())
        latest.unlink()
        latest.symlink_to(saved)
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(len(self.go_events()), 3)

    def test_symlinked_attempt_directory_is_rejected_without_escape_write(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        dry = json.loads(self.run_runner("--dry-run", "--json", "at").stdout)
        outside = self.tmp / "outside-artifacts"
        outside.mkdir()
        parent = self.state / "outputs" / dry["contract_hash"] / "at"
        parent.mkdir(parents=True)
        (parent / "1").symlink_to(outside, target_is_directory=True)
        result = self.run_runner("at")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("refusing to overwrite existing attempt artifacts", result.stderr)
        self.assertEqual(list(outside.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
