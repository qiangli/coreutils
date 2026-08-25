# `date` POSIX.1-2008 Issue 7 Audit

## 1. Required Conversions and E/O Modifiers
All mandatory POSIX Issue 7 `strftime` conversions are supported:
`%a %A %b %B %c %C %d %D %e %F %g %G %h %H %I %j %m %M %n %p %r %R %S %t %T %u %U %V %w %W %x %X %y %Y %z %Z %%`

Alternative E/O modifiers (`%Ec %EC %Ex %EX %Ey %EY %Od %Oe %OH %OI %Om %OM %OS %Ou %OU %OV %Ow %OW %Oy`) are supported and fall back to their unmodified representations in the C/POSIX locale, per POSIX XBD 8.3. Unknown/literal percent sequences pass through literally (as GNU date does).

## 2. Default Format
The default format is `+%a %b %e %H:%M:%S %Z %Y`, matching the POSIX requirement.

## 3. Options: `-u` and `TZ`
The `-u` flag correctly overrides the timezone to UTC. The `TZ` environment variable acts as the location provider for parsing, formatting, and the XSI clock-setter. Unset/null `TZ` appropriately falls back to the system default, bypassing the glibc-shaped generic resolver's UTC fallback.

## 4. LC_TIME, LC_CTYPE, and LC_MESSAGES
*   **LC_TIME**: Supported via hermetic C/POSIX and `de_DE` representations. Locale environment variables (`LC_ALL`, `LC_TIME`, `LANG`) follow correct POSIX precedence.
*   **LC_CTYPE**: Formats are processed byte-by-byte, seamlessly passing through multi-byte UTF-8 without querying `LC_CTYPE`. This is a residual since encoding conversions are not performed.
*   **LC_MESSAGES**: Diagnostic messages are emitted in English and `LC_MESSAGES` is not consulted (no message catalogs are used). This is stated as a runtime residual.

## 5. Two-Digit / Current-Year Rules
The XSI operand `mmddhhmm[[cc]yy]` supports the Issue 7 year century inferences:
*   69-99 maps to 1969-1999.
*   00-68 maps to 2000-2068.
*   Missing year maps to the current year in the target timezone.

## 6. Leap-Second Validation / Rendering
**Residual**: POSIX allows `%S` to render `60` for a leap second. Go's `time` package does not retain leap seconds; it normalizes them to the next minute (e.g. `00` seconds) or rejects them during parsing. Therefore, `date` cannot render or set a leap second.

## 7. Validation-Before-Mutation
The XSI setter performs comprehensive range checks (e.g. valid month, day, exact calendar representation matching) prior to invoking the platform clock setter. Invalid permutations fail immediately before attempting host mutation.

## 8. Platform Clock-Set Provider & Error Routing
Hermetic `setSystemClock` interfaces wrap `unix.Settimeofday`. OS specific sizes for `unix.Timeval` fields (e.g., `int32` vs `int64`) are correctly separated across platforms. Failures from the setter are written to standard error and propagated with an exit status of 1.

## 9. Stdout Errors / Short Writes
Standard output writes explicitly check for short writes and errors (e.g. `fmt.Fprint(rc.Out, ...)`), routing failures to `stderr` and returning a non-zero exit code as mandated by POSIX General Assertion 39.

## 10. Usage and Statuses
Invalid invocations write a diagnostic message to `stderr` and exit with a non-zero status code (>0). Successfully completed operations exit with status `0`.
