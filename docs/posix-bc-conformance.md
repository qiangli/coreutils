# POSIX Issue 7 `bc` conformance checklist

Normative reference: The Open Group Base Specifications Issue 7, 2018
edition, [`bc`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/bc.html).
GNU bc 1.07.1 and Gavin D. Howard's BSD-2-Clause `bc` are differential
oracles only. No GNU/GPL source is used. The Go implementation is intentionally
not registered while `bc` remains external-provider-owned.

| Clause | Required surface | Focused evidence |
|---|---|---|
| Synopsis/options | `bc [-l] [file...]`; Utility Syntax Guidelines | `TestPOSIXOptionAndOperandContract` |
| Input order | all files in order, then standard input; shared state | `TestFilesPrecedeStandardInputAndShareState`, `TestMissingInputStopsProcessing` |
| Lexical conventions | comments, strings, blanks, newlines, backslash-newline, numbers, longest token | `TestPOSIXLexicalConventionsAndLimits` |
| Grammar | statement lists, blocks, functions, `if`/`while`/`for`, `break`, `return`, `quit`, auto lists | `TestPOSIXStatementsAndUnbracedControl`, `TestPOSIXQuitIsLexical`, `TestPOSIXFunctionsArraysAndDynamicLocals` |
| Identifiers/arrays | scalar/array/function namespaces, zero initialization, truncating indices, 2048 minimum elements | `TestPOSIXArraysCopyByValueAndIndexLimits` |
| Registers | `scale` 0..99, `ibase` 2..16, `obase` 2..99, integer truncation, hexadecimal single-digit base assignment | `TestPOSIXRegisterAssignmentAndBases` |
| Decimal arithmetic | observable scale rules for constants, unary, add/subtract, multiply, divide, remainder, power, increment/decrement | `TestPOSIXArithmeticScaleTable` |
| Built-ins | `length`, `scale`, `sqrt` | `TestPOSIXBuiltins` |
| Functions | definitions before use, recursion, invocation-time `ibase`, dynamic scalar/array locals, pass-by-value | `TestPOSIXFunctionsArraysAndDynamicLocals` |
| Output | non-assignment expressions, literal strings, radix digits, bases >16, zero/proper fractions, 70-character continued lines | `TestPOSIXOutputBasesAndLineFolding` |
| Math library | `-l` initializes scale 20 and defines `s`, `c`, `a`, `l`, `e`, and `j`; result scale preserved | `TestPOSIXMathLibraryAllFunctions` |
| Errors/status | diagnostics only on stderr; inaccessible operand terminates; invalid non-interactive input fails | `TestDiagnosticsAndExitStatus`, `TestMissingInputStopsProcessing` |
| Environment/locale | `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`; radix remains period | `TestPOSIXLocaleIndependentRadix` |
| Streaming/interactive | execute complete input items in order; flush/recover on terminal input; `quit` stops at its lexical position | `TestPOSIXStreamingBeforeError`, `TestPOSIXInteractiveExecutionRecoveryAndFlush`, `TestPOSIXQuitIsLexical` |

The locale variables do not change numeric syntax or output: the repository's
documented deterministic `LC_ALL=C` agent contract is a conforming POSIX-locale
execution. Localized diagnostics are not required in the POSIX locale.
