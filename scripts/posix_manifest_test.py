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
        next(row for row in rows if row["command"] == name).update(changes)
        return rows

    def row(self, name: str) -> dict[str, str]:
        return next(row for row in self.rows if row["command"] == name)

    def assertRejected(self, rows: list[dict[str, str]], message: str) -> None:
        with self.assertRaisesRegex(manifest.ManifestError, message):
            manifest.validate(rows, self.providers, self.packages, self.flagsets)

    def test_canonical_manifest_and_generated_document(self) -> None:
        manifest.validate(self.rows, self.providers, self.packages, self.flagsets)
        rendered = manifest.render(self.rows)
        manifest.validate_rendered(rendered, self.rows)
        self.assertEqual(manifest.GUIDE.read_text(), rendered)
        self.assertEqual(manifest.MANIFEST.name, "posix-required-command-interfaces.tsv")

    def test_old_five_column_map_contract_is_unchanged(self) -> None:
        rows = manifest.read_legacy_map()
        self.assertEqual(len(rows), 116)
        self.assertEqual(tuple(rows[0]), manifest.LEGACY_FIELDS)
        self.assertNotIn("base_synopsis", rows[0])
        self.assertNotEqual(manifest.LEGACY_MAP, manifest.MANIFEST)

    def test_old_map_schema_drift_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "old-map.tsv"
            path.write_text("command\tavailability\ntrue\tgo\n")
            with self.assertRaisesRegex(manifest.ManifestError, "five-column"):
                manifest.read_legacy_map(path)

    def test_named_generated_document_absence_fails_check(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            absent = Path(directory) / manifest.GUIDE.name
            with (
                mock.patch.object(manifest, "GUIDE", absent),
                mock.patch.object(sys, "argv", [str(SCRIPT), "--check"]),
                self.assertRaisesRegex(SystemExit, "required generated document is absent"),
            ):
                manifest.main()

    def test_heading_count_and_visible_states(self) -> None:
        rendered = manifest.render(self.rows)
        damaged = rendered.replace("## `alias`", "### `alias`", 1)
        with self.assertRaisesRegex(manifest.ManifestError, "heading count/order"):
            manifest.validate_rendered(damaged, self.rows)
        self.assertEqual(len(re.findall(r"^## `[^`]+`$", rendered, re.MULTILINE)), 116)
        self.assertIn("| Evidence | Verified | 0 |", rendered)
        self.assertIn("| Evidence | Partial | 2 |", rendered)
        self.assertIn("| Evidence | Unverified | 114 |", rendered)

    def test_completion_fails_closed_while_any_row_is_unverified(self) -> None:
        errors = manifest.completion_errors(self.rows)
        self.assertTrue(any(error == "alias: state=unverified" for error in errors))
        with (
            mock.patch.object(sys, "argv", [str(SCRIPT), "--require-complete"]),
            self.assertRaisesRegex(SystemExit, "completion blocked"),
        ):
            manifest.main()

    def test_every_interface_field_is_required(self) -> None:
        for field in manifest.FIELDS:
            with self.subTest(field=field):
                self.assertRejected(self.changed("true", **{field: ""}), "missing field")

    def test_counts_and_selection_axes_are_exact(self) -> None:
        self.assertRejected(self.changed("echo", availability="shell_only"), "availability drift")
        self.assertRejected(self.changed("echo", effective_owner="go"), "owner drift")

    def test_gnu_claims_are_out_of_scope(self) -> None:
        self.assertNotIn("gnu_only_options", manifest.FIELDS)
        self.assertRejected(
            self.changed("true", compatibility_scope="GNU options complete"),
            "GNU compatibility must remain explicitly out of scope",
        )

    def test_fabricated_generic_prose_is_rejected(self) -> None:
        self.assertRejected(
            self.changed("true", stdout="Write the required result format to standard output."),
            "fabricated generic prose",
        )

    def test_absent_or_unfocused_evidence_path_is_rejected(self) -> None:
        self.assertRejected(
            self.changed("pr", go_evidence="cmds/pr/not-present_test.go"),
            "evidence path absent",
        )
        self.assertRejected(
            self.changed("pr", go_evidence="cmds/pr/pr.go"),
            "not a focused test",
        )

    def test_evidence_lanes_cannot_cross(self) -> None:
        self.assertRejected(
            self.changed("xargs", shell_evidence="cmds/xargs/xargs_test.go"),
            "evidence crossed implementation lanes",
        )

    def test_state_laundering_is_rejected(self) -> None:
        self.assertRejected(
            self.changed("true", evidence_state="verified"),
            "verified state launders",
        )
        self.assertRejected(
            self.changed(
                "basename", evidence_state="verified",
                required_options="-Z", go_evidence="cmds/basename/basename_test.go",
            ),
            "verified state launders",
        )

    def test_parser_source_comparison_covers_every_go_selected_row(self) -> None:
        go_rows = [row for row in self.rows if row["effective_owner"] == "go"]
        self.assertEqual(len(go_rows), 78)
        for row in go_rows:
            with self.subTest(command=row["command"]):
                recognized = manifest.recognized_go_options(row)
                gaps = manifest.parser_gaps(row)
                self.assertEqual(manifest.declared_options(row) - recognized, gaps)
                recognized_arguments = manifest.recognized_go_option_arguments(row)
                argument_gaps = manifest.option_argument_gaps(row)
                self.assertEqual(
                    set(manifest.declared_option_arguments(row)) - recognized_arguments,
                    argument_gaps,
                )
                if gaps or argument_gaps:
                    self.assertNotEqual(row["evidence_state"], "verified")

    def test_fabricated_parser_option_is_reported_as_gap(self) -> None:
        row = copy.deepcopy(self.row("basename"))
        row["required_options"] = "-Z"
        self.assertEqual(manifest.parser_gaps(row), {"-Z"})

    def test_fabricated_option_argument_is_reported_as_gap(self) -> None:
        row = copy.deepcopy(self.row("basename"))
        row["required_options"] = "-Z"
        row["option_arguments"] = "-Z=<fabricated>"
        self.assertEqual(manifest.option_argument_gaps(row), {"-Z=<fabricated>"})

    def test_optional_attached_pr_arguments_are_supported(self) -> None:
        row = self.row("pr")
        self.assertIn("-e[char][gap]", row["option_arguments"])
        self.assertIn("-i[char][gap]", row["option_arguments"])
        self.assertIn("-n[char][width]", row["option_arguments"])
        self.assertIn("-s[char]", row["option_arguments"])
        self.assertEqual(manifest.declared_options(row) & {"-e", "-i", "-n", "-s"}, {"-e", "-i", "-n", "-s"})

    def test_known_guideline_10_exceptions_are_exact(self) -> None:
        self.assertIn("does not recognize", self.row("echo")["special_tokens"])
        self.assertIn("does not recognize", self.row("test")["special_tokens"])
        self.assertNotIn("Guideline 10 end-of-options", self.row("pr")["special_tokens"])

    def test_true_false_and_test_streams_are_exact(self) -> None:
        for name in ("true", "false", "test"):
            with self.subTest(command=name):
                self.assertEqual(self.row(name)["stdout"], "Not used.")
                self.assertIn(self.row(name)["stderr"], {"Not used.", "Used only for diagnostic messages."})

    def test_xargs_stderr_and_distinct_statuses_are_exact(self) -> None:
        row = self.row("xargs")
        self.assertIn("-t", row["stderr"])
        self.assertIn("-p", row["stderr"])
        for status in ("1-125", "126", "127"):
            self.assertIn(status, row["exit_status"])


if __name__ == "__main__":
    unittest.main()
