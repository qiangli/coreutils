#!/usr/bin/env python3
"""Validate and render the non-normative POSIX Issue 7 interface ledger."""

from __future__ import annotations

import argparse
import csv
import re
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/posix-required-command-interfaces.tsv"
LEGACY_MAP = ROOT / "docs/posix-required-commands.tsv"
GUIDE = ROOT / "docs/posix-required-command-interfaces.md"
PROVIDER_MANIFEST = ROOT / "pkg/posixprovider/manifest.tsv"

REQUIRED_NAMES = frozenset("""
awk basename bc cat cd chgrp chmod chown cksum cmp comm command cp cut date dd
diff dirname echo ed env expr false find fold getconf getopts grep head id join
kill ln locale localedef logger logname lp ls mailx mkdir mkfifo mv nohup od paste
pathchk pax pr printf pwd read rm rmdir sed sh sleep sort stty tail tee test touch
tr true tty umask uname uniq wait wc xargs alias at batch bg crontab csplit ctags
df du ex expand fc fg file jobs man mesg more newgrp nm nice patch ps renice split
strings tabs talk time tput unalias unexpand uudecode uuencode vi who write ar make
strip hash iconv m4 tsort
""".split())
SHELL_ONLY = frozenset(
    "alias bg cd command fc fg getopts hash jobs read sh umask unalias wait".split()
)
SHELL_SELECTED_OVER_GO = frozenset(
    "echo false kill printf pwd test true time".split()
)
SHELL_SELECTED = SHELL_ONLY | SHELL_SELECTED_OVER_GO
CUSTOM_PARSERS = frozenset("awk dd expr find sed stty".split())

FIELDS = (
    "command", "availability", "go_package", "effective_owner",
    "implementation_source", "parser_model", "base_synopsis",
    "conditional_synopsis", "applicability", "required_options",
    "conditional_options", "compatibility_scope", "option_arguments",
    "operands", "operand_rules", "special_tokens", "stdin", "environment",
    "stdout", "stderr", "effects", "exit_status", "clause_ids",
    "standard_source", "parser_source", "go_evidence", "shell_evidence",
    "provider_evidence", "evidence_state",
)
LEGACY_FIELDS = (
    "command", "coreutils_go_applet", "go_package", "shell_provided",
    "profile_cd_disposition",
)
RENDER_LABELS = (
    "Evidence state", "Applicability", "Issue 7 synopsis candidate",
    "Issue 7 required-option candidate", "Issue 7 conditional-option candidate",
    "Issue 7 option-argument candidate",
    "Operands", "Special tokens", "Standard input", "Environment",
    "Standard output", "Standard error", "Effects", "Exit status",
    "Compatibility scope", "Availability", "Effective owner",
    "Implementation", "Conservative source-token audit", "Evidence lanes",
    "Issue 7 source",
)

OPTION_TOKEN = re.compile(r"^[+-](?:[A-Za-z0-9]+|<[a-z][a-z0-9_]*>)$")
OPTION_ARGUMENT_OPTION = re.compile(
    r"^(?P<option>[+-](?:[A-Za-z0-9]+|<[a-z][a-z0-9_]*>))"
    r"(?:=<[^>]+>|\[[a-z_]+\](?:\[[a-z_]+\])*)$"
)
AVAILABILITY = {"go", "shell_only", "external_provider"}
OWNERS = {"go", "shell", "external_provider"}
PARSER_MODELS = {
    "flagset", "manual", "custom", "none", "shell_builtin",
    "shell_keyword", "external",
}
APPLICABILITY = {"base", "xsi", "development", "optional"}
EVIDENCE_STATES = {"verified", "partial", "unverified"}
REQUIRED_CLAUSES = {
    "SYNOPSIS", "OPTIONS", "OPERANDS", "ENVIRONMENT_VARIABLES", "STDIN",
    "INPUT_FILES", "STDOUT", "STDERR", "OUTPUT_FILES", "EXIT_STATUS",
    "CONSEQUENCES_OF_ERRORS",
}
POSIX_EVIDENCE_PREFIX = (
    "POSIX08-2016:https://pubs.opengroup.org/onlinepubs/"
    "9699919799.2016edition/utilities/"
)
COMPATIBILITY_SCOPE = "POSIX Issue 7 only; GNU compatibility is out of scope"
UNVERIFIED = "UNVERIFIED"
EXPLICIT_NONE = "NONE"
NORMATIVE_SEMANTIC_FIELDS = (
    "base_synopsis", "conditional_synopsis", "required_options",
    "conditional_options", "option_arguments", "operands", "operand_rules",
    "special_tokens", "stdin", "environment", "stdout", "stderr", "effects",
    "exit_status",
)
EVIDENCE_REF = re.compile(
    r"^(?P<path>[^#]+\.go)(?:#(?P<test>Test[A-Za-z0-9_]+))?$"
)
SHELL_EVIDENCE_REF = re.compile(
    r"^sh:(?P<path>[^#]+_test\.go)#(?P<test>Test[A-Za-z0-9_]+)$"
)
GENERIC_PROSE = (
    "where POSIX Utility Syntax Guideline 10 applies",
    "POSIX STDIN clause remains authoritative",
    "Write diagnostics required by POSIX",
    "Write the required result format",
    "Produce the files, state changes",
    "greater than 0 on error, except where POSIX defines",
    "repository evidence named by evidence_ids",
)


class ManifestError(ValueError):
    pass


def _provider_names(path: Path = PROVIDER_MANIFEST) -> set[str]:
    return {
        line.split("\t", 1)[0]
        for line in path.read_text().splitlines()
        if line and not line.startswith("#")
    }


def _go_packages(root: Path = ROOT) -> set[str]:
    source = (root / "cmds/all/all.go").read_text()
    return set(re.findall(r'github\.com/qiangli/coreutils/cmds/([^"/]+)', source))


def _flagset_packages(root: Path = ROOT) -> set[str]:
    result = set()
    for package in (root / "cmds").iterdir():
        if package.is_dir() and any(
            "tool.NewFlags(" in source.read_text() for source in package.glob("*.go")
        ):
            result.add(package.name)
    return result


def read_manifest(path: Path = MANIFEST) -> list[dict[str, str]]:
    with path.open(newline="") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        if tuple(reader.fieldnames or ()) != FIELDS:
            raise ManifestError(f"unexpected interface fields: {reader.fieldnames!r}")
        return list(reader)


def read_legacy_map(path: Path = LEGACY_MAP) -> list[dict[str, str]]:
    with path.open(newline="") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        if tuple(reader.fieldnames or ()) != LEGACY_FIELDS:
            raise ManifestError("the established five-column A/B/C/D map contract changed")
        return list(reader)


def _tokens(command: str, field: str, raw: str) -> list[str]:
    if raw in {"-", EXPLICIT_NONE}:
        return []
    tokens = raw.split(";")
    malformed = [token for token in tokens if not OPTION_TOKEN.fullmatch(token)]
    if malformed:
        raise ManifestError(f"{command}: malformed {field}: {', '.join(malformed)}")
    if len(tokens) != len(set(tokens)):
        raise ManifestError(f"{command}: duplicate {field}")
    return tokens


def _conditional_options(row: dict[str, str]) -> dict[str, list[str]]:
    if row["conditional_options"] in {"-", EXPLICIT_NONE}:
        return {}
    result = {}
    for group in row["conditional_options"].split(";"):
        if ":" not in group:
            raise ManifestError(f"{row['command']}: malformed conditional option group")
        tag, raw = group.split(":", 1)
        if tag not in APPLICABILITY - {"base"} or tag in result:
            raise ManifestError(f"{row['command']}: invalid conditional option group")
        values = raw.split(",")
        if any(not OPTION_TOKEN.fullmatch(value) for value in values):
            raise ManifestError(f"{row['command']}: malformed conditional option token")
        result[tag] = values
    return result


def _conditional_synopses(row: dict[str, str]) -> list[tuple[str, str]]:
    if row["conditional_synopsis"] == "-":
        return []
    result = []
    for item in row["conditional_synopsis"].split(" ; "):
        if "::" not in item:
            raise ManifestError(f"{row['command']}: malformed conditional synopsis")
        tag, form = item.split("::", 1)
        if tag not in APPLICABILITY - {"base"} or not form:
            raise ManifestError(f"{row['command']}: invalid conditional synopsis applicability")
        result.append((tag, form))
    return result


def declared_options(row: dict[str, str]) -> set[str]:
    result = set(_tokens(row["command"], "required options", row["required_options"]))
    result.update(value for values in _conditional_options(row).values() for value in values)
    if row["option_arguments"] not in {"-", EXPLICIT_NONE}:
        for item in row["option_arguments"].split(";"):
            match = OPTION_ARGUMENT_OPTION.fullmatch(item)
            if not match:
                raise ManifestError(f"{row['command']}: malformed option argument: {item}")
            if match.group("option") not in result:
                raise ManifestError(
                    f"{row['command']}: option argument names undeclared option "
                    f"{match.group('option')}"
                )
    return result


def recognized_go_options(row: dict[str, str], root: Path = ROOT) -> set[str]:
    """Conservatively find option-shaped tokens in non-test parser source."""
    package = root / row["go_package"]
    source = "\n".join(
        path.read_text() for path in sorted(package.glob("*.go"))
        if not path.name.endswith("_test.go")
    )
    chars = set(re.findall(
        r'\.\w+P\(\s*"[^"]*"\s*,\s*"([A-Za-z0-9])"', source
    ))
    chars.update(re.findall(
        r'\.\w+VarP\([^,]+,\s*"[^"]*"\s*,\s*"([A-Za-z0-9])"', source
    ))
    for case in re.findall(r"case\s+([^:]+):", source):
        chars.update(re.findall(r"'([A-Za-z0-9])'", case))
    for group in re.findall(r'extractShort\([^,]+,\s*"([A-Za-z0-9]+)"', source):
        chars.update(group)
    result = {"-" + char for char in chars}
    result.update(re.findall(r'"(-[A-Za-z0-9])"', source))
    if "a[i] >= '1' && a[i] <= '3'" in source:
        result.update({"-1", "-2", "-3"})
    if 'strings.ReplaceAll(a, "R", "r")' in source:
        result.add("-R")
    if 'strings.Trim(arg[1:], "0123456789")' in source:
        result.add("-<column>")
    if "scanPlusPage(" in source:
        result.add("+<page>")
    if "parseTabStops(" in source:
        result.add("-<n>")
    return result


def declared_option_arguments(row: dict[str, str]) -> dict[str, str]:
    result = {}
    if row["option_arguments"] in {"-", EXPLICIT_NONE}:
        return result
    for item in row["option_arguments"].split(";"):
        match = OPTION_ARGUMENT_OPTION.fullmatch(item)
        if not match:
            raise ManifestError(f"{row['command']}: malformed option argument: {item}")
        result[item] = match.group("option")
    return result


def recognized_go_option_arguments(
    row: dict[str, str], root: Path = ROOT,
) -> set[str]:
    """Conservatively find declared argument-form tokens in parser source."""
    if row["effective_owner"] != "go":
        return set()
    package = root / row["go_package"]
    source = "\n".join(
        path.read_text() for path in sorted(package.glob("*.go"))
        if not path.name.endswith("_test.go")
    )
    recognized_options = recognized_go_options(row, root)
    result = set()
    for item, option in declared_option_arguments(row).items():
        if option not in recognized_options:
            continue
        short = re.escape(option[1:])
        value_flag = re.search(
            rf'\.(?!Bool)\w+P\([^\n]*"{short}"\s*,', source
        ) or re.search(
            rf'\.(?!Bool)\w+VarP\([^\n]*"{short}"\s*,', source
        )
        manual_value = (
            f'"{option}"' in source
            and (
                "requires an argument" in source
                or "val()" in source
                or "needValue" in source
                or "parseOption" in source
            )
        )
        optional_pr = (
            row["command"] == "pr" and option in {"-e", "-i", "-n", "-s"}
            and f"'{option[1]}':" in source and "NoOptDefVal" in source
        )
        if value_flag or manual_value or optional_pr:
            result.add(item)
    return result


def parser_gaps(row: dict[str, str], root: Path = ROOT) -> set[str]:
    if row["effective_owner"] != "go":
        return set()
    return declared_options(row) - recognized_go_options(row, root)


def option_argument_gaps(row: dict[str, str], root: Path = ROOT) -> set[str]:
    if row["effective_owner"] != "go":
        return set()
    return set(declared_option_arguments(row)) - recognized_go_option_arguments(row, root)


def _test_is_declared(path: Path, test: str) -> bool:
    return bool(re.search(rf"^func\s+{re.escape(test)}\s*\(", path.read_text(), re.MULTILINE))


def _command_test_name(command: str, test: str) -> bool:
    """Require a provider test ID to name the command, not merely contain its letters."""
    suffix = test.removeprefix("Test")
    words = re.findall(r"[A-Z]+(?=[A-Z][a-z]|[0-9_]|$)|[A-Z]?[a-z]+|[0-9]+", suffix)
    if command.casefold() in {word.casefold() for word in words}:
        return True
    # Preserve names such as m4 as a single command token while rejecting an
    # accidental substring such as ar in TestArgvPassthrough.
    length = len(command)
    return (
        suffix[:length].casefold() == command.casefold()
        and (len(suffix) == length or not suffix[length].islower())
    )


def _local_evidence_ref(command: str, raw: str, lane: str, root: Path) -> bool:
    match = EVIDENCE_REF.fullmatch(raw)
    if not match:
        raise ManifestError(f"{command}: malformed {lane} evidence reference: {raw}")
    relative = Path(match.group("path"))
    if relative.is_absolute() or ".." in relative.parts:
        raise ManifestError(f"{command}: evidence path escapes its repository: {raw}")
    path = root / relative
    if not path.is_file() or not path.name.endswith("_test.go"):
        raise ManifestError(f"{command}: evidence path absent or not a focused test: {raw}")
    test = match.group("test")
    if test and not _test_is_declared(path, test):
        raise ManifestError(f"{command}: evidence test ID is absent: {raw}")
    if lane == "go_evidence":
        package = Path("cmds") / command
        if relative.parent != package:
            raise ManifestError(f"{command}: Go evidence is not command-package-focused")
    elif lane == "provider_evidence":
        if relative.parent != Path("cmds/posixproviders"):
            raise ManifestError(f"{command}: provider evidence is not in cmds/posixproviders")
        if not test or not _command_test_name(command, test):
            raise ManifestError(
                f"{command}: provider evidence is not command-specific; "
                "name an explicit command test ID"
            )
    return test is not None


def _shell_evidence_ref(command: str, raw: str, root: Path) -> bool:
    match = SHELL_EVIDENCE_REF.fullmatch(raw)
    if not match:
        raise ManifestError(
            f"{command}: shell evidence must use sh:<repo-path>#<test-ID> contract"
        )
    relative = Path(match.group("path"))
    if relative.is_absolute() or ".." in relative.parts:
        raise ManifestError(f"{command}: shell evidence path escapes the sh repository")
    shell_root = root.parent / "sh"
    path = shell_root / relative
    if not path.is_file() or not _test_is_declared(path, match.group("test")):
        return False
    if not _command_test_name(command, match.group("test")):
        raise ManifestError(f"{command}: shell evidence test ID is not command-specific")
    return True


def _validate_evidence(
    row: dict[str, str], lane: str, root: Path,
) -> tuple[int, bool, bool]:
    raw = row[lane]
    if raw == "-":
        return 0, True, False
    refs = raw.split(";")
    available = True
    explicit = True
    for ref in refs:
        if lane == "shell_evidence":
            available = _shell_evidence_ref(row["command"], ref, root) and available
        else:
            explicit = _local_evidence_ref(row["command"], ref, lane, root) and explicit
    return len(refs), available, explicit


def validate(
    rows: list[dict[str, str]], providers: set[str], go_packages: set[str],
    flagset_packages: set[str], root: Path = ROOT,
) -> None:
    legacy = read_legacy_map(root / "docs/posix-required-commands.tsv")
    if len(REQUIRED_NAMES) != 116 or len(rows) != 116 or len(legacy) != 116:
        raise ManifestError("required-command denominator drifted")
    for index, row in enumerate(rows, 1):
        missing = [field for field in FIELDS if not row.get(field)]
        if missing:
            raise ManifestError(
                f"{row.get('command') or f'row {index}'}: missing field(s): {', '.join(missing)}"
            )
    if [row["command"] for row in legacy] != [row["command"] for row in rows]:
        raise ManifestError("interface ledger no longer matches the old-map command order")
    names = [row["command"] for row in rows]
    if len(names) != len(set(names)) or set(names) != REQUIRED_NAMES:
        raise ManifestError("configured names drifted or contain duplicates")

    for row in rows:
        command = row["command"]
        if row["availability"] not in AVAILABILITY or row["effective_owner"] not in OWNERS:
            raise ManifestError(f"{command}: invalid availability/owner")
        if row["parser_model"] not in PARSER_MODELS:
            raise ManifestError(f"{command}: invalid parser model")
        if row["evidence_state"] not in EVIDENCE_STATES:
            raise ManifestError(f"{command}: invalid evidence state")
        if row["compatibility_scope"] != COMPATIBILITY_SCOPE:
            raise ManifestError(f"{command}: GNU compatibility must remain explicitly out of scope")
        flattened = " ".join(row.values())
        if any(phrase in flattened for phrase in GENERIC_PROSE):
            raise ManifestError(f"{command}: fabricated generic prose is forbidden")

        applicability = row["applicability"].split(";")
        if len(applicability) != len(set(applicability)) or any(
            value not in APPLICABILITY for value in applicability
        ):
            raise ManifestError(f"{command}: absent/invalid applicability")
        synopses = _conditional_synopses(row)
        if (row["base_synopsis"] != "-") != ("base" in applicability):
            raise ManifestError(f"{command}: base synopsis/applicability mismatch")
        if not ({tag for tag, _ in synopses} | set(_conditional_options(row))) <= set(applicability):
            raise ManifestError(f"{command}: undeclared conditional applicability")
        declared_options(row)

        if not row["clause_ids"].startswith(f"XCU:{command}:"):
            raise ManifestError(f"{command}: clause IDs do not identify this command")
        clauses = set(row["clause_ids"].split(":", 2)[2].split(","))
        if clauses != REQUIRED_CLAUSES:
            raise ManifestError(f"{command}: missing clause ID(s)")
        expected_standard = f"{POSIX_EVIDENCE_PREFIX}{command}.html"
        if row["standard_source"] != expected_standard:
            raise ManifestError(f"{command}: missing exact Issue 7 source")

        expected_availability = (
            "external_provider" if command in providers else
            "shell_only" if command in SHELL_ONLY else "go"
        )
        expected_owner = (
            "external_provider" if command in providers else
            "shell" if command in SHELL_SELECTED else "go"
        )
        if row["availability"] != expected_availability:
            raise ManifestError(f"{command}: availability drift")
        if row["effective_owner"] != expected_owner:
            raise ManifestError(f"{command}: owner drift")
        expected_package = (
            f"cmds/{command}" if expected_availability == "go" else
            "cmds/posixproviders" if command in providers else "-"
        )
        if row["go_package"] != expected_package:
            raise ManifestError(f"{command}: package drift")
        expected_source = (
            f"cmds/{command}" if expected_owner == "go" else
            f"shell:{command}" if expected_owner == "shell" else
            f"pkg/posixprovider/manifest.tsv#{command}"
        )
        if row["implementation_source"] != expected_source:
            raise ManifestError(f"{command}: implementation source drift")
        if expected_owner == "go":
            if row["parser_source"] != expected_package or not (root / expected_package).is_dir():
                raise ManifestError(f"{command}: parser source path is absent or unfocused")
            expected_model = (
                "custom" if command in CUSTOM_PARSERS else
                "flagset" if command in flagset_packages else
                "manual" if declared_options(row) else "none"
            )
            if row["parser_model"] != expected_model:
                raise ManifestError(f"{command}: parser model drift")
        elif row["parser_source"] != "-":
            raise ManifestError(f"{command}: shell/provider parser evidence crossed lanes")

        lane = {
            "go": "go_evidence", "shell": "shell_evidence",
            "external_provider": "provider_evidence",
        }[expected_owner]
        for other in {"go_evidence", "shell_evidence", "provider_evidence"} - {lane}:
            if row[other] != "-":
                raise ManifestError(f"{command}: evidence crossed implementation lanes")
        evidence_count, evidence_available, explicit_tests = _validate_evidence(
            row, lane, root
        )

        missing_semantics = [
            field for field in NORMATIVE_SEMANTIC_FIELDS
            if not row[field].strip() or UNVERIFIED in row[field]
        ]
        if row["base_synopsis"] == "-" and row["conditional_synopsis"] == "-":
            missing_semantics.extend(("base_synopsis", "conditional_synopsis"))
        if row["required_options"] == "-" and row["conditional_options"] == "-":
            missing_semantics.extend(("required_options", "conditional_options"))
        for field in (
            "option_arguments", "operands", "operand_rules", "special_tokens",
            "stdin", "environment", "stdout", "stderr", "effects", "exit_status",
        ):
            if row[field] == "-":
                missing_semantics.append(field)
        missing_semantics = list(dict.fromkeys(missing_semantics))
        if row["evidence_state"] == "verified":
            if (
                missing_semantics or not evidence_count or not evidence_available
                or not explicit_tests
                or parser_gaps(row, root) or option_argument_gaps(row, root)
            ):
                detail = ",".join(missing_semantics) or "focused behavioral evidence"
                raise ManifestError(
                    f"{command}: verified state launders missing semantics/evidence: {detail}"
                )
        if row["evidence_state"] == "partial" and (
            not evidence_count or not evidence_available
        ):
            raise ManifestError(f"{command}: partial state requires focused evidence")

    availability = Counter(row["availability"] for row in rows)
    owners = Counter(row["effective_owner"] for row in rows)
    if availability != Counter({"go": 86, "shell_only": 14, "external_provider": 16}):
        raise ManifestError(f"availability axis drift: {dict(availability)}")
    if owners != Counter({"go": 78, "shell": 22, "external_provider": 16}):
        raise ManifestError(f"effective-selection axis drift: {dict(owners)}")


def completion_errors(rows: list[dict[str, str]], root: Path = ROOT) -> list[str]:
    errors = []
    for row in rows:
        if row["evidence_state"] != "verified":
            errors.append(f"{row['command']}: state={row['evidence_state']}")
        gaps = sorted(parser_gaps(row, root))
        if gaps:
            errors.append(f"{row['command']}: parser gaps={','.join(gaps)}")
        argument_gaps = sorted(option_argument_gaps(row, root))
        if argument_gaps:
            errors.append(
                f"{row['command']}: parser argument gaps={','.join(argument_gaps)}"
            )
    return errors


def _display(raw: str) -> str:
    return "none" if raw in {"-", EXPLICIT_NONE} else raw.replace(";", "; ")


def _synopsis(row: dict[str, str]) -> str:
    forms = [] if row["base_synopsis"] == "-" else row["base_synopsis"].split(" ; ")
    forms.extend(f"[{tag}] {form}" for tag, form in _conditional_synopses(row))
    return "\n".join(forms)


def render(rows: list[dict[str, str]]) -> str:
    availability = Counter(row["availability"] for row in rows)
    owners = Counter(row["effective_owner"] for row in rows)
    states = Counter(row["evidence_state"] for row in rows)
    lines = [
        "# POSIX-required command interface evidence ledger", "",
        "Generated from `docs/posix-required-command-interfaces.tsv` by",
        "`scripts/posix_manifest.py`. This ledger is an audit aid, not a normative",
        "specification or a claim of complete POSIX conformance. `UNVERIFIED` means",
        "the command-specific Issue 7 semantics have not yet been transcribed and",
        "backed by focused repository evidence.", "",
        "GNU compatibility is explicitly out of scope and deferred.", "",
        "| Axis | Value | Count |", "| --- | --- | ---: |",
        f"| Availability | Go | {availability['go']} |",
        f"| Availability | Shell-only | {availability['shell_only']} |",
        f"| Availability | Provider | {availability['external_provider']} |",
        f"| Effective owner | Go | {owners['go']} |",
        f"| Effective owner | Shell | {owners['shell']} |",
        f"| Effective owner | Provider | {owners['external_provider']} |",
        f"| Evidence | Verified | {states['verified']} |",
        f"| Evidence | Partial | {states['partial']} |",
        f"| Evidence | Unverified | {states['unverified']} |", "",
        "Completion is deliberately fail-closed: `scripts/posix_manifest.py",
        "--require-complete` fails until all 116 rows have focused behavioral evidence",
        "and complete normative semantics. The parser scan below is only a conservative",
        "source-token audit; finding a token is never proof of runtime behavior.", "",
        "Evidence is lane-specific. Go references stay in `cmds/<command>`; provider",
        "references name a command-specific test in `cmds/posixproviders`; shell",
        "references use `sh:<path>#<TestID>` against the sibling sh repository. A",
        "missing cross-repository shell reference cannot support partial or verified state.", "",
        "For verified rows, `NONE` explicitly records an empty option-argument or",
        "operand set; `-` in those normative slots means missing data. Likewise, paired",
        "`-` synopsis or option fields are incomplete, and normative prose cannot be `-`.", "",
    ]
    for row in rows:
        standard_url = row["standard_source"].split(":", 1)[1]
        gaps = sorted(parser_gaps(row))
        argument_gaps = sorted(option_argument_gaps(row))
        parser_result = "not applicable to a Go-selected parser" if row["effective_owner"] != "go" else (
            "tokens found for all declared options and argument forms; behavioral evidence still required"
            if not gaps and not argument_gaps else
            "token gaps: options=" + (", ".join(gaps) or "none")
            + "; argument-form gaps=" + (", ".join(argument_gaps) or "none")
        )
        lines.extend([
            f"## `{row['command']}`", "",
            f"**Evidence state:** `{row['evidence_state']}`.", "",
            f"**Applicability:** `{_display(row['applicability'])}`.", "",
            "**Issue 7 synopsis candidate:**", "", "```text", _synopsis(row), "```", "",
            f"**Issue 7 required-option candidate:** `{_display(row['required_options'])}`.", "",
            f"**Issue 7 conditional-option candidate:** `{_display(row['conditional_options'])}`.", "",
            f"**Issue 7 option-argument candidate:** `{_display(row['option_arguments'])}`.", "",
            f"**Operands:** `{_display(row['operands'])}`. {row['operand_rules']}", "",
            f"**Special tokens:** {row['special_tokens']}", "",
            f"**Standard input:** {row['stdin']}", "",
            f"**Environment:** `{_display(row['environment'])}`.", "",
            f"**Standard output:** {row['stdout']}", "",
            f"**Standard error:** {row['stderr']}", "",
            f"**Effects:** `{row['effects']}`.", "",
            f"**Exit status:** {row['exit_status']}", "",
            f"**Compatibility scope:** {row['compatibility_scope']}.", "",
            f"**Availability:** `{row['availability']}`.", "",
            f"**Effective owner:** `{row['effective_owner']}` (`{row['parser_model']}`).", "",
            f"**Implementation:** `{row['implementation_source']}`.", "",
            f"**Conservative source-token audit:** {parser_result}; source `{row['parser_source']}`. "
            "This audit is not proof of behavior.", "",
            "**Evidence lanes:** "
            f"Go=`{row['go_evidence']}`; shell=`{row['shell_evidence']}`; "
            f"provider=`{row['provider_evidence']}`; clauses=`{row['clause_ids']}`.", "",
            f"**Issue 7 source:** [{row['command']}]({standard_url}).", "",
        ])
    return "\n".join(lines)


def validate_rendered(rendered: str, rows: list[dict[str, str]]) -> None:
    headings = re.findall(r"^## `([^`]+)`$", rendered, re.MULTILINE)
    if headings != [row["command"] for row in rows] or len(headings) != 116:
        raise ManifestError("per-command heading count/order drifted")
    sections = re.split(r"^## `[^`]+`$", rendered, flags=re.MULTILINE)[1:]
    for row, section in zip(rows, sections, strict=True):
        missing = [label for label in RENDER_LABELS if f"**{label}:**" not in section]
        if missing:
            raise ManifestError(f"{row['command']}: generated section missing fields")
        if f"`{row['evidence_state']}`" not in section:
            raise ManifestError(f"{row['command']}: generated section hides evidence state")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="validate and fail if output is stale")
    parser.add_argument(
        "--require-complete", action="store_true",
        help="also fail unless all interfaces have complete semantics and behavioral evidence",
    )
    args = parser.parse_args()
    rows = read_manifest()
    validate(rows, _provider_names(), _go_packages(), _flagset_packages())
    rendered = render(rows)
    validate_rendered(rendered, rows)
    if args.require_complete:
        errors = completion_errors(rows)
        if errors:
            raise SystemExit(
                f"POSIX interface completion blocked by {len(errors)} item(s):\n"
                + "\n".join(errors)
            )
    if args.check:
        if not GUIDE.exists():
            raise SystemExit(f"required generated document is absent: {GUIDE.name}")
        if GUIDE.read_text() != rendered:
            raise SystemExit("POSIX interface document is stale; run scripts/posix_manifest.py")
        states = Counter(row["evidence_state"] for row in rows)
        print(
            "posix-manifest: PASS (116 headings; availability 86/14/16; "
            "selection 78/22/16; evidence "
            f"{states['verified']} verified/{states['partial']} partial/"
            f"{states['unverified']} unverified)"
        )
        return
    GUIDE.write_text(rendered)
    print(f"posix-manifest: wrote {GUIDE.relative_to(ROOT)}")


if __name__ == "__main__":
    try:
        main()
    except ManifestError as error:
        raise SystemExit(f"posix-manifest: {error}") from error
