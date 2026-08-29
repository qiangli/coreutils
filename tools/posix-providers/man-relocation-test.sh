#!/bin/sh
# Uniform POSIX contract for tracked workflow scripts.
POSIXLY_CORRECT=1
export POSIXLY_CORRECT
if (set -o posix) 2>/dev/null; then set -o posix; fi
# Build the pinned man provider and prove its self-manual survives relocation.
set -eu

here=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
tmp=${POSIX_PROVIDER_MAN_TEST_TMP:-}
owned_tmp=0
if [ -z "$tmp" ]; then
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/posix-man-relocation.XXXXXX")
  owned_tmp=1
fi
cleanup() {
  [ "$owned_tmp" -eq 0 ] || rm -rf "$tmp"
}
trap cleanup 0 1 2 15

cache=$tmp/cache
work=$tmp/work
POSIX_PROVIDER_JOBS=${POSIX_PROVIDER_JOBS:-1}
export POSIX_PROVIDER_JOBS
BASHY_BIN_CACHE=$cache POSIX_PROVIDER_WORK=$work "$here/build.sh" man >/dev/null

version=$(awk -F '\t' '$1 == "man" { print $2; exit }' "$here/../../pkg/posixprovider/manifest.tsv")
[ -n "$version" ]
source_dir=$cache/man/$version

# The resolver tells an operator to rebuild after corruption; prove the build
# cache does not mistake an intact executable for a healthy whole provider.
printf 'tampered\n' >>"$source_dir/share/man/man1/man.1"
BASHY_BIN_CACHE=$cache POSIX_PROVIDER_WORK=$work "$here/build.sh" man >/dev/null

relocated_cache=$tmp/relocated-cache
relocated=$relocated_cache/man/$version
mkdir -p "$relocated"
cp -R "$source_dir/." "$relocated/"

page=$relocated/share/man/man1/man.1
[ -s "$page" ]
want=$(awk -F '\t' '$1 == "manual_man1_sha256" { print $2; exit }' "$relocated/provenance.tsv")
case "$want" in ''|*[!0-9a-f]* ) exit 1 ;; esac
[ ${#want} -eq 64 ]
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$page" | awk '{print $1}')
else
  got=$(shasum -a 256 "$page" | awk '{print $1}')
fi
[ "$got" = "$want" ]

# Give the relocated page a test-only identity and update its copied
# provenance. The final rendered marker cannot come from a host man(1), so the
# probe fails if the real adapter forgets to publish this exact cache root.
marker=BASHY_RELOCATED_MAN_PROVIDER_PROBE
{
  printf '\n.SH PROVIDER_RELOCATION_PROBE\n'
  printf '%s\n' "$marker"
} >>"$page"
if command -v sha256sum >/dev/null 2>&1; then
  relocated_sha=$(sha256sum "$page" | awk '{print $1}')
else
  relocated_sha=$(shasum -a 256 "$page" | awk '{print $1}')
fi
awk -F '\t' -v digest="$relocated_sha" \
  'BEGIN { OFS="\t" } $1 == "manual_man1_sha256" { $2=digest } { print }' \
  "$relocated/provenance.tsv" >"$relocated/provenance.tsv.tmp"
mv "$relocated/provenance.tsv.tmp" "$relocated/provenance.tsv"

# Exercise the real registered adapter without compiling the full multicall
# inventory. The certification binary imports every applet, but this probe only
# needs the provider package and tool registry; keeping that boundary narrow
# avoids unrelated compiler memory dominating a provider relocation check.
probe=$tmp/man-adapter-probe.go
cat >"$probe" <<'GO'
package main

import (
	"context"
	"os"

	_ "github.com/qiangli/coreutils/cmds/posixproviders"
	"github.com/qiangli/coreutils/tool"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, Env: os.Environ(),
		InvocationName: "man", DirIsProcessCwd: true, DedicatedProcess: true,
		FS: tool.NewLocalFS(),
		Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
	}
	tl := tool.Lookup("man")
	if tl == nil {
		_, _ = os.Stderr.WriteString("registered man provider is unavailable\n")
		os.Exit(127)
	}
	os.Exit(tl.Run(rc, []string{"--", "man"}))
}
GO

(
  unset MANPATH BASHY_POSIX_PROVIDERS
  cd "$here/../.."
  BASHY_BIN_CACHE=$relocated_cache MANPAGER=cat PAGER=cat LC_ALL=C \
    go run "$probe"
) >"$tmp/man.out" 2>"$tmp/man.err"
[ -s "$tmp/man.out" ]
[ ! -s "$tmp/man.err" ]
grep -Fq "$marker" "$tmp/man.out"

printf 'PASS man provider self-manual after cache relocation\n'
