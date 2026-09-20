# Roadmap status (as of 2026-07)

Moved verbatim from `CLAUDE.md` on 2026-09-20 — historical; `docs/commands.md` + `cmds/all/all.go` are the live truth.

Done: git relocation; the `tool/` framework + Phase A userland (incl. the
2026-07 file-utilities batch: dd, install, shred, mkfifo, mknod, chcon,
dircolors, dir/vdir); the `shell/` adapter (`shell/Handler()` /
`HandlerFunc()` — an `interp.ExecHandler` middleware for `mvdan.cc/sh/v3`
that dispatches any argv[0] naming a registered `tool.Tool` to `Tool.Run`,
else falls through to PATH; precedence is **pure-Go first**, a host opts
out by not wiring it — bashy's AgentOS shell turns it on, the `bash`
drop-in leaves it off; the adapter imports sh, sh never imports
coreutils); the `cmd/coreutils` multicall binary (the `multicall/`
package factors out Resolve/Dispatch/Main so bashy reuses the same
argv[0] dispatch); and much of the former Phase C — sed, xargs, awk
(goawk), jq (gojq), time/timeout, watch, tree, and the agentic extras
(at/atq/atrm/batch/crontab, browser, fetch, clip, tokens, duration, tz,
ntp, cal, tsort).

Phase B is essentially complete (2026-07): expr, od, nl, fold,
expand/unexpand, cksum, b2sum, basenc, csplit, numfmt, nproc, arch,
tail -f (polling follow), plus the sh-utils sweep (who/users/pinky,
pr, ptx, factor, stdbuf, stty, hexdump, yes, which, …) all shipped.
Remaining (per docs/commands.md): printf, test/[. The 2026-07-07
**uutils option-parity sprint** then closed flag/option gaps against
`reference/uutils-coreutils` across the whole userland (ls, df, du,
ln, tail, sort, stat, checksums, …) — see
`docs/uutils-parity-sprint-2026-07-07.md` for what landed and
`docs/bashy-uutils-option-comparison.md` for the final per-command
gap status. Conformance is still judged against GNU/POSIX docs; the
uutils reference was parity guidance, never translated source.

The not-supported tier is docs/commands.md's **NO list** (canonical —
grouped by reason: needs-exec, unix-only machinery, low agent value,
sysadmin out-of-scope). Recognized-but-NO names get a clear error naming
the command, the reason, and the nearest alternative — never a silent
fallthrough. Note the list evolves: several early "NO ↻ revisit" entries
(timeout, time) and former skips (mkfifo, mknod, dircolors, chcon, tsort)
have since shipped — trust docs/commands.md + `cmds/all/all.go` over any
older skip list.

**Known defect — `env` breaks the never-a-silent-fallthrough rule (found
2026-08-05).** `env` correctly and loudly refuses to RUN a COMMAND (needs-exec,
NO list). But its option parsing does **not stop at the first operand** the way
POSIX/GNU `env` does, so it consumes the *command's* flags as its own:

```
env FOO=1 ls -l            -> env: unknown shorthand flag: 'l' in -l     (loud, fine)
env FOO=1 bashy --version  -> env (qiangli/coreutils) dev                (SILENT WRONG ANSWER)
```

The second is the serious one: `--version`/`--help` are recognised by `env`, so
it answers about *itself* while appearing to answer about the command. That is
exactly the "recognized-but-NO names get a clear error … never a silent
fallthrough" invariant, inverted. Fix: stop option parsing at the first
non-option operand, then report the needs-exec refusal naming the command.
Until then, do not use `env VAR=x <cmd> <flags>` in scripts or tests on a bashy
shell — it will either error confusingly or answer the wrong question. (It cost
a false bug report during the execlog work: `env -u X bashy -c '…'` never ran
bashy at all, and with stderr redirected that was indistinguishable from the
recorder failing to write.)

