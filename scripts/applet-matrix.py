#!/usr/bin/env python3
"""Generate the shipped Bashy applet coverage/provider matrix."""

from __future__ import annotations

import argparse
import csv
import io
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

# GNU all-known command inventory from docs/reference-coreutils-comparison.md,
# intersected with command names (the multicall entrypoint itself is omitted).
GNU = set("""
[ arch b2sum base32 base64 basename basenc cat chcon chgrp chmod chown chroot
cksum comm cp csplit cut date dd df dir dircolors dirname du echo env expand expr
factor false fmt fold groups head hostid hostname id install join kill link ln
logname ls md5sum mkdir mkfifo mknod mktemp mv nice nl nohup nproc numfmt od
paste pathchk pinky pr printenv printf ptx pwd readlink realpath rm rmdir runcon
seq sha1sum sha224sum sha256sum sha384sum sha512sum shred shuf sleep sort split
stat stdbuf stty sum sync tac tail tee test timeout touch tr true truncate tsort
tty uname unexpand uniq unlink uptime users vdir wc who whoami yes
""".split())

# Required utility names from the configured VSC-PCTS2016 POSIX08 Commands &
# Utilities scenario. The rendered matrix intersects this set with the canonical
# shipped inventory; keeping the complete set here makes a newly shipped required
# command acquire the documentation label automatically.
POSIX_CERT_REQUIRED = set("""
awk basename bc cat cd chgrp chmod chown cksum cmp comm command cp cut date dd
diff dirname echo ed env expr false find fold getconf getopts grep head id join
kill ln locale localedef logger logname lp ls mailx mkdir mkfifo mv nohup od paste
pathchk pax pr printf pwd read rm rmdir sed sh sleep sort stty tail tee test touch
tr true tty umask uname uniq wait wc xargs alias at batch bg crontab csplit ctags
df du ex expand fc fg file jobs man mesg more newgrp nm nice patch ps renice split
strings tabs talk time tput unalias unexpand uudecode uuencode vi who write ar make
strip hash iconv m4 tsort
""".split())

# Required names supplied by the shell rather than by cmds/all in the assembled
# Profile C/D environment. `sh` is the shell entry point; the other names are
# shell builtins in both the GNU Bash control and Bashy.
SHELL_PROVIDED = set("""
alias bg cd command fc fg getopts hash jobs read sh umask unalias wait
""".split())

ALIASES = {
    "[": "test",
    "gunzip": "gzip",
    "ncal": "cal",
    "sntp": "ntp",
    "zcat": "gzip",
}

TEST_FUNC = re.compile(r"^func (?:Test|Fuzz|Benchmark)\w+", re.MULTILINE)
IMPORT = re.compile(r'github\.com/qiangli/coreutils/cmds/([^"/]+)"')

# One package registers MANY names: cmds/posixproviders registers the pinned
# POSIX external providers plus the `posix-providers` provisioning applet. The
# provider names come from the canonical manifest so this file can never drift
# from what is actually pinned.
#
# These are deliberately NOT reported as Go applets. The multicall owns the name
# and executes a locally built, provenance-checked copy of the upstream program;
# calling that a Go applet would overstate the Go userland by twelve commands and
# corrupt the very denominator this matrix exists to state honestly.
PROVIDER_PACKAGE = "posixproviders"
PROVIDER_ADMIN = "posix-providers"
PROVIDER_MANIFEST = ROOT / "pkg/posixprovider/manifest.tsv"
PROVIDER_FAMILY = "POSIX external provider"


def provider_names() -> list[str]:
    names = []
    for line in PROVIDER_MANIFEST.read_text().splitlines():
        if not line or line.startswith("#"):
            continue
        names.append(line.split("\t")[0].strip())
    if not names:
        raise SystemExit(f"no provider entries in {PROVIDER_MANIFEST}")
    return names


def shipped_packages() -> list[str]:
    text = (ROOT / "cmds/all/all.go").read_text()
    return sorted(set(IMPORT.findall(text)))


def package_tests(package: str) -> tuple[int, int]:
    files = sorted((ROOT / "cmds" / package).glob("*_test.go"))
    funcs = sum(len(TEST_FUNC.findall(path.read_text())) for path in files)
    return len(files), funcs


def rows() -> list[dict[str, str | int]]:
    if len(POSIX_CERT_REQUIRED) != 116:
        raise SystemExit(
            f"POSIX certification inventory changed: {len(POSIX_CERT_REQUIRED)} names; "
            "reconcile it with the configured scenario before regenerating"
        )
    packages = shipped_packages()
    providers = set(provider_names())
    applet_package = {package: package for package in packages}
    # cmds/posixproviders is not itself an advertised name; the names it
    # registers are.
    applet_package.pop(PROVIDER_PACKAGE, None)
    if PROVIDER_PACKAGE not in packages:
        raise SystemExit(f"cmds/{PROVIDER_PACKAGE} is no longer in cmds/all; reconcile the provider axis")
    for applet in sorted(providers) + [PROVIDER_ADMIN]:
        applet_package[applet] = PROVIDER_PACKAGE
    for applet, package in ALIASES.items():
        if package not in applet_package:
            raise SystemExit(f"alias {applet!r} refers to unshipped package {package!r}")
        applet_package[applet] = package

    result: list[dict[str, str | int]] = []
    for applet in sorted(applet_package):
        package = applet_package[applet]
        files, funcs = package_tests(package)
        if applet in providers:
            family = PROVIDER_FAMILY
        elif applet in GNU:
            family = "GNU Coreutils"
        elif applet in POSIX_CERT_REQUIRED:
            family = "POSIX/Unix utility"
        else:
            family = "Bashy/other extension"
        result.append({
            "applet": applet,
            "go_package": f"cmds/{package}",
            "alias_of": package if applet in ALIASES else "",
            "family": family,
            "gnu_coreutils": "yes" if applet in GNU else "no",
            "posix_cert_required": "yes" if applet in POSIX_CERT_REQUIRED else "no",
            "test_files": files,
            "test_functions": funcs,
        })

    if len(packages) != 153 or len(result) != 174:
        raise SystemExit(
            f"inventory changed: packages={len(packages)} applets={len(result)}; "
            "update the documented snapshot and generator assertions"
        )
    if any(row["test_files"] == 0 for row in result):
        missing = [str(row["applet"]) for row in result if row["test_files"] == 0]
        raise SystemExit("shipped applets lack package-local tests: " + ", ".join(missing))
    return result


def render_tsv(data: list[dict[str, str | int]]) -> str:
    fields = list(data[0])
    handle = io.StringIO(newline="")
    writer = csv.DictWriter(handle, fieldnames=fields, delimiter="\t", lineterminator="\n")
    writer.writeheader()
    writer.writerows(data)
    return handle.getvalue()


def required_rows(data: list[dict[str, str | int]]) -> list[dict[str, str]]:
    providers = set(provider_names())
    # A provider is registered in the multicall but is NOT a Go applet: the name
    # is ours, the implementation is upstream's, built locally from a pinned
    # source tarball. It is therefore neither `go_applet` (which would overstate
    # the Go userland) nor `external_gap` (which it no longer is, since Profile C
    # now resolves it from the provider cache instead of the host's PATH).
    internal = {
        str(row["applet"]): str(row["go_package"])
        for row in data
        if row["applet"] not in providers
    }
    result = []
    for command in sorted(POSIX_CERT_REQUIRED):
        if command in providers:
            disposition = "external_provider"
        elif command in internal:
            disposition = "go_applet"
        elif command in SHELL_PROVIDED:
            disposition = "shell"
        else:
            disposition = "external_gap"
        package = internal.get(command, "")
        if command in providers:
            package = f"cmds/{PROVIDER_PACKAGE}"
        result.append({
            "command": command,
            "coreutils_go_applet": "yes" if command in internal else "no",
            "go_package": package,
            "shell_provided": "yes" if command in SHELL_PROVIDED else "no",
            "profile_cd_disposition": disposition,
        })
    return result


def render_required_tsv(data: list[dict[str, str]]) -> str:
    handle = io.StringIO(newline="")
    writer = csv.DictWriter(handle, fieldnames=list(data[0]), delimiter="\t", lineterminator="\n")
    writer.writeheader()
    writer.writerows(data)
    return handle.getvalue()


def render_required_markdown(data: list[dict[str, str]]) -> str:
    go_count = sum(row["profile_cd_disposition"] == "go_applet" for row in data)
    shell_count = sum(row["profile_cd_disposition"] == "shell" for row in data)
    gap_count = sum(row["profile_cd_disposition"] == "external_gap" for row in data)
    provider_count = sum(row["profile_cd_disposition"] == "external_provider" for row in data)
    absent = len(data) - go_count
    lines = [
        "# POSIX-required command coverage for Profiles C/D",
        "",
        "Generated by `scripts/applet-matrix.py`; do not hand-edit the table.",
        "",
        "This is the canonical 116-name inventory from the configured",
        "VSC-PCTS2016 POSIX08 Commands & Utilities scenario. It separates the",
        "Bashy Go coreutils inventory from names supplied by the shell. Profile B",
        "does not use this Go-applet provider axis; it uses frozen GNU/system",
        "providers. Profiles C/D place Bashy Go coreutils first.",
        "",
        "## Totals",
        "",
        "| Disposition | Count |",
        "| --- | ---: |",
        f"| Registered Bashy Go applet | {go_count} |",
        f"| Shell entry point or builtin | {shell_count} |",
        f"| Pinned POSIX external provider | {provider_count} |",
        f"| External provider gap in assembled C/D | {gap_count} |",
        f"| Required names | {len(data)} |",
        "",
        f"Coreutils alone therefore covers {go_count} of {len(data)} same-name required sets and",
        f"is absent for {absent} names. {shell_count} of those {absent} are supplied by the shell and",
        f"{provider_count} by a pinned POSIX external provider that the multicall itself registers",
        "and resolves from the provider cache (`pkg/posixprovider`), leaving",
        f"{gap_count} true external-provider gaps in the assembled C/D environment.",
        "",
        "A provider row is NOT a Go applet: the name belongs to the multicall, the",
        "implementation is upstream's, built locally from a pinned source tarball",
        "and checked against its recorded provenance before it runs. It is listed",
        "separately precisely so it can never be counted as Go coverage.",
        "",
        "Presence is not behavioral conformance; test results must still be",
        "attributed to the executable or builtin actually selected.",
        "",
        "Machine-readable source: `docs/posix-required-commands.tsv`.",
        "",
        "## Complete required inventory",
        "",
        "| Command | Bashy Go applet | Go package | Shell supplied | C/D disposition |",
        "| --- | :---: | --- | :---: | --- |",
    ]
    labels = {
        "go_applet": "internal Go applet",
        "shell": "shell entry/builtin",
        "external_provider": "pinned external provider (registered)",
        "external_gap": "external provider required",
    }
    for row in data:
        package = f"`{row['go_package']}`" if row["go_package"] else "—"
        lines.append(
            f"| `{row['command']}` | {row['coreutils_go_applet']} | {package} | "
            f"{row['shell_provided']} | {labels[row['profile_cd_disposition']]} |"
        )
    lines.extend([
        "",
        f"The product/provider allocation for the {gap_count} remaining external gaps lives in",
        "`../../docs/posix-utility-provider-strategy.md`. The pins, licences and",
        "build recipe for the registered providers live in",
        "`pkg/posixprovider/manifest.tsv` + `tools/posix-providers/build.sh`.",
        "",
    ])
    return "\n".join(lines)


def render_markdown(data: list[dict[str, str | int]]) -> str:
    packages = len(shipped_packages())
    gnu = sum(row["gnu_coreutils"] == "yes" for row in data)
    posix_cert = sum(row["posix_cert_required"] == "yes" for row in data)
    aliases = sum(bool(row["alias_of"]) for row in data)
    lines = [
        "# Bashy shipped applet matrix",
        "",
        "Generated by `scripts/applet-matrix.py`; do not hand-edit the table.",
        "Generated from the current `cmds/all` registration tree; `chroot` and",
        "`runcon` remain withheld pending privileged integration coverage.",
        "",
        "## Interpretation",
        "",
        "- **GNU Coreutils** means the name belongs to GNU Coreutils' all-known",
        "  command inventory. It does not claim complete GNU option parity.",
        "- **POSIX cert required** means the configured VSC-PCTS2016 POSIX08",
        "  Commands & Utilities scenario contains a test set with the same name. It does",
        "  not prove that every applet option is covered or that an applet with",
        "  `no` is never invoked indirectly.",
        "- **Test files/functions** count package-local Go test files and top-level",
        "  `Test`, `Fuzz`, and `Benchmark` functions. Table-driven subtests are not",
        "  expanded, so these are structural indicators rather than coverage percentages.",
        "- Aliases share their implementation package but are listed separately",
        "  because they are separately advertised command names.",
        "",
        "## Totals",
        "",
        "| Measure | Count |",
        "|---|---:|",
        f"| Shipped Go command packages | {packages} |",
        f"| Advertised applet names | {len(data)} |",
        f"| Alias applet names | {aliases} |",
        f"| GNU Coreutils names | {gnu} |",
        f"| POSIX-cert-required names | {posix_cert} |",
        f"| Shipped names without a same-name required test set | {len(data) - posix_cert} |",
        "| Shipped names lacking package-local tests | 0 |",
        "| Release-withheld implementations | 2 (`chroot`, `runcon`) |",
        "",
        "The machine-readable source is `docs/applet-matrix.tsv`.",
        "",
        "## Complete matrix",
        "",
        "| Applet | Go package | Alias of | Family | GNU | POSIX cert required | Test files | Test functions |",
        "|---|---|---|---|:---:|:---:|---:|---:|",
    ]
    for row in data:
        alias = str(row["alias_of"]) or "—"
        applet = f"`{row['applet']}`"
        lines.append(
            f"| {applet} | `{row['go_package']}` | {alias} | {row['family']} | "
            f"{row['gnu_coreutils']} | {row['posix_cert_required']} | "
            f"{row['test_files']} | {row['test_functions']} |"
        )
    lines.extend([
        "",
        "## Release-withheld implementations",
        "",
        "| Name | Reason not advertised | Return condition |",
        "|---|---|---|",
        "| `chroot` | privileged root/credential behavior lacks bounded integration coverage | prove root change, credentials, child status, and cleanup |",
        "| `runcon` | SELinux label transition has only superficial coverage | prove transition, child status, restoration, and unsupported platforms |",
        "",
    ])
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="fail if generated files are stale")
    args = parser.parse_args()
    data = rows()
    required = required_rows(data)
    if len(SHELL_PROVIDED) != 14 or not SHELL_PROVIDED <= POSIX_CERT_REQUIRED:
        raise SystemExit("shell-provided POSIX inventory changed; reconcile before regenerating")
    dispositions = {row["profile_cd_disposition"] for row in required}
    counts = {kind: sum(row["profile_cd_disposition"] == kind for row in required) for kind in dispositions}
    if counts != {"go_applet": 86, "shell": 14, "external_provider": 16}:
        raise SystemExit(f"required coverage changed: {counts}; reconcile before regenerating")
    tsv = ROOT / "docs/applet-matrix.tsv"
    markdown = ROOT / "docs/applet-matrix.md"
    required_tsv = ROOT / "docs/posix-required-commands.tsv"
    required_markdown = ROOT / "docs/posix-required-commands.md"
    rendered_tsv = render_tsv(data)
    rendered_markdown = render_markdown(data)
    rendered_required_tsv = render_required_tsv(required)
    rendered_required_markdown = render_required_markdown(required)
    if args.check:
        if (not tsv.exists() or not markdown.exists() or
                not required_tsv.exists() or not required_markdown.exists() or
                tsv.read_text() != rendered_tsv or markdown.read_text() != rendered_markdown or
                required_tsv.read_text() != rendered_required_tsv or
                required_markdown.read_text() != rendered_required_markdown):
            raise SystemExit("applet matrix was stale; regenerate with scripts/applet-matrix.py")
        print("applet-matrix: PASS")
        return
    tsv.write_text(rendered_tsv)
    markdown.write_text(rendered_markdown)
    required_tsv.write_text(rendered_required_tsv)
    required_markdown.write_text(rendered_required_markdown)
    print(f"applet-matrix: wrote {len(data)} applets")


if __name__ == "__main__":
    main()
