# Sprint #340, story #158: manifest evidence checkout

The lp follow-up is implemented, but the full manifest gate cannot pass in
this isolated workspace because the bashy shell-routing evidence checkout
is unavailable. The first diagnostic is:

```
alias: shell routing evidence is unavailable or unfocused
```

Three entry points confirmed the same blocker:

- `python3 scripts/posix_manifest.py`
- `python3 -m unittest scripts.posix_manifest_test` (58 tests run;
  32 failures and 7 errors, including subtest failures)
- `sh scripts/crossvet.sh` (stops at that Python suite)

The validator resolves bashy evidence from the sibling `bashy` checkout or
`POSIX_BASHY_EVIDENCE_ROOT`. No override is set and no routing evidence exists
inside this workspace. The assignment forbids leaving the clone or following
remotes; no external checkout was searched or fetched, and evidence validation
was not weakened. Provide the real checkout through the supported environment
override, then rerun the above gates and `python3 scripts/posix_manifest.py --check`.

The rendered interface document was regenerated with the existing
`read_manifest`, `render`, and `validate_rendered` functions; it exactly matches
the generator output. This is not a substitute for full evidence validation.
The lp row independently passes parser/option-argument coverage and all eight
Go evidence references resolve to existing tests.

Passing checks:

- `go build ./...`
- `go test ./cmds/lp/ ./cmds/posixgate/ ./pkg/posixprovider/ -count=1`
  (61 top-level tests: 8 lp, 33 posixgate, 20 posixprovider)
- `go vet ./cmds/lp/`
- `GOOS=windows go vet ./cmds/lp/`
- `GOOS=linux go vet ./cmds/lp/`
- `sh scripts/validate-posix-required-commands.sh` (116 names, 93/14/9)
- `python3 scripts/applet-matrix.py --check`

Red/green: all five TestWriteSubscription cases failed on the missing -w
option before implementation and pass afterward. Terminal delivery remains
delegated to the IPP server using the requested write marker.
