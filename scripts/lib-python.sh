#!/bin/sh
# scripts/lib-python.sh — locate a Python 3.10+ interpreter for coreutils dev scripts.
#
# Sets PY and provides py() helper for running Python commands.
# If $PYTHON is set, use it (and verify 3.10+).
# Else if bashy is on PATH, use "bashy python" (pinned 3.10+).
# Else if python3 exists and is 3.10+, use python3.
# Else exit 2 with diagnostic.

if [ -n "${PYTHON:-}" ]; then
	# shellcheck disable=SC2086
	if $PYTHON -c 'import sys; sys.exit(sys.version_info < (3,10))' >/dev/null 2>&1; then
		PY="$PYTHON"
	else
		echo "coreutils scripts need Python 3.10+: install bashy (bashy python) or set PYTHON" >&2
		exit 2
	fi
elif command -v bashy >/dev/null 2>&1; then
	PY="bashy python"
elif command -v python3 >/dev/null 2>&1 && python3 -c 'import sys; sys.exit(sys.version_info < (3,10))' >/dev/null 2>&1; then
	PY="python3"
else
	echo "coreutils scripts need Python 3.10+: install bashy (bashy python) or set PYTHON" >&2
	exit 2
fi

export PY

py() {
	# shellcheck disable=SC2086
	PYTHONDONTWRITEBYTECODE=1 $PY "$@"
}
