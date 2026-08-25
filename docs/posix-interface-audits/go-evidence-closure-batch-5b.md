# POSIX Go evidence closure: wave 5B

Wave 5B covers the Go-owned `du`, `file`, and `find` interfaces against
POSIX.1-2016 Issue 7. All three rows move from `unverified` to `partial`.
The independent review deliberately separates normative requirements from
Bashy's deterministic or upstream-compatible choices.

| Command | Evidence closed | Remaining boundary |
| --- | --- | --- |
| `du` | Default `.` hierarchy, 512-byte and `-k` 1024-byte rounded units, exact record shape, `-s`, same-device `-x`, and default symlink handling. | POSIX does not prescribe record order, so the Issue 7 assertion is order-independent. A real cross-device mount, hard-link deduplication product, locale diagnostics, and output-failure paths remain. Non-POSIX `-A`/`-b` appear only as deterministic fixture controls. |
| `file` | Required operand arity, operand-order processing, one stdout record per operand, the explicit inaccessible/undetermined-operand status exception, continued processing, and Bashy's stdin choice for `-`. | Exact type labels, silence on stderr for inaccessible operands, status 2 for usage, and interpreting `-` as stdin are Bashy implementation choices rather than Issue 7 mandates. Locale/magic-database breadth remains. The source change is explanatory only; no functional defect was found. |
| `find` | `!`/implicit `-a`/`-o` precedence, leading-period pattern behavior, numeric `+n`/`n`/`-n`, leading-only `-H`/`-L`, `--`, and the Unix `-nouser` positive seam. | Issue 7 requires one or more path operands; Bashy's omitted-path `.` default is an extension. Declining `-ok` makes the primary false but does not fail the utility. Remaining primaries, locale collation/affirmative responses, real ownership databases, filesystem-error products, and execution side effects remain covered only by broader tests or residual analysis. |

Focused count-10 and race tests pass for all three packages. The Unix
ownership seam is isolated in `issue7_unix_test.go`; Windows vet passes. The
manifest evidence link and applet matrix are regenerated after that split so
the committed inventory points to the test that actually exists.
