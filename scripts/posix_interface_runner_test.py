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
            "import pathlib, sys\n"
            "if sys.argv[1:] == ['rev-parse', 'HEAD']:\n"
            "    print('rev-' + pathlib.Path.cwd().name)\n"
            "    raise SystemExit(0)\n"
            "raise SystemExit(1)\n"
        )
        git.chmod(0o755)
        go = self.bin / "go"
        go.write_text(
            "#!/usr/bin/env python3\n"
            "import json, os, pathlib, sys\n"
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
            "    if os.environ.get('RUNNER_EMPTY') == '1':\n"
            "        raise SystemExit(0)\n"
            "    print('stdout:' + ' '.join(sys.argv[1:]))\n"
            "    print('stderr:' + str(pathlib.Path.cwd()), file=sys.stderr)\n"
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
        self.assertEqual(events[0]["argv"], ["test", "-v", "-run", "^(?:TestAtOne|TestAtTwo)$", "./cmds/at"])

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
        result = self.run_runner("--dry-run", "--json", "at")
        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertTrue(payload["dry_run"])
        self.assertEqual(self.go_events(), [])
        self.assertFalse((self.state / "posix-interface-runner-ledger.json").exists())

    def test_successful_resume_skips_same_contract(self) -> None:
        self.add_test(self.repo, "cmds/at/at_test.go", "TestAtOne")
        self.write_manifest(
            [{"command": "at", "effective_owner": "go", "go_evidence": "cmds/at/at_test.go#TestAtOne"}]
        )
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(self.run_runner("at").returncode, 0)
        self.assertEqual(len(self.go_events()), 1)

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
                    "schema_version": "posix-interface-runner-v1",
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
        self.assertIn("state directory must be outside the worktree", inside_result.stderr)

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

    def test_lock_atomic_posixly_correct_and_output_hashes(self) -> None:
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


if __name__ == "__main__":
    unittest.main()
