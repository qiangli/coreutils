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
# The manifest is CANONICAL at pkg/posixprovider/manifest.tsv, because the Go
# library embeds it (//go:embed cannot reach outside its own package directory)
# and exactly one copy of the pins may exist. This script reads that same file so
# the recipe and the resolver can never disagree about what is pinned.
manifest=${POSIX_PROVIDER_MANIFEST:-$here/../../pkg/posixprovider/manifest.tsv}
# The cache root MUST agree with pkg/binmgr.CacheDir(), which is what
# pkg/posixprovider.Resolve reads. A provider built into a directory the
# resolver does not look in is invisible, and the symptom is a "not
# provisioned" error nobody can explain. binmgr uses Go's os.UserCacheDir(),
# and that is ~/Library/Caches on darwin -- NOT the XDG path.
default_cache() {
  case "$(uname -s)" in
    Darwin) printf '%s' "$HOME/Library/Caches/bashy/bin" ;;
    MINGW*|MSYS*|CYGWIN*) printf '%s' "${LOCALAPPDATA:-$HOME/AppData/Local}/bashy/bin" ;;
    *) printf '%s' "${XDG_CACHE_HOME:-$HOME/.cache}/bashy/bin" ;;
  esac
}
cache=${BASHY_BIN_CACHE:-$(default_cache)}
work=${POSIX_PROVIDER_WORK:-${TMPDIR:-/tmp}/posix-providers}

fail() { printf 'posix-provider: %s\n' "$*" >&2; exit 2; }
say()  { printf 'posix-provider: %s\n' "$*"; }

cmd=${1:?usage: build.sh <command> [--check]}
check_only=${2:-}

row=$(awk -F'\t' -v c="$cmd" '!/^#/ && $1 == c {print; exit}' "$manifest")
[ -n "$row" ] || fail "no manifest entry for $cmd"
version=$(printf '%s' "$row" | cut -f2)
license=$(printf '%s' "$row" | cut -f3)
platforms=$(printf '%s' "$row" | cut -f4)
sha=$(printf '%s' "$row" | cut -f5)
url=$(printf '%s' "$row" | cut -f6)

# Refuse an undeclared platform rather than attempting it: a clear refusal beats
# an obscure configure failure three minutes in.
host_os=$(uname -s | tr 'A-Z' 'a-z')
case "$host_os" in darwin) host_os=darwin ;; linux) host_os=linux ;; mingw*|msys*|cygwin*) host_os=windows ;; esac
case ",$platforms," in
  *",$host_os,"*) ;;
  *) fail "$cmd is not declared for $host_os (declared: $platforms)" ;;
esac
[ -n "$sha" ] && [ ${#sha} -eq 64 ] || fail "$cmd has no full sha256 pin; refusing to build"

dest=$cache/$cmd/$version
target=$dest/$cmd
if [ -x "$target" ]; then say "cached: $target ($license)"; printf '%s\n' "$target"; exit 0; fi
[ "$check_only" != "--check" ] || fail "$cmd $version is not provisioned (cache miss)"

need() { command -v "$1" >/dev/null 2>&1 || fail "required build tool not found: $1"; }
for t in curl tar make; do need "$t"; done

# Compiler selection, most explicit first. Whichever is chosen is RECORDED next
# to the binary: a provider built by an unrecorded compiler cannot be attributed
# to a known input, and certification evidence turns on exactly that.
#
#   POSIX_PROVIDER_CC   explicit override, e.g. "gcc-14"
#   zig cc              bashy's own pinned toolchain (external/zigcc) - the
#                       portable path, present on hosts with no compiler
#   host cc             correct for a certification host, which already pins
#                       gcc-14 and source-builds GNU Bash 5.3 / Coreutils 9.11
#
# zig cc IS clang - Zig bundles the real LLVM frontend and backend rather than
# reimplementing them - so codegen is upstream clang's. The residual risk is
# driver-flag compatibility with autotools, which is why the choice is recorded
# per provider rather than assumed uniform. Measured: GNU make 4.3 configures
# and builds cleanly under zig cc, as do CUPS 2.4.7 (lp) and s-nail 14.9.25
# (mailx), both also run-tested on darwin; binutils and vim are not yet verified.
#
# "zig cc" is TWO WORDS, and that is a trap. Autotools tolerates a $CC with a
# space; a build system that treats $CC as a single word does not, and the
# failure surfaces nowhere near the cause -- s-nail's configuration step quietly
# produces nothing and the compile then dies on undeclared su_ERR_* symbols
# hundreds of lines later. Measured both ways on the same zig: wrapped, s-nail
# builds; unwrapped, the identical toolchain fails. So the two-word form is
# collapsed into one executable word, and CC_LABEL keeps the provenance record
# naming the real compiler rather than the wrapper path.
# The label is written beside the wrapper rather than returned: select_cc runs
# in a command substitution, so anything it assigns is lost with the subshell.
zig_wrapper() {
  mkdir -p "$work"
  printf '#!/bin/sh\nexec %s cc "$@"\n' "$1" > "$work/zig-cc"
  chmod 0755 "$work/zig-cc"
  printf '%s cc\n' "$1" > "$work/zig-cc.label"
  printf '%s' "$work/zig-cc"
}
select_cc() {
  if [ -n "${POSIX_PROVIDER_CC:-}" ]; then printf '%s' "$POSIX_PROVIDER_CC"; return; fi
  if [ -n "${ZIG:-}" ] && [ -x "${ZIG}" ]; then zig_wrapper "$ZIG"; return; fi
  if command -v zig >/dev/null 2>&1; then zig_wrapper "$(command -v zig)"; return; fi
  command -v cc >/dev/null 2>&1 || fail "no C compiler: set POSIX_PROVIDER_CC, provide zig, or install cc"
  printf '%s' "$(command -v cc)"
}
CC=$(select_cc)
if [ "$CC" = "$work/zig-cc" ] && [ -f "$work/zig-cc.label" ]; then
  CC_LABEL=$(cat "$work/zig-cc.label")
else
  CC_LABEL=$CC
fi
export CC
say "compiler: $CC_LABEL"

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

# Some providers need MORE than one pinned input. fetch_verified applies the
# same rule to each of them: download, hash, compare, and only then let the
# bytes be used. A secondary input with no digest is refused exactly like a
# primary one -- otherwise the weakest link decides the provenance.
fetch_verified() { # url sha256 dest
  _u=$1; _s=$2; _d=$3
  [ ${#_s} -eq 64 ] || fail "secondary input $_u has no full sha256 pin"
  if [ ! -f "$_d" ]; then
    curl -sfL "$_u" -o "$_d.part" || fail "download failed: $_u"
    mv "$_d.part" "$_d"
  fi
  _a=$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$_d" | awk '{print $1}';
       else shasum -a 256 "$_d" | awk '{print $1}'; fi)
  [ "$_a" = "$_s" ] || { rm -f "$_d"; fail "digest mismatch for $_u: got $_a want $_s"; }
}

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
  lp)
    # CUPS has no VPATH support, so build in-tree like vim. Only libcups and the
    # systemv clients are built: the scheduler, filters and backends are a
    # printing SYSTEM, and none of them is the POSIX lp(1) under test.
    # --disable-shared makes the client self-contained, so the cached binary does
    # not depend on a libcups.so that was never installed.
    #
    # LIBZ= is not a downgrade, it removes a DUPLICATE. Upstream's zlib probe
    # sets both LIBZ=-lz and LIBS+=-lz, so -lz reaches the link line twice on
    # every platform; COMMONLIBS still supplies it. Apple's ld dedupes with a
    # warning, lld does not, and the resulting Mach-O carries two LC_LOAD_DYLIB
    # entries for libz that dyld refuses at load: the binary links, then aborts
    # the first time it runs. Measured on darwin: without LIBZ=, an lp built by
    # zig cc dies with "duplicate linked dylib"; with it, the same toolchain
    # produces a working lp. When zlib is absent LIBZ is empty anyway, so the
    # override is a no-op rather than a lost dependency.
    (cd "$src" && ./configure --disable-shared --disable-dbus --disable-pam \
       --disable-libusb --without-rcdir --without-systemd >/dev/null &&
       make -C cups libcups.a >/dev/null &&
       make -C systemv lp LIBZ= >/dev/null)
    found=$src/systemv/lp
    ;;
  mailx)
    # s-nail is not autotools: its own make.rc/mk scripts configure it, so the
    # knobs are make variables rather than configure flags.
    #   VAL_SID= VAL_MAILX=mailx  name the binary "mailx" instead of "s-nail",
    #                             which is also what the found= probe expects.
    #   OPT_AUTOCC=no             keep $CC as selected above. Without it s-nail
    #                             picks and tunes a compiler itself, and the
    #                             provenance line would then name a compiler
    #                             that did not build the binary.
    #   OPT_NET=no                POSIX mailx has no network features; disabling
    #                             them drops the OpenSSL/GSSAPI probes, so the
    #                             build no longer varies with what headers the
    #                             host happens to have. Determinism, not size.
    #   OPT_DOTLOCK=no            that helper is installed setuid root; this
    #                             recipe installs one unprivileged binary into a
    #                             user cache and never runs as root.
    (cd "$src" && make VAL_SID= VAL_MAILX=mailx OPT_AUTOCC=no OPT_NET=no \
       OPT_DOTLOCK=no config >/dev/null &&
       make VAL_SID= VAL_MAILX=mailx OPT_AUTOCC=no OPT_NET=no \
       OPT_DOTLOCK=no build >/dev/null)
    found=$src/.obj/mailx
    ;;
  localedef)
    # THE HONEST SHAPE OF THIS ONE: localedef has no standalone upstream build.
    # It lives in glibc's locale/programs, and glibc/locale has no configure of
    # its own -- only a subdir Makefile the top-level build includes -- so
    # upstream's answer is "build all of glibc". What makes a single-binary build
    # possible is a build harness plus a patch set that neither upstream ships:
    #
    #   glibc 2.39            the SOURCES (pinned in the manifest row, the same
    #                         release Ubuntu 24.04's libc-bin is built from)
    #   kraj/localedef        a ~14-file autotools harness that compiles
    #                         locale/programs against the HOST libc instead of a
    #                         glibc under construction. No releases, no tags, so
    #                         it is pinned by commit.
    #   4 OpenEmbedded patches  the load-bearing one is 0013, which forward-ports
    #                         eglibc's cross-locale support; without it
    #                         locale/localeinfo.h pulls glibc-internal <tls.h>
    #                         and the build stops. 0001/0002 add the hardlink
    #                         resolver the harness links against. All four are
    #                         Upstream-Status: Pending -- carried out of tree
    #                         since 2015, which is the real maintenance risk here.
    #
    # This is Yocto's cross-localedef-native path, and the version pairing is not
    # optional: the harness commit and the glibc release must match, because the
    # patches carry file-level context. Re-pin all three together or not at all.
    #
    # Why it is worth the machinery: a localedef from a DIFFERENT glibc than the
    # host's writes locale data the host's libc may mis-read. Measured on
    # ubuntu:24.04 (glibc 2.39): the binary built here produces LC_CTYPE,
    # LC_COLLATE, LC_TIME, LC_NUMERIC and LC_MONETARY that are BYTE-IDENTICAL to
    # the host localedef's output for the same source, and the host libc loads
    # them. That equality is the whole reason to pin 2.39 rather than "latest".
    need patch
    _h=$work/ldef-$version
    _pd=$work/ldef-patches-$version
    rm -rf "$_h"; mkdir -p "$_h" "$_pd"
    fetch_verified \
      https://codeload.github.com/kraj/localedef/tar.gz/cba02c503d7c853a38ccfb83c57e343ca5ecd7e5 \
      358b131aceb9c4b7914598242bfa34120ad547acfca4799e5392a62a905db901 \
      "$_pd/localedef-harness.tar.gz"
    tar xzf "$_pd/localedef-harness.tar.gz" -C "$_h" --strip-components=1

    _oe=543550522f831479f07d332a40ba343c53ae1065
    _oebase=https://raw.githubusercontent.com/openembedded/openembedded-core/$_oe/meta/recipes-core/glibc/glibc
    while read -r _pf _psha; do
      [ -n "$_pf" ] || continue
      fetch_verified "$_oebase/$_pf" "$_psha" "$_pd/$_pf"
      (cd "$src" && patch -p1 --forward --silent < "$_pd/$_pf") ||
        fail "localedef: patch $_pf did not apply to glibc $version"
    done <<'PATCHES'
0001-localedef-Add-hardlink-resolver-from-util-linux.patch e77e2e527674ab7cadecbe503388e718dc6e4e95c925627b5214c9f5ce6aeb92
0002-localedef-fix-ups-hardlink-to-make-it-compile.patch 9faa11e6e2d386f2dd945e63f29a1b77c20a211e0a12ff42b0c9d483b19345d3
0013-eglibc-Forward-port-cross-locale-generation-support.patch cbe2e5e881af37de43015632753327de631ee2a5763a9cb9ba36f85305a9e7ef
0014-localedef-add-to-archive-uses-a-hard-coded-locale-pa.patch 6d7ff33fc45d1fc7225c67f0a1a0a329dcf87b8f7c2e75d0bfc36e1585bc1f55
PATCHES

    # The harness ships an eglibc-era config.sub that does not know aarch64 and
    # aborts configure with "machine not recognized". glibc's own scripts/ has
    # current copies, from the same pinned tarball, so no extra input is needed.
    cp "$src/scripts/config.sub" "$src/scripts/config.guess" "$_h/"

    # -DIS_IN(x)=0 is what tells glibc's sources they are NOT being compiled into
    # libc; without it intl/l10nflist.c fails on "#if IS_IN (libc)". The parens
    # are escaped because make hands this string to the shell, and the escaping
    # cannot move to configure time -- an unescaped define there fails the
    # compiler sanity check and configure exits 77.
    (cd "$_h" && ./configure --with-glibc="$src" >/dev/null &&
       make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" \
         CFLAGS='-O2 -fgnu89-inline -std=gnu99 -DIS_IN\(x\)=0' >/dev/null)
    found=$_h/localedef
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

# Provenance sidecar: what was built, from which pinned bytes, by which
# compiler. Without the compiler line a provider is unattributable.
{
  printf 'command\t%s\n' "$cmd"
  printf 'version\t%s\n' "$version"
  printf 'license\t%s\n' "$license"
  printf 'source_url\t%s\n' "$url"
  printf 'source_sha256\t%s\n' "$sha"
  printf 'compiler\t%s\n' "$CC_LABEL"
  printf 'built_sha256\t%s\n' "$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$target" | awk '{print $1}'; else shasum -a 256 "$target" | awk '{print $1}'; fi)"
  printf 'distributed\tno (built locally; provider binaries are never republished)\n'
} > "$dest/provenance.tsv"

say "PASS $cmd $version -> $target ($license, built locally from pinned upstream source)"
printf '%s\n' "$target"
