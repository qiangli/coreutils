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

    def completed_semantics(self, name: str) -> dict[str, str]:
        row = self.row(name)
        changes = {
            field: "Specified by the command-specific POSIX interface."
            for field in manifest.NORMATIVE_SEMANTIC_FIELDS
            if manifest.UNVERIFIED in row[field]
        }
        if row["required_options"] == row["conditional_options"] == "-":
            changes["required_options"] = manifest.EXPLICIT_NONE
        if row["option_arguments"] == "-":
            changes["option_arguments"] = manifest.EXPLICIT_NONE
        if row["operands"] == "-":
            changes["operands"] = manifest.EXPLICIT_NONE
        for field in (
            "operand_rules", "special_tokens", "stdin", "environment", "stdout",
            "stderr", "effects", "exit_status",
        ):
            if row[field] == "-":
                changes[field] = "Not used."
        return changes

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
        self.assertIn("| Evidence | Implemented | 3 |", rendered)
        self.assertIn("| Evidence | Partial | 98 |", rendered)
        self.assertIn("| Evidence | Missing | 15 |", rendered)
        self.assertEqual(self.row("nice")["evidence_state"], "implemented")

    def test_exact_four_state_vocabulary_is_enforced(self) -> None:
        self.assertEqual(
            manifest.EVIDENCE_STATES,
            {"missing", "partial", "implemented", "verified"},
        )
        for state in ("missing", "partial", "implemented"):
            with self.subTest(state=state):
                manifest.validate(
                    self.changed("nice", evidence_state=state),
                    self.providers, self.packages, self.flagsets,
                )
        self.assertRejected(
            self.changed("nice", evidence_state="verified"),
            "verified state is unavailable.*implemented is the highest",
        )
        for state in ("unverified", "complete", "typo"):
            with self.subTest(state=state):
                self.assertRejected(
                    self.changed("nice", evidence_state=state), "invalid evidence state",
                )

    def test_completion_fails_closed_while_any_row_is_unverified(self) -> None:
        errors = manifest.completion_errors(self.rows)
        self.assertTrue(any(error == "bg: state=partial" for error in errors))
        with (
            mock.patch.object(sys, "argv", [str(SCRIPT), "--require-complete"]),
            self.assertRaisesRegex(SystemExit, "completion blocked"),
        ):
            manifest.main()

    def test_owned_completion_gate_has_exact_79_plus_22_scope(self) -> None:
        owned_errors = manifest.completion_errors(
            self.rows, owners=manifest.OWNED_IMPLEMENTATION_OWNERS,
        )
        provider_names = {
            row["command"] for row in self.rows
            if row["effective_owner"] == "external_provider"
        }
        self.assertEqual(
            sum(
                row["effective_owner"] in manifest.OWNED_IMPLEMENTATION_OWNERS
                for row in self.rows
            ),
            101,
        )
        self.assertFalse(
            any(error.split(":", 1)[0] in provider_names for error in owned_errors)
        )
        self.assertTrue(any(error == "bg: state=partial" for error in owned_errors))
        self.assertTrue(any(error == "xargs: state=partial" for error in owned_errors))
        with (
            mock.patch.object(
                sys, "argv", [str(SCRIPT), "--require-owned-complete"],
            ),
            self.assertRaisesRegex(SystemExit, "owned POSIX interface completion blocked"),
        ):
            manifest.main()

    def test_full_completion_gate_still_includes_external_providers(self) -> None:
        full_errors = manifest.completion_errors(self.rows)
        owned_errors = manifest.completion_errors(
            self.rows, owners=manifest.OWNED_IMPLEMENTATION_OWNERS,
        )
        self.assertGreater(len(full_errors), len(owned_errors))
        self.assertIn("ar: state=missing", full_errors)
        self.assertNotIn("ar: state=missing", owned_errors)

    def test_final_gates_explicitly_reject_implemented(self) -> None:
        self.assertIn("nice: state=implemented", manifest.completion_errors(self.rows))
        self.assertIn(
            "nice: state=implemented",
            manifest.completion_errors(
                self.rows, owners=manifest.OWNED_IMPLEMENTATION_OWNERS,
            ),
        )

    def test_every_interface_field_is_required(self) -> None:
        for field in manifest.FIELDS:
            with self.subTest(field=field):
                self.assertRejected(self.changed("true", **{field: ""}), "missing field")

    def test_counts_and_selection_axes_are_exact(self) -> None:
        self.assertRejected(self.changed("echo", availability="shell_only"), "availability drift")
        self.assertRejected(self.changed("echo", effective_owner="go"), "owner drift")

    def test_sh_is_a_staged_shell_entrypoint(self) -> None:
        row = self.row("sh")
        self.assertEqual(row["availability"], "shell_only")
        self.assertEqual(row["effective_owner"], "shell")
        self.assertEqual(row["parser_model"], "shell_entrypoint")
        self.assertRejected(
            self.changed("sh", parser_model="shell_builtin"),
            "parser/owner model drift",
        )

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

    def test_shell_routing_lane_names_every_accepted_profile_b_route(self) -> None:
        self.assertIn("shell_routing_evidence", manifest.FIELDS)
        route_tests = {
            "alias": "Alias", "bg": "Bg", "cd": "Cd", "command": "Command",
            "echo": "Echo", "false": "False", "fc": "Fc", "fg": "Fg",
            "getopts": "Getopts", "hash": "Hash", "jobs": "Jobs", "kill": "Kill",
            "printf": "Printf", "pwd": "Pwd", "read": "Read", "sh": "Sh",
            "test": "Test", "time": "Time", "true": "True", "umask": "Umask",
            "unalias": "Unalias", "wait": "Wait",
        }
        expected = {
            command: (
                "bashy:internal/cli/profile_b_routing_test.go"
                f"#TestProfileBRoute{suffix}"
            )
            for command, suffix in route_tests.items()
        }
        expected["sh"] = ";".join((
            "bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteSh",
            "bashy:internal/cli/main_test.go#TestStrictPosixEngagedByArgv0Sh",
            "bashy:internal/cli/profile_b_sh_entrypoint_unix_test.go"
            "#TestProfileBShUtilityEntrypointContract",
        ))
        actual = {
            row["command"]: row["shell_routing_evidence"]
            for row in self.rows if row["shell_routing_evidence"] != "-"
        }
        self.assertEqual(actual, expected)
        self.assertEqual(
            {row["command"] for row in self.rows if row["effective_owner"] == "shell"},
            set(expected),
        )

    def test_shell_routing_evidence_is_shell_owner_only(self) -> None:
        self.assertRejected(
            self.changed(
                "xargs",
                shell_routing_evidence=(
                    "bashy:internal/cli/posix_routing_test.go"
                    "#TestXargsShellRouting"
                ),
            ),
            "shell routing evidence is only valid for shell-selected commands",
        )

    def test_shell_routing_reference_contract_is_fail_closed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "coreutils"
            root.mkdir()
            test_path = root.parent / "bashy/internal/cli/posix_routing_test.go"
            test_path.parent.mkdir(parents=True)
            test_path.write_text(
                "package cli\n\n"
                "func TestTrueShellWinsOverGoApplet(t *testing.T) {}\n"
                "func TestEchoShellWinsOverGoApplet(t *testing.T) {}\n"
            )
            valid = (
                "bashy:internal/cli/posix_routing_test.go"
                "#TestTrueShellWinsOverGoApplet"
            )
            self.assertTrue(manifest._shell_routing_evidence_ref("true", valid, root))
            self.assertFalse(
                manifest._shell_routing_evidence_ref(
                    "true",
                    "bashy:internal/cli/posix_routing_test.go#TestTrueAbsent",
                    root,
                )
            )
            with self.assertRaisesRegex(
                manifest.ManifestError, "not command-specific",
            ):
                manifest._shell_routing_evidence_ref(
                    "true",
                    "bashy:internal/cli/posix_routing_test.go"
                    "#TestEchoShellWinsOverGoApplet",
                    root,
                )

    def test_shell_routing_reference_rejects_wrong_repo_and_path(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "coreutils"
            root.mkdir()
            with self.assertRaisesRegex(
                manifest.ManifestError, "must use bashy:<approved-path>",
            ):
                manifest._shell_routing_evidence_ref(
                    "true", "sh:internal/cli/route_test.go#TestTrueRouting", root,
                )
            with self.assertRaisesRegex(manifest.ManifestError, "escapes"):
                manifest._shell_routing_evidence_ref(
                    "true",
                    "bashy:internal/cli/../route_test.go#TestTrueRouting",
                    root,
                )
            with self.assertRaisesRegex(manifest.ManifestError, "outside approved"):
                manifest._shell_routing_evidence_ref(
                    "true",
                    "bashy:cmd/bashy/route_test.go#TestTrueRouting",
                    root,
                )
            with self.assertRaisesRegex(manifest.ManifestError, "outside approved"):
                manifest._shell_routing_evidence_ref(
                    "true",
                    "bashy:internal/agentos/route_test.go#TestTrueRouting",
                    root,
                )

    def test_routing_evidence_cannot_substitute_for_shell_semantics(self) -> None:
        rows = self.changed(
            "bg",
            evidence_state="partial",
            shell_evidence="-",
            shell_routing_evidence=(
                "bashy:internal/cli/posix_routing_test.go#TestBgShellRouting"
            ),
        )
        with mock.patch.object(
            manifest, "_shell_routing_evidence_ref", return_value=True,
        ):
            self.assertRejected(rows, "partial state requires focused semantic evidence")

    def test_implemented_shell_row_requires_both_semantic_and_routing_lanes(self) -> None:
        semantic = "sh:interp/posix_bg_test.go#TestBgIssue7Interface"
        routing = (
            "bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteBg"
        )
        changes = self.completed_semantics("bg")
        changes.update(
            evidence_state="implemented", shell_evidence=semantic,
            shell_routing_evidence="-",
        )
        with mock.patch.object(manifest, "_shell_evidence_ref", return_value=True):
            self.assertRejected(
                self.changed("bg", **changes),
                "focused shell routing evidence",
            )

        changes = self.completed_semantics("bg")
        changes.update(
            evidence_state="implemented",
            shell_evidence="-",
            shell_routing_evidence=routing,
        )
        with mock.patch.object(
            manifest, "_shell_routing_evidence_ref", return_value=True,
        ):
            self.assertRejected(
                self.changed("bg", **changes),
                "focused behavioral evidence",
            )

        changes["shell_evidence"] = semantic
        with (
            mock.patch.object(manifest, "_shell_evidence_ref", return_value=True),
            mock.patch.object(
                manifest, "_shell_routing_evidence_ref", return_value=True,
            ),
        ):
            manifest.validate(
                self.changed("bg", **changes),
                self.providers,
                self.packages,
                self.flagsets,
            )

    def test_unavailable_shell_routing_reference_is_rejected_even_unverified(self) -> None:
        self.assertRejected(
            self.changed(
                "bg",
                shell_routing_evidence=(
                    "bashy:internal/cli/not_present_test.go#TestBgShellRouting"
                ),
            ),
            "shell routing evidence is unavailable or unfocused",
        )

    def test_xargs_operands_and_environment_cannot_be_laundered(self) -> None:
        evidence = "cmds/xargs/xargs_test.go#TestXargsDefaultEcho"
        for field in ("operands", "environment"):
            for missing in (manifest.UNVERIFIED, "-"):
                with self.subTest(field=field, missing=missing):
                    self.assertRejected(
                        self.changed(
                            "xargs", evidence_state="implemented", go_evidence=evidence,
                            **{field: missing},
                        ),
                        rf"implemented state launders.*{field}",
                    )

    def test_nlspath_is_recorded_as_xsi_applicable(self) -> None:
        environment = next(
            row["environment"] for row in self.rows if row["command"] == "cat"
        )
        self.assertIn("xsi:NLSPATH", environment)
        self.assertNotRegex(environment, manifest.BARE_NLSPATH)
        self.assertRejected(
            self.changed(
                "cat", environment=environment.replace("xsi:NLSPATH", "NLSPATH"),
            ),
            "NLSPATH must be recorded with xsi: applicability",
        )

    def test_xsi_nlspath_requires_lc_messages(self) -> None:
        environment = next(
            row["environment"] for row in self.rows if row["command"] == "cat"
        )
        self.assertRejected(
            self.changed("cat", environment=environment.replace("LC_MESSAGES;", "")),
            "xsi:NLSPATH requires the LC_MESSAGES category",
        )

    def test_lc_messages_keeps_the_xsi_nlspath_disposition(self) -> None:
        environment = next(
            row["environment"] for row in self.rows if row["command"] == "cat"
        )
        self.assertRejected(
            self.changed("cat", environment=environment.replace(";xsi:NLSPATH", "")),
            "LC_MESSAGES requires the XSI NLSPATH disposition",
        )

    def test_explicit_none_is_distinct_from_missing_normative_data(self) -> None:
        self.assertEqual(manifest._tokens("true", "options", manifest.EXPLICIT_NONE), [])
        self.assertEqual(manifest._display(manifest.EXPLICIT_NONE), "none")

    def test_true_cannot_substitute_pr_test_as_shell_evidence(self) -> None:
        changes = self.completed_semantics("true")
        changes.update(
            evidence_state="implemented",
            shell_evidence="cmds/pr/pr_test.go#TestPRDefaultPageStructure",
        )
        self.assertRejected(
            self.changed("true", **changes),
            "shell evidence must use sh:<repo-path>#<test-ID> contract",
        )

    def test_sh_semantic_lane_uses_only_approved_sh_tests(self) -> None:
        approved = "sh:interp/interp_test.go#TestRunnerPosixStdinArgv0"
        with mock.patch.object(Path, "is_file", return_value=True), mock.patch.object(
            manifest, "_test_is_declared", return_value=True,
        ):
            self.assertTrue(manifest._shell_evidence_ref("sh", approved, manifest.ROOT))
            with self.assertRaisesRegex(manifest.ManifestError, "not command-specific"):
                manifest._shell_evidence_ref(
                    "sh", "sh:interp/interp_test.go#TestRunnerEnvNoModify", manifest.ROOT,
                )

    def test_sh_semantic_lane_cannot_use_bashy_routing_evidence(self) -> None:
        changes = self.completed_semantics("sh")
        changes.update(
            evidence_state="partial",
            shell_evidence=(
                "bashy:internal/cli/profile_b_sh_entrypoint_unix_test.go"
                "#TestProfileBShUtilityEntrypointContract"
            ),
        )
        self.assertRejected(
            self.changed("sh", **changes),
            "shell evidence must use sh:<repo-path>#<test-ID> contract",
        )

    def test_ar_cannot_substitute_pr_test_as_provider_evidence(self) -> None:
        changes = self.completed_semantics("ar")
        changes.update(
            evidence_state="implemented",
            provider_evidence="cmds/pr/pr_test.go#TestPRDefaultPageStructure",
        )
        self.assertRejected(
            self.changed("ar", **changes),
            "provider evidence is not in cmds/posixproviders",
        )

    def test_ar_cannot_use_an_unrelated_provider_test(self) -> None:
        self.assertRejected(
            self.changed(
                "ar",
                provider_evidence=(
                    "cmds/posixproviders/posixproviders_test.go"
                    "#TestMaterializedManifestMatchesTheEmbeddedOne"
                ),
            ),
            "provider evidence is not command-specific",
        )

    def test_command_specific_provider_test_ids_have_token_boundaries(self) -> None:
        self.assertTrue(manifest._command_test_name("ar", "TestArOperands"))
        self.assertTrue(manifest._command_test_name("m4", "TestM4Diagnostics"))
        self.assertFalse(manifest._command_test_name("ar", "TestArgvPassthrough"))

    def test_unavailable_cross_repo_shell_evidence_cannot_be_focused(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "coreutils"
            root.mkdir()
            self.assertFalse(
                manifest._shell_evidence_ref(
                    "true", "sh:interp/true_test.go#TestTrueExitStatus", root
                )
            )
        self.assertRejected(
            self.changed(
                "true", **self.completed_semantics("true"),
                evidence_state="implemented",
                shell_evidence=(
                    "sh:interp/posix_true_evidence_test.go#TestTrueIssue7Interface"
                ),
            ),
            "implemented state launders.*focused behavioral evidence",
        )

    def test_implemented_go_evidence_requires_an_explicit_test_id(self) -> None:
        self.assertRejected(
            self.changed(
                "xargs", evidence_state="implemented",
                go_evidence="cmds/xargs/xargs_test.go",
            ),
            "implemented state launders.*focused behavioral evidence",
        )

    def test_integration_evidence_is_deferred_and_fails_closed(self) -> None:
        for evidence in (
            "certification ledger",
            "profile-c@" + "a" * 40 + "#nice/run.tsv@sha256=" + "b" * 64,
        ):
            with self.subTest(evidence=evidence):
                self.assertRejected(
                    self.changed("nice", integration_evidence=evidence),
                    "integration verification gate is deferred/unavailable",
                )

    def test_verified_cannot_be_mocked_past_the_deferred_gate(self) -> None:
        with mock.patch.object(
            manifest, "_integration_profiles", return_value={"profile-c", "profile-d"},
        ):
            self.assertRejected(
                self.changed("nice", evidence_state="verified"),
                "verified state is unavailable.*implemented is the highest",
            )

    def test_future_integration_profile_mapping_is_exact(self) -> None:
        self.assertEqual(
            manifest.REQUIRED_INTEGRATION_PROFILES,
            {
                "go": frozenset({"profile-c", "profile-d"}),
                "shell": frozenset({"profile-b", "profile-d"}),
                "external_provider": frozenset({"profile-c", "profile-d"}),
            },
        )

    def test_owned_source_gate_has_exact_scope_and_accepts_only_ready_states(self) -> None:
        errors = manifest.owned_source_errors(self.rows)
        self.assertEqual(sum(error.endswith("state=partial") for error in errors), 98)
        self.assertFalse(any(error.startswith("ar:") for error in errors))
        with (
            mock.patch.object(sys, "argv", [str(SCRIPT), "--require-owned-source-complete"]),
            self.assertRaisesRegex(SystemExit, "owned POSIX source completion blocked"),
        ):
            manifest.main()

    def test_owned_source_gate_accepts_ready_rows_and_rejects_missing(self) -> None:
        rows = copy.deepcopy(self.rows)
        for row in rows:
            if row["effective_owner"] in manifest.OWNED_IMPLEMENTATION_OWNERS:
                row["evidence_state"] = "implemented"
        self.assertEqual(manifest.owned_source_errors(rows), [])
        next(row for row in rows if row["command"] == "nice")["evidence_state"] = "missing"
        self.assertIn("nice: state=missing", manifest.owned_source_errors(rows))

    def test_implemented_state_rejects_every_incomplete_source_lane(self) -> None:
        self.assertRejected(
            self.changed("nice", effects=manifest.UNVERIFIED),
            "implemented state launders.*effects",
        )
        self.assertRejected(
            self.changed("nice", go_evidence="cmds/nice/absent_test.go#TestNiceAbsent"),
            "evidence path absent",
        )
        self.assertRejected(
            self.changed("false", shell_routing_evidence="-"),
            "implemented state launders.*shell routing evidence",
        )
        with mock.patch.object(manifest, "parser_gaps", return_value={"-Z"}):
            self.assertRejected(self.changed("nice"), "implemented state launders")
        with mock.patch.object(
            manifest, "option_argument_gaps", return_value={"-n=<adjustment>"},
        ):
            self.assertRejected(self.changed("nice"), "implemented state launders")

    def test_provider_registration_alone_cannot_establish_implemented(self) -> None:
        changes = self.completed_semantics("ar")
        changes.update({
            "evidence_state": "implemented",
            "provider_evidence": (
                "cmds/posixproviders/posixproviders_test.go#"
                "TestProviderNamesAreRegistered"
            ),
        })
        self.assertRejected(
            self.changed("ar", **changes), "provider evidence is not command-specific",
        )

    def test_partial_go_evidence_requires_explicit_test_ids(self) -> None:
        self.assertRejected(
            self.changed("xargs", go_evidence="cmds/xargs/xargs_test.go"),
            "partial state requires focused evidence",
        )

    def test_state_laundering_is_rejected(self) -> None:
        self.assertRejected(
            self.changed("bg", evidence_state="implemented", effects="UNVERIFIED"),
            "implemented state launders",
        )

    def test_all_normative_semantic_fields_are_fail_closed(self) -> None:
        evidence = "cmds/xargs/xargs_test.go#TestXargsDefaultEcho"
        for field in manifest.NORMATIVE_SEMANTIC_FIELDS:
            for missing in ("", manifest.UNVERIFIED):
                with self.subTest(field=field, missing=missing):
                    rows = self.changed(
                        "xargs", evidence_state="implemented", go_evidence=evidence,
                        **{field: missing},
                    )
                    self.assertRejected(rows, "missing field" if not missing else ".+")
        self.assertRejected(
            self.changed(
                "basename", evidence_state="implemented",
                required_options="-Z", go_evidence="cmds/basename/basename_test.go",
            ),
            "implemented state launders",
        )

    def test_owned_partial_and_unverified_rows_require_complete_semantics(self) -> None:
        self.assertRejected(
            self.changed("xargs", operand_rules=manifest.UNVERIFIED),
            "owned row has incomplete normative semantics.*operand_rules",
        )
        self.assertRejected(
            self.changed("bg", stdout=manifest.UNVERIFIED),
            "owned row has incomplete normative semantics.*stdout",
        )

    def test_parser_source_comparison_covers_every_go_selected_row(self) -> None:
        go_rows = [row for row in self.rows if row["effective_owner"] == "go"]
        self.assertEqual(len(go_rows), 79)
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

    def test_render_calls_parser_scan_a_conservative_token_audit(self) -> None:
        rendered = manifest.render(self.rows)
        self.assertIn("conservative source-token audit", rendered.lower())
        self.assertIn("never proof of runtime behavior", rendered)
        self.assertNotIn("PASS: all declared options", rendered)

    def test_render_defines_states_deferred_integration_and_final_gates(self) -> None:
        rendered = manifest.render(self.rows)
        for state in ("missing", "partial", "implemented", "verified"):
            self.assertRegex(rendered, rf"- `{state}`:")
        self.assertIn("Integration verification is deferred and unavailable", rendered)
        self.assertIn("implemented` is therefore the highest currently attainable", rendered)
        self.assertIn("Go and external", rendered)
        self.assertIn("provider rows require Profiles C+D", rendered)
        self.assertIn("shell rows require Profiles B+D", rendered)
        self.assertIn("Both final gates accept", rendered)
        self.assertIn("only `verified`", rendered)
        self.assertIn("caller-authored", rendered)

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

    def test_manual_and_helper_parsers_have_no_scanner_false_gaps(self) -> None:
        for command in ("diff", "file", "join", "pr", "tabs"):
            with self.subTest(command=command):
                row = self.row(command)
                self.assertEqual(manifest.parser_gaps(row), set())
                self.assertEqual(manifest.option_argument_gaps(row), set())

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
