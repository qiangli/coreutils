#!/bin/sh
# Preserve the established generated five-column A/B/C/D coverage contract.
set -eu
cd "$(git rev-parse --show-toplevel)"

scripts/applet-matrix.py --check
PYTHONDONTWRITEBYTECODE=1 python3 - <<'PY'
import csv
from collections import Counter

path = "docs/posix-required-commands.tsv"
fields = (
    "command", "coreutils_go_applet", "go_package", "shell_provided",
    "profile_cd_disposition",
)
with open(path, newline="") as handle:
    reader = csv.DictReader(handle, delimiter="\t")
    if tuple(reader.fieldnames or ()) != fields:
        raise SystemExit("POSIX required-command map is not the exact five-column contract")
    rows = list(reader)
if len(rows) != 116 or len({row["command"] for row in rows}) != 116:
    raise SystemExit("POSIX required-command map must contain exactly 116 unique names")
counts = Counter(row["profile_cd_disposition"] for row in rows)
want = Counter({"go_applet": 90, "shell": 14, "external_provider": 12})
if counts != want:
    raise SystemExit(f"POSIX required-command availability drift: {dict(counts)}")
print("validate-posix-required-commands: PASS (five columns; 116 names; 90/14/12)")
PY
