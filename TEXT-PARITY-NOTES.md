# Uutils Option Parity Status for Text/Stream Utilities

This document outlines the option gaps that have been reduced/closed as part of this iteration.

## Closed/Implemented Option Gaps

We have fully implemented, integrated, and verified the requested option sets for the following utilities:

### 1. `comm`
- **Options Added**:
  - `--check-order` / `--nocheck-order` (forces/disables sorting verification on all input lines)
  - `--output-delimiter=STR` (customizes output column separators)
  - `--total` (emits a summary of line counts per column, respecting suppression)
  - `-z` / `--zero-terminated` (switches line terminator to NUL)
- **Tests**: Comprehensive unit tests added in [comm_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/comm/comm_test.go).

### 2. `cut`
- **Options Added**:
  - `--output-delimiter=STRING` (customizes field and range delimiters in the output)
  - `-z` / `--zero-terminated` (reads/writes NUL-terminated lines)
  - `-n` (compatibility flag, parsed and ignored)
  - `-w` / `--whitespace-delimited` (splits fields by arbitrary runs of consecutive spaces/tabs and trims leading/trailing spaces)
- **Tests**: Comprehensive unit tests added in [cut_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/cut/cut_test.go).

### 3. `join`
- **Options Added**:
  - `--check-order` / `--nocheck-order` (enforces or disables ordering verification)
  - `--header` (treats first line of each file as header and joins them directly)
  - `-o FORMAT` (customizes output columns/formatting)
  - `-e EMPTY` (customizes filler for missing fields under custom output formatting)
  - `-z` / `--zero-terminated` (reads/writes NUL-terminated lines)
- **Tests**: Comprehensive unit tests added in [join_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/join/join_test.go).

### 4. `printenv`
- **Options Added**:
  - `-0` / `--null` (terminates each printed environment variable or value with NUL)
- **Tests**: Verification tests added in [printenv_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/printenv/printenv_test.go).

### 5. `seq`
- **Options Added**:
  - `-t` / `--terminator=STRING` (terminates the output with a custom string instead of a newline)
- **Tests**: Verification tests added in [seq_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/seq/seq_test.go).

### 6. `shuf`
- **Options Added**:
  - `-o` / `--output=FILE` (redirects shuf output directly to a file)
  - `-r` / `--repeat` (allows repeated selection of input lines indefinitely or up to `-n`)
  - `--random-source=FILE` (uses bytes from FILE to seed/drive the shuf RNG)
  - `--random-seed=SEED` (uses a deterministic seed value to initialize shuf RNG)
  - `-z` / `--zero-terminated` (reads and writes NUL-delimited lines)
- **Tests**: Verification tests added in [shuf_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/shuf/shuf_test.go).

### 7. `unexpand`
- **Options Added**:
  - `-f` shorthand alias for `--first-only`
- **Tests**: Verification tests updated in [unexpand_test.go](file:///Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-40/cmds/unexpand/unexpand_test.go).

### 8. `wc`
- **Status**: Checked and verified that `-h` and `-V` aliases are safely processed via `tool.AliasHelpVersion(args)`.

---

## Deferred Options (Future Work)

Due to the size of the overall coreutils scope, options for the following packages are deferred:

- **`sort`**: `check-silent`, `dictionary`, `general`, `month`, `merge`, `random`, `version`, `zero`, `files0`, `temp`
- **`split`**: `suffix`, `filter`, `separator`, `verbose`
- **`tac`**: `before`, `regex`
- **`tr`**: `truncate-set1`
