#!/usr/bin/env python3
"""Validate the canonical POSIX08 interface manifest and render its guide.

The TSV is curated normative data. In particular, this program never mines
command help, Synopsis strings, or other prose for option spellings.
"""

from __future__ import annotations

import argparse
import csv
import re
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "docs/posix-required-commands.tsv"
GUIDE = ROOT / "docs/posix-required-commands.md"
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

SHELL_ONLY = frozenset("alias bg cd command fc fg getopts hash jobs read sh umask unalias wait".split())
SHELL_SELECTED_OVER_GO = frozenset("echo false kill printf pwd test true time".split())
SHELL_SELECTED = SHELL_ONLY | SHELL_SELECTED_OVER_GO

# These implementations parse POSIX command grammars directly. They are
# explicit because a flag-registration scan cannot discover their options.
CUSTOM_PARSERS = frozenset("awk dd expr find sed stty".split())

FIELDS = (
    "command", "coreutils_go_applet", "go_package", "shell_provided",
    "profile_cd_disposition", "implementation_available", "parser_model",
    "syntax_forms", "required_options", "option_arguments", "operands",
    "environment", "required_effects", "clause_ids", "evidence_ids",
    "evidence_state",
)

OPTION_TOKEN = re.compile(r"^[+-](?:[A-Za-z0-9]+|<[a-z][a-z0-9_]*>)$")
OPTION_ARGUMENT = re.compile(
    r"^(?P<option>[+-](?:[A-Za-z0-9]+|<[a-z][a-z0-9_]*>))="
    r"(?P<argument><[a-z][a-z0-9_]*(?:\[=[a-z][a-z0-9_]*\])?>|"
    r"<[a-z][a-z0-9_]*>\[\.\.\.\])$"
)
DISPOSITIONS = {"go_applet", "shell", "external_provider"}
PARSER_MODELS = {"flagset", "manual", "custom", "none", "shell_builtin", "shell_keyword", "external"}
EVIDENCE_STATES = {"specified", "verified", "partial", "unverified"}
REQUIRED_CLAUSES = {
    "SYNOPSIS", "OPTIONS", "OPERANDS", "ENVIRONMENT_VARIABLES", "STDIN",
    "INPUT_FILES", "STDOUT", "STDERR", "OUTPUT_FILES", "EXIT_STATUS",
    "CONSEQUENCES_OF_ERRORS",
}


class ManifestError(ValueError):
    pass


def _provider_names(path: Path = PROVIDER_MANIFEST) -> set[str]:
    names: set[str] = set()
    for line in path.read_text().splitlines():
        if line and not line.startswith("#"):
            names.add(line.split("\t", 1)[0])
    return names


def _go_packages(root: Path = ROOT) -> set[str]:
    source = (root / "cmds/all/all.go").read_text()
    return set(re.findall(r'github\.com/qiangli/coreutils/cmds/([^"/]+)', source))


def _flagset_packages(root: Path = ROOT) -> set[str]:
    result: set[str] = set()
    for package in (root / "cmds").iterdir():
        if not package.is_dir():
            continue
        if any("tool.NewFlags(" in source.read_text() for source in package.glob("*.go")):
            result.add(package.name)
    return result


def read_manifest(path: Path = MANIFEST) -> list[dict[str, str]]:
    with path.open(newline="") as handle:
        reader = csv.DictReader(handle, delimiter="\t")
        if tuple(reader.fieldnames or ()) != FIELDS:
            raise ManifestError(f"unexpected fields: {reader.fieldnames!r}; want {FIELDS!r}")
        return list(reader)


def _tokens(row: dict[str, str]) -> list[str]:
    raw = row["required_options"]
    if raw == "-":
        return []
    tokens = raw.split(";")
    malformed = [token for token in tokens if not OPTION_TOKEN.fullmatch(token)]
    if malformed:
        raise ManifestError(f"{row['command']}: malformed option token(s): {', '.join(malformed)}")
    duplicates = sorted(token for token, count in Counter(tokens).items() if count > 1)
    if duplicates:
        raise ManifestError(f"{row['command']}: duplicate option token(s): {', '.join(duplicates)}")
    return tokens


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
            raise ManifestError(f"{row['command']}: option argument names undeclared option {option}")
        if option in seen:
            raise ManifestError(f"{row['command']}: duplicate option argument for {option}")
        seen.add(option)


def validate(
    rows: list[dict[str, str]],
    providers: set[str],
    go_packages: set[str],
    flagset_packages: set[str],
) -> None:
    if len(REQUIRED_NAMES) != 116:
        raise ManifestError(f"configured denominator drifted to {len(REQUIRED_NAMES)}")
    if len(providers) != 16:
        raise ManifestError(f"pinned-provider denominator drifted to {len(providers)}")
    if len(rows) != 116:
        raise ManifestError(f"manifest denominator drifted to {len(rows)}")

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
        missing = [field for field in FIELDS if not row.get(field)]
        if missing:
            raise ManifestError(f"{command or '<unnamed>'}: missing field(s): {', '.join(missing)}")
        if row["profile_cd_disposition"] not in DISPOSITIONS:
            raise ManifestError(f"{command}: invalid disposition {row['profile_cd_disposition']}")
        if row["parser_model"] not in PARSER_MODELS:
            raise ManifestError(f"{command}: invalid parser model {row['parser_model']}")
        if row["evidence_state"] not in EVIDENCE_STATES:
            raise ManifestError(f"{command}: invalid evidence state {row['evidence_state']}")
        if row["implementation_available"] != "yes":
            raise ManifestError(f"{command}: selected implementation is not available")
        if not row["clause_ids"].startswith(f"XCU:{command}:"):
            raise ManifestError(f"{command}: clause_ids do not identify its XCU clauses")
        clauses = set(row["clause_ids"].split(":", 2)[2].split(","))
        if clauses != REQUIRED_CLAUSES:
            missing_clauses = sorted(REQUIRED_CLAUSES - clauses)
            raise ManifestError(f"{command}: missing clause ID(s): {', '.join(missing_clauses)}")
        evidence = row["evidence_ids"].split(";")
        if (len(evidence) != 2 or not evidence[0].startswith("POSIX08-2016:https://")
                or not evidence[1].startswith("REPO:") or evidence[1] == "REPO:"):
            raise ManifestError(f"{command}: missing POSIX08-2016 or repository evidence ID")

        options = set(_tokens(row))
        _validate_option_arguments(row, options)
        if not options and row["option_arguments"] != "-":
            raise ManifestError(f"{command}: option arguments present without options")

        expected_owner = (
            "external_provider" if command in providers else
            "shell" if command in SHELL_SELECTED else "go_applet"
        )
        if row["profile_cd_disposition"] != expected_owner:
            raise ManifestError(f"{command}: owner drift: {row['profile_cd_disposition']}; want {expected_owner}")
        has_go = command in go_packages and command not in providers
        if row["coreutils_go_applet"] != ("yes" if has_go else "no"):
            raise ManifestError(f"{command}: Go availability drift")
        expected_package = f"cmds/{command}" if has_go else ("cmds/posixproviders" if command in providers else "-")
        if row["go_package"] != expected_package:
            raise ManifestError(f"{command}: package drift: {row['go_package']}; want {expected_package}")
        if row["shell_provided"] != ("yes" if command in SHELL_SELECTED else "no"):
            raise ManifestError(f"{command}: shell availability/selection drift")

        model = row["parser_model"]
        if expected_owner == "external_provider" and model != "external":
            raise ManifestError(f"{command}: provider must use external parser model")
        if expected_owner == "shell":
            wanted = "shell_keyword" if command == "time" else "shell_builtin"
            if model != wanted:
                raise ManifestError(f"{command}: shell parser model drift: {model}; want {wanted}")
        if expected_owner == "go_applet" and model not in {"flagset", "manual", "custom", "none"}:
            raise ManifestError(f"{command}: Go owner has non-Go parser model {model}")
        if expected_owner == "go_applet" and options and model == "none":
            raise ManifestError(f"{command}: required options need an explicit parser model")
        if expected_owner == "go_applet":
            expected_model = (
                "custom" if command in CUSTOM_PARSERS else
                "flagset" if command in flagset_packages else
                "manual" if options else "none"
            )
            if model != expected_model:
                raise ManifestError(f"{command}: parser model drift: {model}; want {expected_model}")

    counts = Counter(row["profile_cd_disposition"] for row in rows)
    expected_counts = Counter({"go_applet": 78, "shell": 22, "external_provider": 16})
    if counts != expected_counts:
        raise ManifestError(f"owner denominator drift: {dict(counts)}; want {dict(expected_counts)}")


def render(rows: list[dict[str, str]]) -> str:
    counts = Counter(row["profile_cd_disposition"] for row in rows)
    go_available = sum(row["coreutils_go_applet"] == "yes" for row in rows)
    lines = [
        "# POSIX-required command interfaces for Profiles C/D", "",
        "Generated from the canonical machine-readable manifest",
        "`docs/posix-required-commands.tsv` by `scripts/posix_manifest.py`.", "",
        "The manifest is limited to the 116 required names configured by the",
        "VSC-PCTS2016 POSIX08 Commands & Utilities scenario. Its syntax and",
        "interface fields are curated from POSIX.1 Issue 7, 2016 Edition; the",
        "generator deliberately never extracts options from help or prose.", "",
        "## Ownership baseline", "",
        "| Effective Profile C/D owner | Count |", "| --- | ---: |",
        f"| Go-selected | {counts['go_applet']} |",
        f"| Shell-selected | {counts['shell']} |",
        f"| Pinned external provider | {counts['external_provider']} |",
        f"| Required names | {len(rows)} |", "",
        f"There are {go_available} same-name Go implementations available. Eight are",
        "intentionally shadowed by the shell (`echo`, `false`, `kill`, `printf`,",
        "`pwd`, `test`, `true`, and the `time` keyword), so availability must not",
        "be confused with effective ownership.", "",
        "## TSV contract", "",
        "`syntax_forms`, `operands`, `environment`, and `required_effects` are",
        "semicolon-separated normalized interface facts. `required_options` contains",
        "only explicit option tokens; `option_arguments` maps those tokens to required",
        "arguments. A single `-` means none. `parser_model` records flag-set, manual,",
        "custom, shell, keyword, and external parsing explicitly.", "",
        "`clause_ids` identify applicable XCU sections. `evidence_ids` bind each row",
        "to the 2016 POSIX page and selected implementation source. `specified` means",
        "the interface has been recorded, not that behavioral conformance is proved.", "",
        "Run `python3 scripts/posix_manifest.py --check` to fail on stale generated",
        "documentation, denominator/owner drift, duplicate or malformed option tokens,",
        "and missing clauses or evidence.", "",
        "## Effective owner index", "",
        "| Command | Go available | Shell selected | Effective owner | Parser | Evidence state |",
        "| --- | :---: | :---: | --- | --- | --- |",
    ]
    labels = {"go_applet": "Go", "shell": "shell", "external_provider": "pinned provider"}
    for row in rows:
        lines.append(
            f"| `{row['command']}` | {row['coreutils_go_applet']} | {row['shell_provided']} | "
            f"{labels[row['profile_cd_disposition']]} | `{row['parser_model']}` | "
            f"`{row['evidence_state']}` |"
        )
    return "\n".join(lines) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="validate and fail if the guide is stale")
    args = parser.parse_args()
    rows = read_manifest()
    validate(rows, _provider_names(), _go_packages(), _flagset_packages())
    rendered = render(rows)
    if args.check:
        if not GUIDE.exists() or GUIDE.read_text() != rendered:
            raise SystemExit("POSIX interface guide is stale; run scripts/posix_manifest.py")
        print("posix-manifest: PASS (116 names; owners 78 Go / 22 shell / 16 pinned)")
        return
    GUIDE.write_text(rendered)
    print("posix-manifest: wrote docs/posix-required-commands.md")


if __name__ == "__main__":
    try:
        main()
    except ManifestError as error:
        raise SystemExit(f"posix-manifest: {error}") from error
