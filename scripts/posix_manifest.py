#!/usr/bin/env python3
"""Validate and render the canonical POSIX08 command-interface manifest."""

from __future__ import annotations

import argparse
import csv
import re
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/posix-required-commands.tsv"
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
    "conditional_options", "gnu_only_options", "option_arguments",
    "operands", "operand_rules", "special_tokens", "stdin", "environment",
    "output", "effects", "diagnostics", "exit_status", "clause_ids",
    "evidence_ids", "tests", "evidence_state",
)

RENDER_LABELS = (
    "Requirement / applicability", "Normative POSIX synopsis",
    "Mandatory base options", "Conditional / optional options",
    "GNU-only material", "Option arguments", "Operands / arity / order",
    "Special `-` / `--` / standard input", "Environment", "Output / effects",
    "Diagnostics / status", "Availability", "Effective Profile C/D owner",
    "Implementation source", "Tests / evidence / state",
    "Official Open Group Issue 7/2016 link",
)

OPTION_TOKEN = re.compile(r"^[+-](?:[A-Za-z0-9]+|<[a-z][a-z0-9_]*>)$")
OPTION_ARGUMENT = re.compile(
    r"^(?P<option>[+-](?:[A-Za-z0-9]+|<[a-z][a-z0-9_]*>))="
    r"(?P<argument><[a-z][a-z0-9_]*(?:\[=[a-z][a-z0-9_]*\])?>|"
    r"<[a-z][a-z0-9_]*>\[\.\.\.\])$"
)
AVAILABILITY = {"go", "shell_only", "external_provider"}
OWNERS = {"go", "shell", "external_provider"}
PARSER_MODELS = {
    "flagset", "manual", "custom", "none", "shell_builtin",
    "shell_keyword", "external",
}
APPLICABILITY = {"base", "xsi", "development", "optional"}
EVIDENCE_STATES = {"specified", "verified", "partial", "unverified"}
REQUIRED_CLAUSES = {
    "SYNOPSIS", "OPTIONS", "OPERANDS", "ENVIRONMENT_VARIABLES", "STDIN",
    "INPUT_FILES", "STDOUT", "STDERR", "OUTPUT_FILES", "EXIT_STATUS",
    "CONSEQUENCES_OF_ERRORS",
}
POSIX_EVIDENCE_PREFIX = (
    "POSIX08-2016:https://pubs.opengroup.org/onlinepubs/"
    "9699919799.2016edition/utilities/"
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
    result: set[str] = set()
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
            raise ManifestError(
                f"unexpected fields: {reader.fieldnames!r}; want {FIELDS!r}"
            )
        return list(reader)


def _option_tokens(command: str, field: str, raw: str) -> list[str]:
    if raw == "-":
        return []
    tokens = raw.split(";")
    malformed = [token for token in tokens if not OPTION_TOKEN.fullmatch(token)]
    if malformed:
        raise ManifestError(
            f"{command}: malformed {field} option token(s): {', '.join(malformed)}"
        )
    duplicates = sorted(token for token, count in Counter(tokens).items() if count > 1)
    if duplicates:
        raise ManifestError(
            f"{command}: duplicate {field} option token(s): {', '.join(duplicates)}"
        )
    return tokens


def _conditional_options(row: dict[str, str]) -> dict[str, list[str]]:
    raw = row["conditional_options"]
    if raw == "-":
        return {}
    result: dict[str, list[str]] = {}
    for group in raw.split(";"):
        if ":" not in group:
            raise ManifestError(f"{row['command']}: malformed conditional option group")
        tag, values = group.split(":", 1)
        if tag not in APPLICABILITY - {"base"}:
            raise ManifestError(f"{row['command']}: invalid option applicability {tag}")
        if tag in result:
            raise ManifestError(f"{row['command']}: duplicate conditional option group {tag}")
        tokens = values.split(",")
        malformed = [token for token in tokens if not OPTION_TOKEN.fullmatch(token)]
        if malformed:
            raise ManifestError(
                f"{row['command']}: malformed conditional option token(s): "
                + ", ".join(malformed)
            )
        result[tag] = tokens
    return result


def _conditional_synopses(row: dict[str, str]) -> list[tuple[str, str]]:
    raw = row["conditional_synopsis"]
    if raw == "-":
        return []
    result = []
    for item in raw.split(" ; "):
        if "::" not in item:
            raise ManifestError(f"{row['command']}: malformed conditional synopsis")
        tag, form = item.split("::", 1)
        if tag not in APPLICABILITY - {"base"} or not form:
            raise ManifestError(f"{row['command']}: invalid conditional synopsis applicability")
        result.append((tag, form))
    return result


def _validate_option_arguments(row: dict[str, str], options: set[str]) -> None:
    raw = row["option_arguments"]
    if raw == "-":
        return
    seen: set[str] = set()
    for item in raw.split(";"):
        match = OPTION_ARGUMENT.fullmatch(item)
        if not match:
            raise ManifestError(f"{row['command']}: malformed option argument: {item}")
        option = match.group("option")
        if option not in options:
            raise ManifestError(
                f"{row['command']}: option argument names undeclared option {option}"
            )
        if option in seen:
            raise ManifestError(f"{row['command']}: duplicate option argument for {option}")
        seen.add(option)


def _validate_synopsis(row: dict[str, str]) -> None:
    command = row["command"]
    forms = [] if row["base_synopsis"] == "-" else row["base_synopsis"].split(" ; ")
    forms.extend(form for _, form in _conditional_synopses(row))
    if not forms or any(not form.strip() for form in forms):
        raise ManifestError(f"{command}: missing normative synopsis")
    for form in forms:
        words = form.split()
        if command not in words and not (command == "test" and form.startswith("[ [")):
            raise ManifestError(f"{command}: synopsis does not name its command: {form}")


def validate(
    rows: list[dict[str, str]], providers: set[str], go_packages: set[str],
    flagset_packages: set[str],
) -> None:
    if len(REQUIRED_NAMES) != 116 or len(rows) != 116:
        raise ManifestError(f"manifest denominator drifted to {len(rows)}")
    if len(providers) != 16:
        raise ManifestError(f"pinned-provider denominator drifted to {len(providers)}")
    for index, row in enumerate(rows, start=1):
        missing = [field for field in FIELDS if not row.get(field)]
        if missing:
            raise ManifestError(
                f"{row.get('command') or f'row {index}'}: missing field(s): "
                + ", ".join(missing)
            )
    commands = [row.get("command", "") for row in rows]
    duplicates = sorted(name for name, count in Counter(commands).items() if count > 1)
    if duplicates:
        raise ManifestError("duplicate command row(s): " + ", ".join(duplicates))
    actual = set(commands)
    if actual != REQUIRED_NAMES:
        raise ManifestError(
            "configured names drifted; missing=" + ",".join(sorted(REQUIRED_NAMES - actual))
            + " extra=" + ",".join(sorted(actual - REQUIRED_NAMES))
        )

    for row in rows:
        command = row["command"]
        if row["availability"] not in AVAILABILITY:
            raise ManifestError(f"{command}: invalid availability {row['availability']}")
        if row["effective_owner"] not in OWNERS:
            raise ManifestError(f"{command}: invalid owner {row['effective_owner']}")
        if row["parser_model"] not in PARSER_MODELS:
            raise ManifestError(f"{command}: invalid parser model {row['parser_model']}")
        if row["evidence_state"] not in EVIDENCE_STATES:
            raise ManifestError(f"{command}: invalid evidence state {row['evidence_state']}")

        applicability = row["applicability"].split(";")
        if len(applicability) != len(set(applicability)) or any(
            item not in APPLICABILITY for item in applicability
        ):
            raise ManifestError(f"{command}: absent/invalid applicability")
        if (row["base_synopsis"] != "-") != ("base" in applicability):
            raise ManifestError(f"{command}: base synopsis/applicability mismatch")
        conditional_synopses = _conditional_synopses(row)
        conditional_options = _conditional_options(row)
        conditional_tags = {tag for tag, _ in conditional_synopses} | set(conditional_options)
        if not conditional_tags <= set(applicability):
            raise ManifestError(f"{command}: conditional applicability is undeclared")

        _validate_synopsis(row)
        required = set(_option_tokens(command, "mandatory", row["required_options"]))
        conditional = {token for tokens in conditional_options.values() for token in tokens}
        gnu = set(_option_tokens(command, "GNU-only", row["gnu_only_options"]))
        if required & conditional:
            raise ManifestError(f"{command}: conditional option mixed into mandatory set")
        if (conditional | gnu) & required or conditional & gnu:
            raise ManifestError(f"{command}: option applicability sets overlap")
        _validate_option_arguments(row, required | conditional | gnu)

        if not row["clause_ids"].startswith(f"XCU:{command}:"):
            raise ManifestError(f"{command}: clause_ids do not identify its XCU clauses")
        clauses = set(row["clause_ids"].split(":", 2)[2].split(","))
        if clauses != REQUIRED_CLAUSES:
            raise ManifestError(
                f"{command}: missing clause ID(s): "
                + ", ".join(sorted(REQUIRED_CLAUSES - clauses))
            )
        evidence = row["evidence_ids"].split(";")
        expected_link = f"{POSIX_EVIDENCE_PREFIX}{command}.html"
        if (
            len(evidence) != 2 or evidence[0] != expected_link
            or not evidence[1].startswith("REPO:") or evidence[1] == "REPO:"
        ):
            raise ManifestError(
                f"{command}: missing exact POSIX08-2016 or repository evidence ID"
            )

        expected_availability = (
            "external_provider" if command in providers else
            "shell_only" if command in SHELL_ONLY else "go"
        )
        if row["availability"] != expected_availability:
            raise ManifestError(f"{command}: availability drift")
        expected_owner = (
            "external_provider" if command in providers else
            "shell" if command in SHELL_SELECTED else "go"
        )
        if row["effective_owner"] != expected_owner:
            raise ManifestError(f"{command}: owner drift")
        expected_package = (
            f"cmds/{command}" if expected_availability == "go" else
            "cmds/posixproviders" if command in providers else "-"
        )
        if row["go_package"] != expected_package:
            raise ManifestError(f"{command}: package drift; want {expected_package}")
        expected_source = (
            f"cmds/{command}" if expected_owner == "go" else
            f"shell:{command}" if expected_owner == "shell" else
            f"pkg/posixprovider/manifest.tsv#{command}"
        )
        if row["implementation_source"] != expected_source:
            raise ManifestError(f"{command}: implementation source drift")

        model = row["parser_model"]
        if expected_owner == "external_provider" and model != "external":
            raise ManifestError(f"{command}: provider must use external parser model")
        if expected_owner == "shell":
            wanted = "shell_keyword" if command == "time" else "shell_builtin"
            if model != wanted:
                raise ManifestError(f"{command}: shell parser model drift; want {wanted}")
        if expected_owner == "go":
            expected_model = (
                "custom" if command in CUSTOM_PARSERS else
                "flagset" if command in flagset_packages else
                "manual" if required or conditional else "none"
            )
            if model != expected_model:
                raise ManifestError(f"{command}: parser model drift; want {expected_model}")

    availability_counts = Counter(row["availability"] for row in rows)
    wanted_availability = Counter({"go": 86, "shell_only": 14, "external_provider": 16})
    if availability_counts != wanted_availability:
        raise ManifestError(
            f"availability axis drift: {dict(availability_counts)}; "
            f"want {dict(wanted_availability)}"
        )
    owner_counts = Counter(row["effective_owner"] for row in rows)
    wanted_owners = Counter({"go": 78, "shell": 22, "external_provider": 16})
    if owner_counts != wanted_owners:
        raise ManifestError(
            f"effective-selection axis drift: {dict(owner_counts)}; want {dict(wanted_owners)}"
        )


def _display(raw: str) -> str:
    return "none" if raw == "-" else raw.replace(";", "; ")


def _synopsis(row: dict[str, str]) -> str:
    lines = []
    if row["base_synopsis"] != "-":
        lines.extend(row["base_synopsis"].split(" ; "))
    lines.extend(f"[{tag}] {form}" for tag, form in _conditional_synopses(row))
    return "\n".join(lines)


def render(rows: list[dict[str, str]]) -> str:
    availability = Counter(row["availability"] for row in rows)
    owners = Counter(row["effective_owner"] for row in rows)
    lines = [
        "# POSIX-required command interfaces for Profiles C/D", "",
        "Generated from the canonical machine-readable data in",
        "`docs/posix-required-commands.tsv` by `scripts/posix_manifest.py`.",
        "The 116 sections below preserve requirement applicability and keep",
        "mandatory base interfaces separate from XSI, software-development,",
        "other optional, and GNU-only material.", "",
        "### Availability axis", "", "| Implementation available | Count |",
        "| --- | ---: |", f"| Go same-name applet | {availability['go']} |",
        f"| Shell-only name | {availability['shell_only']} |",
        f"| Pinned external provider | {availability['external_provider']} |", "",
        "### Effective Profile C/D selection axis", "",
        "| Selected implementation | Count |", "| --- | ---: |",
        f"| Go | {owners['go']} |", f"| Shell | {owners['shell']} |",
        f"| Pinned external provider | {owners['external_provider']} |", "",
        "Availability and effective selection are independent: eight available Go",
        "applets are intentionally shadowed by shell interfaces.", "",
    ]
    availability_labels = {
        "go": "Go same-name applet", "shell_only": "shell-only interface",
        "external_provider": "pinned external provider",
    }
    owner_labels = {
        "go": "Go", "shell": "shell", "external_provider": "pinned external provider"
    }
    for row in rows:
        link = row["evidence_ids"].split(";", 1)[0].split(":", 1)[1]
        repo_evidence = row["evidence_ids"].split(";", 1)[1]
        lines.extend([
            f"## `{row['command']}`", "",
            f"**Requirement / applicability:** {_display(row['applicability'])}.", "",
            "**Normative POSIX synopsis:**", "", "```text", _synopsis(row), "```", "",
            f"**Mandatory base options:** `{_display(row['required_options'])}`.", "",
            f"**Conditional / optional options:** `{_display(row['conditional_options'])}`.", "",
            f"**GNU-only material:** `{_display(row['gnu_only_options'])}`.", "",
            f"**Option arguments:** `{_display(row['option_arguments'])}`.", "",
            f"**Operands / arity / order:** {_display(row['operands'])}. {row['operand_rules']}", "",
            f"**Special `-` / `--` / standard input:** {_display(row['special_tokens'])}. {row['stdin']}", "",
            f"**Environment:** `{_display(row['environment'])}`.", "",
            f"**Output / effects:** {row['output']} Effects classification: `{row['effects']}`.", "",
            f"**Diagnostics / status:** {row['diagnostics']} Exit status: {row['exit_status']}", "",
            f"**Availability:** {availability_labels[row['availability']]}.", "",
            f"**Effective Profile C/D owner:** {owner_labels[row['effective_owner']]} "
            f"(`{row['parser_model']}`).", "",
            f"**Implementation source:** `{row['implementation_source']}`.", "",
            f"**Tests / evidence / state:** `{row['tests']}`; `{repo_evidence}`; "
            f"clauses `{row['clause_ids']}`; state `{row['evidence_state']}`.", "",
            f"**Official Open Group Issue 7/2016 link:** "
            f"[{row['command']}]({link}).", "",
        ])
    return "\n".join(lines)


def validate_rendered(rendered: str, rows: list[dict[str, str]]) -> None:
    headings = re.findall(r"^## `([^`]+)`$", rendered, re.MULTILINE)
    if len(headings) != 116 or headings != [row["command"] for row in rows]:
        raise ManifestError(
            f"per-command heading count/order drifted: {len(headings)}; want 116"
        )
    sections = re.split(r"^## `[^`]+`$", rendered, flags=re.MULTILINE)[1:]
    for row, section in zip(rows, sections, strict=True):
        missing = [label for label in RENDER_LABELS if f"**{label}:**" not in section]
        if missing:
            raise ManifestError(
                f"{row['command']}: generated section missing field(s): {', '.join(missing)}"
            )
        expected_link = f"{POSIX_EVIDENCE_PREFIX}{row['command']}.html".split(":", 1)[1]
        if expected_link not in section:
            raise ManifestError(f"{row['command']}: generated section missing exact official link")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="validate and fail if output is stale")
    args = parser.parse_args()
    rows = read_manifest()
    validate(rows, _provider_names(), _go_packages(), _flagset_packages())
    rendered = render(rows)
    validate_rendered(rendered, rows)
    if args.check:
        if not GUIDE.exists():
            raise SystemExit(f"required generated document is absent: {GUIDE.name}")
        if GUIDE.read_text() != rendered:
            raise SystemExit("POSIX interface document is stale; run scripts/posix_manifest.py")
        print(
            "posix-manifest: PASS (116 headings; availability 86 Go / 14 shell-only / "
            "16 providers; selection 78 Go / 22 shell / 16 providers)"
        )
        return
    GUIDE.write_text(rendered)
    print(f"posix-manifest: wrote {GUIDE.relative_to(ROOT)}")


if __name__ == "__main__":
    try:
        main()
    except ManifestError as error:
        raise SystemExit(f"posix-manifest: {error}") from error
