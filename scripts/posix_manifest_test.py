#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("posix_manifest.py")
SPEC = importlib.util.spec_from_file_location("posix_manifest", SCRIPT)
assert SPEC and SPEC.loader
manifest = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(manifest)


class ManifestValidationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.rows = manifest.read_manifest()
        cls.providers = manifest._provider_names()
        cls.packages = manifest._go_packages()
        cls.flagsets = manifest._flagset_packages()

    def changed(self, command: str, **changes: str) -> list[dict[str, str]]:
        rows = copy.deepcopy(self.rows)
        next(item for item in rows if item["command"] == command).update(changes)
        return rows

    def assertRejected(self, rows: list[dict[str, str]], message: str) -> None:
        with self.assertRaisesRegex(manifest.ManifestError, message):
            manifest.validate(rows, self.providers, self.packages, self.flagsets)

    def test_canonical_manifest(self) -> None:
        manifest.validate(self.rows, self.providers, self.packages, self.flagsets)

    def test_denominator_drift_fails(self) -> None:
        self.assertRejected(self.rows[:-1], "denominator drifted")

    def test_owner_drift_fails(self) -> None:
        self.assertRejected(self.changed("echo", profile_cd_disposition="go_applet"), "owner drift")

    def test_duplicate_option_fails(self) -> None:
        self.assertRejected(self.changed("pwd", required_options="-L;-P;-L"), "duplicate option")

    def test_malformed_option_fails(self) -> None:
        self.assertRejected(self.changed("pwd", required_options="-L;--physical"), "malformed option")

    def test_orphan_option_argument_fails(self) -> None:
        self.assertRejected(self.changed("head", option_arguments="-x=<number>"), "undeclared option")

    def test_parser_model_drift_fails(self) -> None:
        self.assertRejected(self.changed("xargs", parser_model="flagset"), "parser model drift")

    def test_missing_clause_fails(self) -> None:
        self.assertRejected(self.changed("true", clause_ids="XCU:true:SYNOPSIS"), "missing clause ID")

    def test_missing_evidence_fails(self) -> None:
        self.assertRejected(
            self.changed("true", evidence_ids="POSIX08-2016:https://example.invalid/true"),
            "missing POSIX08-2016 or repository evidence ID",
        )

    def test_manual_parser_is_explicit(self) -> None:
        manual = {row["command"] for row in self.rows if row["parser_model"] in {"manual", "custom"}}
        self.assertTrue({"awk", "dd", "find", "sed", "stty"} <= manual)
        test = next(row for row in self.rows if row["command"] == "test")
        self.assertEqual(test["parser_model"], "shell_builtin")


if __name__ == "__main__":
    unittest.main()
