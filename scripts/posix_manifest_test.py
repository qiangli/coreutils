#!/usr/bin/env python3

from __future__ import annotations

import copy
import importlib.util
import re
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

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

    def changed(self, name: str, **changes: str) -> list[dict[str, str]]:
        rows = copy.deepcopy(self.rows)
        next(item for item in rows if item["command"] == name).update(changes)
        return rows

    def assertRejected(self, rows: list[dict[str, str]], message: str) -> None:
        with self.assertRaisesRegex(manifest.ManifestError, message):
            manifest.validate(rows, self.providers, self.packages, self.flagsets)

    def test_canonical_manifest_and_generated_document(self) -> None:
        manifest.validate(self.rows, self.providers, self.packages, self.flagsets)
        rendered = manifest.render(self.rows)
        manifest.validate_rendered(rendered, self.rows)
        self.assertEqual(manifest.GUIDE.name, "posix-required-command-interfaces.md")
        self.assertEqual(manifest.GUIDE.read_text(), rendered)

    def test_named_document_absent_fails_check(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            absent = Path(directory) / "posix-required-command-interfaces.md"
            with (
                mock.patch.object(manifest, "GUIDE", absent),
                mock.patch.object(sys, "argv", [str(SCRIPT), "--check"]),
                self.assertRaisesRegex(SystemExit, "required generated document is absent"),
            ):
                manifest.main()

    def test_per_command_heading_count_must_be_116(self) -> None:
        rendered = manifest.render(self.rows)
        damaged = rendered.replace("## `alias`", "### `alias`", 1)
        with self.assertRaisesRegex(manifest.ManifestError, "heading count/order drifted"):
            manifest.validate_rendered(damaged, self.rows)
        self.assertEqual(len(re.findall(r"^## `[^`]+`$", rendered, re.MULTILINE)), 116)

    def test_every_rendered_section_field_is_required(self) -> None:
        rendered = manifest.render(self.rows)
        damaged = rendered.replace("**Environment:**", "**Environment omitted:**", 1)
        with self.assertRaisesRegex(manifest.ManifestError, "missing field.*Environment"):
            manifest.validate_rendered(damaged, self.rows)

    def test_every_canonical_field_is_required(self) -> None:
        for field in manifest.FIELDS:
            with self.subTest(field=field):
                self.assertRejected(self.changed("true", **{field: ""}), "missing field")

    def test_denominator_drift_fails(self) -> None:
        self.assertRejected(self.rows[:-1], "denominator drifted")

    def test_availability_axis_drift_fails(self) -> None:
        self.assertRejected(self.changed("echo", availability="shell_only"), "availability drift")

    def test_effective_selection_axis_drift_fails(self) -> None:
        self.assertRejected(self.changed("echo", effective_owner="go"), "owner drift")

    def test_invalid_applicability_fails(self) -> None:
        self.assertRejected(self.changed("true", applicability="gnu"), "absent/invalid applicability")

    def test_conditional_option_cannot_enter_mandatory_set(self) -> None:
        self.assertRejected(self.changed("df", required_options="-k"), "mixed into mandatory")

    def test_duplicate_option_fails(self) -> None:
        self.assertRejected(self.changed("pwd", required_options="-L;-P;-L"), "duplicate mandatory")

    def test_orphan_option_argument_fails(self) -> None:
        self.assertRejected(self.changed("head", option_arguments="-x=<number>"), "undeclared option")

    def test_missing_clause_fails(self) -> None:
        self.assertRejected(self.changed("true", clause_ids="XCU:true:SYNOPSIS"), "missing clause ID")

    def test_wrong_or_nonofficial_link_fails(self) -> None:
        row = next(item for item in self.rows if item["command"] == "true")
        self.assertRejected(
            self.changed("true", evidence_ids=row["evidence_ids"].replace("true.html", "false.html")),
            "missing exact POSIX08-2016",
        )

    def test_malformed_conditional_synopsis_fails(self) -> None:
        self.assertRejected(
            self.changed("df", conditional_synopsis="gnu::df [-k]"),
            "invalid conditional synopsis applicability",
        )


if __name__ == "__main__":
    unittest.main()
