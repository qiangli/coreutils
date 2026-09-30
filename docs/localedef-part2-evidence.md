# localedef part2 worker delivery evidence

Sprint #340, story #156 (`9f8ce58c4dd7`), worker `s340-localedef-astra`.
This records scoped worker evidence; the parent conductor independently gates
acceptance and integration. It does not claim full localedef conformance.

## Preserved candidate chain

Oldest to newest:

- `1a8b083388086a58a840a7c707d6d5ae8cf7d7b0`: Go locale store and compilation.
- `e0cddf47b440340f5e3b0e027d6865ff01cfdcc2`: locale applet reads compiled locales.
- `2c221abeed2e34f6bf64fd6e047fbb984b741d60`: numeric sort consumes the store.
- `9c78e6dbbfdb719f6d04b6fc6c6de957fa31b35f`: preserved interrupted run20 work.
- `c69da490b1e59c1855edd596f4e443e3398917f8`: monetary category resolution.
- `80b4b2d557a39b71ba3f4bb1ad275e365ce6a891`: compiled UTF-8 numeric precedence.
- `24b5dfcf3a1e0bdb2c438d756005b578de9d05bc`: story161 CLI and path corrections.

## Scoped evidence

Before correction, regression tests reproduced writes after validation errors
with `-c`, writes after unforced warnings, silent `-u` relabeling, and relative
store resolution against process cwd. Tests cover preserving an existing
artifact and avoiding a new artifact on rejection.

After correction, the following commands completed with exit 0 for
`./pkg/locale ./pkg/localedef ./cmds/localedef ./cmds/locale ./cmds/sort`:

- `go test` with `-count=1`.
- `go test -race` with `-count=1`.
- `CGO_ENABLED=0 go vet` under `GOOS=windows GOARCH=amd64`,
  `GOOS=linux GOARCH=amd64`, and `GOOS=darwin GOARCH=arm64`.

Store acceptance exercised compilation, reload/copy, `locale -a`, distinctive
`locale -k` numeric/messages values, and numeric sort consuming the compiled
radix and thousands separator. Relative LOCPATH uses `rc.Path` for writers and
readers. The inherited UTF-8 precedence test had an incorrect arithmetic
expectation (1.9 before 1.20); its replacement distinguishes compiled grouping
from fallback using 1000.5 versus 900. These are focused checks, not a full
repository or integrated merge gate.

## Remaining required interface work

Story #162 (`02d2bb56298e`) tracks required `-u` codeset mapping, pathname name
operands, and successful-category stdout reporting. Current `-u` returns an
explicit unsupported status 2 without changing output; that is honest refusal,
not implementation of the required option. Part3/collation and registration
changes are outside this delivery.

Behavior was derived from POSIX documentation and black-box tests only. No
GNU/glibc/CUPS implementation source was copied, translated or paraphrased.
The official Open Group URLs returned 403; the IEEE/Open Group POSIX.1-2017
text reproduction at <https://man7.org/linux/man-pages/man1/localedef.1p.html>
provided OPTIONS, EXTENDED DESCRIPTION and error/output rules.
