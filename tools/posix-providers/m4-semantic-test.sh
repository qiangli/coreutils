#!/bin/sh
# Public executable regression for Bashy's locally built GNU m4 provider.
# Takes the uninstalled candidate pathname; never resolves a host m4 from PATH.
set -u

m4=${1:?usage: m4-semantic-test.sh /path/to/built/m4}
[ -x "$m4" ] || { printf 'm4 semantic test: not executable: %s\n' "$m4" >&2; exit 2; }

work=$(mktemp -d "${TMPDIR:-/tmp}/bashy-m4-semantic.XXXXXX") || exit 2
watchdog=
cleanup() {
  [ -z "$watchdog" ] || kill "$watchdog" 2>/dev/null || :
  rm -rf "$work"
}
trap cleanup 0
trap 'exit 1' 1 2 15
fail() { printf 'm4 semantic test: %s\n' "$*" >&2; exit 1; }
flags='--traditional --fatal-warnings'

printf 'eval(48879,16)\n' | "$m4" $flags >"$work/eval.out" || fail 'eval failed'
[ "$(cat "$work/eval.out")" = BEEF ] || fail 'eval did not use POSIX uppercase digits'

cat >"$work/changequote.in" <<'EOF'
define(item,VALUE)dnl
changequote(<,>)dnl
<item> item
changequote()dnl
`item' item
EOF
cat >"$work/changequote.want" <<'EOF'
item VALUE
item VALUE
EOF
"$m4" $flags "$work/changequote.in" >"$work/changequote.out" || fail 'changequote failed'
cmp -s "$work/changequote.out" "$work/changequote.want" || fail 'changequote() did not restore defaults'

printf 'm4wrap(`first\n'\'')m4wrap(`second\n'\'')dnl\n' | "$m4" $flags >"$work/wrap.out" ||
  fail 'm4wrap failed'
printf 'first\nsecond\n' >"$work/wrap.want"
cmp -s "$work/wrap.out" "$work/wrap.want" || fail 'repeated m4wrap was not FIFO'

if "$m4" $flags -D3invalid=value </dev/null >"$work/name.out" 2>"$work/name.err"; then
  fail 'invalid -D macro name succeeded'
fi
[ -s "$work/name.err" ] || fail 'invalid -D macro name had no diagnostic'
"$m4" $flags -D_valid9=value -U_valid9 </dev/null >"$work/name-ok.out" 2>"$work/name-ok.err" ||
  fail 'valid -D/-U macro name failed'
[ ! -s "$work/name-ok.err" ] || fail 'valid -D/-U macro name had a diagnostic'

if printf 'eval(item[0])\n' | "$m4" $flags >"$work/expression.out" 2>"$work/expression.err"; then
  fail 'invalid expression succeeded'
fi
[ -s "$work/expression.err" ] || fail 'invalid expression had no diagnostic'

if printf 'mkstemp(%s/not-present/fXXXXXX)dnl\n' "$work" |
    "$m4" $flags >"$work/mkstemp.out" 2>"$work/mkstemp.err"; then
  fail 'failed mkstemp returned success'
fi
[ ! -s "$work/mkstemp.out" ] || fail 'failed mkstemp produced defining text'
[ -s "$work/mkstemp.err" ] || fail 'failed mkstemp had no diagnostic'

LC_ALL=C "$m4" --help >"$work/help.out" || fail '--help failed'
grep 'nesting-limit=.*\[0\]$' "$work/help.out" >/dev/null ||
  fail 'provider did not preserve upstream effective unlimited nesting setting'

case $(uname -s) in
  Linux)  signal_pairs='ABRT:6 BUS:7 FPE:8 ILL:4 SEGV:11' ;;
  Darwin) signal_pairs='ABRT:6 BUS:10 FPE:8 ILL:4 SEGV:11' ;;
  *)      signal_pairs='' ;;
esac

if [ -n "$signal_pairs" ]; then
  ulimit -c 0 2>/dev/null || :
  fifo=$work/signal.in
  mkfifo "$fifo" || fail 'cannot create signal input FIFO'
  exec 3<>"$fifo"
  for pair in $signal_pairs; do
    sig=${pair%:*}
    number=${pair#*:}
    "$m4" $flags <"$fifo" >/dev/null 2>"$work/default-$sig.err" &
    pid=$!
    (sleep 4; kill -KILL "$pid" 2>/dev/null || :) &
    watchdog=$!
    sleep 1
    kill -"$sig" "$pid" || fail "cannot send $sig"
    wait "$pid"
    status=$?
    kill "$watchdog" 2>/dev/null || :
    wait "$watchdog" 2>/dev/null || :
    watchdog=
    [ "$status" -eq $((128 + number)) ] ||
      fail "default $sig status $status, want $((128 + number))"
  done

  for pair in $signal_pairs; do
    sig=${pair%:*}
    number=${pair#*:}
    (trap '' "$number"; exec "$m4" $flags <"$fifo" >/dev/null 2>"$work/ignore-$sig.err") &
    pid=$!
    (sleep 4; kill -KILL "$pid" 2>/dev/null || :) &
    watchdog=$!
    sleep 1
    kill -"$sig" "$pid" || fail "cannot send ignored $sig"
    sleep 1
    kill -0 "$pid" || fail "inherited SIG_IGN for $sig was not preserved"
    kill -TERM "$pid" 2>/dev/null || :
    wait "$pid" 2>/dev/null || :
    kill "$watchdog" 2>/dev/null || :
    wait "$watchdog" 2>/dev/null || :
    watchdog=
  done
fi

printf '%s\n' 'm4-semantic-test: PASS'
