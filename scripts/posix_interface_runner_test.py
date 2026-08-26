import unittest
import tempfile
import os
import sys
import subprocess
import json
import shutil
import hashlib
from pathlib import Path

class TestPosixInterfaceRunner(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)
        
        self.repo = Path(self.tmp) / "repo"
        self.repo.mkdir()
        
        self.state_dir = Path(self.tmp) / "state"
        
        # Fake git and go
        self.bin_dir = Path(self.tmp) / "bin"
        self.bin_dir.mkdir()
        
        self.git_fake = self.bin_dir / "git"
        with open(self.git_fake, "w") as f:
            f.write("#!/bin/sh\n")
            f.write("if [ \"$1\" = 'rev-parse' ]; then echo 'deadbeef'; exit 0; fi\n")
            f.write("exit 1\n")
        self.git_fake.chmod(0o755)
        
        self.go_fake = self.bin_dir / "go"
        with open(self.go_fake, "w") as f:
            f.write("#!/bin/sh\n")
            f.write("if [ \"$1\" = 'version' ]; then echo 'go version go1.26.4 darwin/amd64'; exit 0; fi\n")
            f.write("if [ \"$1\" = 'env' ]; then echo 'darwin\namd64'; exit 0; fi\n")
            f.write("if [ \"$1\" = 'test' ]; then echo 'test output'; exit 0; fi\n")
            f.write("exit 1\n")
        self.go_fake.chmod(0o755)
        
        self.env = os.environ.copy()
        self.env["PATH"] = str(self.bin_dir) + ":" + self.env.get("PATH", "")
        self.env["POSIX_SH_EVIDENCE_ROOT"] = str(self.repo)
        self.env["POSIX_BASHY_EVIDENCE_ROOT"] = str(self.repo)
        
        self.runner_script = Path(__file__).parent / "posix_interface_runner.py"
        self.docs_dir = self.repo / "docs"
        self.docs_dir.mkdir()
        self.tsv_path = self.docs_dir / "posix-required-command-interfaces.tsv"

    def write_tsv(self, lines):
        with open(self.tsv_path, "w") as f:
            f.write("command\ta\tb\teffective_owner\tc\td\te\tf\tg\th\ti\tj\tk\tl\tm\tn\to\tp\tq\tr\ts\tt\tu\tv\tw\tgo_evidence\tshell_evidence\tshell_routing_evidence\tx\ty\tz\n")
            for l in lines:
                f.write(l + "\n")

    def run_runner(self, args, check=True):
        cmd = [sys.executable, str(self.runner_script)] + args
        return subprocess.run(cmd, env=self.env, cwd=str(self.repo), capture_output=True, text=True, check=check)

    def test_exact_selection(self):
        self.write_tsv([
            "at\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestA\t-\t-\t-\t-\t-",
            "sh\t-\t-\tshell\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tsh:interp/foo_test.go#TestS\tbashy:internal/cli/bar_test.go#TestB\t-\t-\t-"
        ])
        res = self.run_runner(["at", "--state-dir", str(self.state_dir)])
        self.assertIn("Running at (attempt 1)", res.stdout)
        self.assertIn("Passed at", res.stdout)
        self.assertNotIn("Running sh", res.stdout)
        
    def test_owner_routing(self):
        self.write_tsv([
            "at\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestA\t-\t-\t-\t-\t-"
        ])
        res = self.run_runner(["--owner", "go", "--state-dir", str(self.state_dir)])
        self.assertIn("Running at", res.stdout)
        
    def test_successful_resume_skip(self):
        self.write_tsv([
            "at\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestA\t-\t-\t-\t-\t-"
        ])
        self.run_runner(["at", "--state-dir", str(self.state_dir)])
        res = self.run_runner(["at", "--state-dir", str(self.state_dir)])
        self.assertIn("Skipping at (prior success)", res.stdout)
        
    def test_contract_change_rerun(self):
        self.write_tsv([
            "at\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestA\t-\t-\t-\t-\t-"
        ])
        self.run_runner(["at", "--state-dir", str(self.state_dir)])
        
        self.write_tsv([
            "at\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestA\t-\t-\t-\t-\t-",
            "new\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestN\t-\t-\t-\t-\t-"
        ])
        res = self.run_runner(["at", "--state-dir", str(self.state_dir)])
        self.assertIn("Running at (attempt 1)", res.stdout)
        
    def test_failure_retry(self):
        self.write_tsv([
            "at\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tcmds/at/at_test.go#TestA\t-\t-\t-\t-\t-"
        ])
        with open(self.go_fake, "w") as f:
            f.write("#!/bin/sh\n")
            f.write("if [ \"$1\" = 'version' ]; then echo 'go version go1.26.4 darwin/amd64'; exit 0; fi\n")
            f.write("if [ \"$1\" = 'env' ]; then echo 'darwin\namd64'; exit 0; fi\n")
            f.write("if [ \"$1\" = 'test' ]; then exit 1; fi\n")
            f.write("exit 1\n")
            
        res = self.run_runner(["at", "--state-dir", str(self.state_dir)], check=False)
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Failed at", res.stdout)
        
        with open(self.go_fake, "w") as f:
            f.write("#!/bin/sh\n")
            f.write("if [ \"$1\" = 'version' ]; then echo 'go version go1.26.4 darwin/amd64'; exit 0; fi\n")
            f.write("if [ \"$1\" = 'env' ]; then echo 'darwin\namd64'; exit 0; fi\n")
            f.write("if [ \"$1\" = 'test' ]; then echo 'test output'; exit 0; fi\n")
            f.write("exit 1\n")
            
        res = self.run_runner(["at", "--state-dir", str(self.state_dir)])
        self.assertIn("Running at (attempt 2)", res.stdout)
        
    def test_empty_malformed_rejection(self):
        self.write_tsv([
            "malformed\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tbadref\t-\t-\t-\t-\t-",
            "empty\t-\t-\tgo\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-"
        ])
        res = self.run_runner(["malformed", "--state-dir", str(self.state_dir)], check=False)
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Malformed reference", res.stderr)
        
        res = self.run_runner(["empty", "--state-dir", str(self.state_dir)], check=False)
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("missing evidence", res.stderr)

    def test_sibling_root_requirements(self):
        self.write_tsv([
            "sh\t-\t-\tshell\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\t-\tsh:interp/foo_test.go#TestS\tbashy:internal/cli/bar_test.go#TestB\t-\t-\t-"
        ])
        del self.env["POSIX_SH_EVIDENCE_ROOT"]
        res = self.run_runner(["sh", "--state-dir", str(self.state_dir)], check=False)
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Unavailable root for repo: sh", res.stderr)

if __name__ == "__main__":
    unittest.main()
