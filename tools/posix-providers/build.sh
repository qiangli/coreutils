#!/bin/bash
# Build one POSIX external provider from pinned upstream source.
#
# Download and build are SEPARATE on purpose — see manifest.tsv. This script
# fetches UPSTREAM source (never a mirror of ours), verifies its pinned digest
# BEFORE extraction, builds locally, and installs the executable into the binmgr
# cache. Nothing built here may be republished: these providers are copyleft and
# this project is deliberately not a distributor of their binaries.
set -euo pipefail

here=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
manifest=${POSIX_PROVIDER_MANIFEST:-$here/manifest.tsv}
cache=${BASHY_BIN_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bashy/bin}
work=${POSIX_PROVIDER_WORK:-${TMPDIR:-/tmp}/posix-providers}

fail() { printf 'posix-provider: %s\n' "$*" >&2; exit 2; }
say()  { printf 'posix-provider: %s\n' "$*"; }

cmd=${1:?usage: build.sh <command> [--check]}
check_only=${2:-}

row=$(awk -F'\t' -v c="$cmd" '!/^#/ && $1 == c {print; exit}' "$manifest")
[ -n "$row" ] || fail "no manifest entry for $cmd"
version=$(printf '%s' "$row" | cut -f2)
license=$(printf '%s' "$row" | cut -f3)
sha=$(printf '%s' "$row" | cut -f4)
url=$(printf '%s' "$row" | cut -f5)
[ -n "$sha" ] && [ ${#sha} -eq 64 ] || fail "$cmd has no full sha256 pin; refusing to build"

dest=$cache/$cmd/$version
target=$dest/$cmd
if [ -x "$target" ]; then say "cached: $target ($license)"; printf '%s\n' "$target"; exit 0; fi
[ "$check_only" != "--check" ] || fail "$cmd $version is not provisioned (cache miss)"

need() { command -v "$1" >/dev/null 2>&1 || fail "required build tool not found: $1"; }
for t in curl tar make cc; do need "$t"; done

mkdir -p "$work" "$dest"
archive=$work/$(basename "$url")
if [ ! -f "$archive" ]; then
  say "fetching upstream $cmd $version ($license)"
  curl -sfL "$url" -o "$archive.part" || fail "download failed: $url"
  mv "$archive.part" "$archive"
fi

# Verify BEFORE extraction: a tampered archive must never reach tar.
actual=$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$archive" | awk '{print $1}';
         else shasum -a 256 "$archive" | awk '{print $1}'; fi)
[ "$actual" = "$sha" ] || { rm -f "$archive"; fail "$cmd digest mismatch: got $actual want $sha"; }
say "digest verified"

src=$work/src-$cmd-$version
rm -rf "$src"; mkdir -p "$src"
case "$archive" in
  *.tar.gz|*.tgz) tar xzf "$archive" -C "$src" --strip-components=1 ;;
  *.tar.xz)       tar xJf "$archive" -C "$src" --strip-components=1 ;;
  *.tar.lz)       command -v lzip >/dev/null 2>&1 || fail "lzip is required for $cmd"
                  lzip -dc "$archive" | tar xf - -C "$src" --strip-components=1 ;;
  *) fail "unsupported archive form: $archive" ;;
esac

build_dir=$work/build-$cmd-$version
rm -rf "$build_dir"; mkdir -p "$build_dir"
say "building $cmd $version"
case "$cmd" in
  ar|nm|strip)
    (cd "$build_dir" && "$src/configure" --disable-nls --disable-werror >/dev/null &&
       make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" >/dev/null)
    found=$(find "$build_dir/binutils" -maxdepth 1 -type f -name "$cmd-new" -o -maxdepth 1 -type f -name "$cmd" 2>/dev/null | head -1)
    ;;
  ex|vi)
    (cd "$src" && ./configure --with-features=normal --disable-gui --without-x \
       --disable-nls --disable-netbeans >/dev/null && make -j2 >/dev/null)
    found=$src/src/vim
    ;;
  *)
    (cd "$build_dir" && "$src/configure" --disable-nls >/dev/null 2>&1 || "$src/configure" >/dev/null)
    (cd "$build_dir" && make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" >/dev/null)
    found=$(find "$build_dir" -maxdepth 3 -type f -perm -111 -name "$cmd" 2>/dev/null | head -1)
    ;;
esac
[ -n "${found:-}" ] && [ -f "$found" ] || fail "$cmd build produced no executable"

install -m 0755 "$found" "$target"
say "PASS $cmd $version -> $target ($license, built locally from pinned upstream source)"
printf '%s\n' "$target"
