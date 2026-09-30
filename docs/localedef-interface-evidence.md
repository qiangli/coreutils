# localedef codeset and pathname worker evidence

Sprint #340, story #162 (`02d2bb56298e`), worker `s340-codeset-astra`,
weave run 26, starting from frozen `7aa286e9`. Conductor message mb:2549
released this work with part3 source idle. No merge, push or installation.

The implementation follows the POSIX.1-2017 localedef OPTIONS, OPERANDS,
STDOUT and EXTENDED DESCRIPTION text reproduced at
<https://man7.org/linux/man-pages/man1/localedef.1p.html>. Official Open Group
requests returned HTTP 403. No upstream implementation code was used.
Supported aliases, UCS syntax, target metadata, pathname storage and remaining
collation limits are documented in [localedef-parser.md](localedef-parser.md).
This delivery implements the bounded interface story, not full POSIX localedef.

## Red and green

Before production changes, the new CLI cases failed for every supported
ASCII/UTF-8 target (blanket status 2), and both relative and absolute output
paths failed as invalid locale names. Log: `/tmp/issue26-red.log`.

Green coverage includes ASCII, non-ASCII and supplementary UTF-8 mapping;
ASCII rejection of non-ASCII positions/bytes; surrogate and out-of-range
rejection; aliases; UCS ranges crossing UTF-8 length boundaries; mixed numeric
and UCS encodings; final repertoire checks for literal and copied values;
exact pathname creation without LOCPATH/HOME; relative rc.Dir selection and
copy; locale keyword readback; numeric sort and compiled collation sort;
processed-category stdout; preservation of existing output on errors and
unforced warnings; forced-warning output; moved/private artifact lookup and
malformed artifact detection. Existing public-store and host fallback tests
remain green.

## Executed gates

All Go invocations used `GOFLAGS=-p=1 GOMAXPROCS=2`. The configured run-26 cache
was outside sandbox write roots and could not be created; these gates used the
isolated writable `GOCACHE=/private/tmp/coreutils-issue26-cache` instead.

For `./cmds/localedef ./pkg/localedef ./pkg/locale ./pkg/collate ./cmds/locale
./cmds/sort`:

- Normal tests passed in all six packages: `/tmp/issue26-normal.log`.
- `go test -race -count=1` passed in all six packages:
  `/tmp/issue26-race.log`.
- `CGO_ENABLED=0 go vet` passed for windows/amd64, linux/amd64 and
  darwin/arm64: `/tmp/issue26-platform.log`. These are cross-platform static
  and compilation checks, not execution on Windows or Linux.
- Final test-only additions to localedef (mixed encodings, copy rejection,
  category-report assertion) passed normal and race runs of both localedef
  packages: `/tmp/issue26-final-normal.log`, `/tmp/issue26-final-tests.log`.
- `git diff --check` passed.

The optional external-provider differential and host locale-source tests
skipped because those fixtures/providers were unavailable. No external
mapping parity or full-tree gate is claimed. Required collation grammar and
multicharacter bracket work remains with the separately tracked part3 stories.
