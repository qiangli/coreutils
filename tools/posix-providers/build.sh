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

provider_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

provider_download_candidates() {
  _provider_url=$1
  printf '%s\n' "$_provider_url"
  case "$_provider_url" in
    https://ftp.gnu.org/gnu/*)
      _provider_path=${_provider_url#https://ftp.gnu.org/gnu/}
      # These fixed HTTPS endpoints are on GNU's published mirror list. Keep
      # the order explicit: ftpmirror.gnu.org redirects by client geography,
      # which makes it unsuitable for a repeatable certification recipe.
      printf '%s\n' \
        "https://ftp.fau.de/gnu/$_provider_path" \
        "https://mirror.csclub.uwaterloo.ca/gnu/$_provider_path" \
        "https://mirrors.ocf.berkeley.edu/gnu/$_provider_path"
      ;;
  esac
}

provider_positive_integer() {
  case "$1" in ''|*[!0-9]*|0) return 1 ;; esac
}

# The standalone localedef harness defaults to /usr/local and disables its
# gzip/bzip2 charmap reader. Linux distributions install the locale source tree
# below /usr/share/i18n and normally compress the charmaps there. Keep the
# provider aligned with the glibc installation it is built to accompany.
provider_prepare_localedef_makefile() {
  _provider_makefile=$1
  if ! grep -q '^[[:space:]]*-DNO_UNCOMPRESS' "$_provider_makefile"; then
    printf 'posix-provider: localedef harness no longer has the expected NO_UNCOMPRESS define\n' >&2
    return 1
  fi
  _provider_makefile_tmp=$_provider_makefile.tmp.$$
  sed '/^[[:space:]]*-DNO_UNCOMPRESS/d' "$_provider_makefile" > "$_provider_makefile_tmp" || {
    rm -f "$_provider_makefile_tmp"
    return 1
  }
  mv "$_provider_makefile_tmp" "$_provider_makefile"
}

# glibc 2.39 promotes POSIXLY_CORRECT to global --verbose mode even though the
# requirement cited by its own comment applies only to missing characters in
# the charmap. That leaks diagnostics for omitted fields in GNU-only LC_*
# categories and unrelated table-size chatter into otherwise valid POSIX
# compilations. Apply a pinned source correction which passes conformance
# verbosity only to the charmap reader and leaves explicit --verbose intact.
provider_prepare_localedef_source() {
  _provider_localedef_src=$1
  _provider_localedef_record=$2
  _provider_localedef_patch=$here/patches/glibc-2.39-posix-verbosity.patch
  _provider_localedef_patch_sha=b13314db242417133d9a769170cc348cb5ca4ca7b8e1cfcc9325ab73e7e199fd
  [ -f "$_provider_localedef_patch" ] || {
    printf 'posix-provider: localedef POSIX verbosity correction is missing: %s\n' \
      "$_provider_localedef_patch" >&2
    return 1
  }
  _provider_localedef_actual=$(provider_sha256 "$_provider_localedef_patch")
  [ "$_provider_localedef_actual" = "$_provider_localedef_patch_sha" ] || {
    printf 'posix-provider: localedef POSIX verbosity correction digest mismatch: got %s want %s\n' \
      "$_provider_localedef_actual" "$_provider_localedef_patch_sha" >&2
    return 1
  }
  command -v patch >/dev/null 2>&1 || {
    printf 'posix-provider: patch is required to prepare localedef\n' >&2
    return 1
  }
  (cd "$_provider_localedef_src" && patch -f -p1 < "$_provider_localedef_patch") || {
    printf 'posix-provider: localedef POSIX verbosity correction did not apply cleanly\n' >&2
    return 1
  }
  printf 'recipe_patch\t%s\t%s\n' \
    "patches/glibc-2.39-posix-verbosity.patch" "$_provider_localedef_patch_sha" \
    >> "$_provider_localedef_record"
}

# CUPS 2.4.7's libcups initialization changes SIGPIPE to SIG_IGN process-wide
# on platforms without SO_NOSIGPIPE. POSIX lp declares ASYNCHRONOUS EVENTS as
# Default, so a utility invocation must preserve the inherited disposition.
# Apply the small, pinned source correction before building the client and
# record its identity beside the upstream source identity.
provider_prepare_lp_source() {
  _provider_lp_src=$1
  _provider_lp_record=$2
  _provider_lp_patch=$here/patches/cups-2.4.7-posix-sigpipe.patch
  _provider_lp_patch_sha=4846fba634296d6ec4cc642637bb4ee3d33d43bec7ad7e3ba9ba56762d3cff71
  [ -f "$_provider_lp_patch" ] || {
    printf 'posix-provider: lp source correction is missing: %s\n' "$_provider_lp_patch" >&2
    return 1
  }
  _provider_lp_actual=$(provider_sha256 "$_provider_lp_patch")
  [ "$_provider_lp_actual" = "$_provider_lp_patch_sha" ] || {
    printf 'posix-provider: lp source correction digest mismatch: got %s want %s\n' \
      "$_provider_lp_actual" "$_provider_lp_patch_sha" >&2
    return 1
  }
  command -v patch >/dev/null 2>&1 || {
    printf 'posix-provider: patch is required to prepare lp\n' >&2
    return 1
  }
  (cd "$_provider_lp_src" && patch -f -p1 < "$_provider_lp_patch") || {
    printf 'posix-provider: lp POSIX SIGPIPE correction did not apply cleanly\n' >&2
    return 1
  }
  printf 'recipe_patch\t%s\t%s\n' \
    "patches/cups-2.4.7-posix-sigpipe.patch" "$_provider_lp_patch_sha" >> "$_provider_lp_record"
}

# GNU m4 1.4.19's traditional switch disables extensions but retains several
# historical GNU behaviors that conflict with the POSIX interface: lower-case
# eval digits above nine, LIFO repeated m4wrap calls, changequote() disabling
# quotes, unrestricted -D/-U names, and a fatal GNU deprecation warning for the
# POSIX maketemp macro. It also replaces default fatal-signal dispositions with
# diagnostic handlers. Apply the reviewed correction to the unpacked upstream
# tree. The GPL source remains an external, local build input. The tracked
# source-derived patch is GPL-3.0-or-later; its path and digest are also recorded
# in the provider provenance sidecar. Neither it nor GNU m4 is linked into the
# Go multicall binary.
provider_prepare_m4_source() {
  _provider_m4_src=$1
  _provider_m4_record=$2
  _provider_m4_patch=$here/patches/m4-1.4.19-posix-semantics.patch
  _provider_m4_patch_sha=63715144b5dce740bffa85860b819c65f26422cee8c66e56212d39fc0de334a2
  [ -f "$_provider_m4_patch" ] || {
    printf 'posix-provider: m4 POSIX correction is missing: %s\n' \
      "$_provider_m4_patch" >&2
    return 1
  }
  _provider_m4_actual=$(provider_sha256 "$_provider_m4_patch")
  [ "$_provider_m4_actual" = "$_provider_m4_patch_sha" ] || {
    printf 'posix-provider: m4 POSIX correction digest mismatch: got %s want %s\n' \
      "$_provider_m4_actual" "$_provider_m4_patch_sha" >&2
    return 1
  }
  command -v git >/dev/null 2>&1 || {
    printf 'posix-provider: git is required to prepare m4\n' >&2
    return 1
  }
  (cd "$_provider_m4_src" &&
     git apply --unidiff-zero --check "$_provider_m4_patch" &&
     git apply --unidiff-zero "$_provider_m4_patch") || {
    printf 'posix-provider: m4 POSIX correction did not apply cleanly\n' >&2
    return 1
  }
  printf 'recipe_patch\t%s\t%s\n' \
    "patches/m4-1.4.19-posix-semantics.patch" "$_provider_m4_patch_sha" \
    >> "$_provider_m4_record"
}

# provider_recipe_current PROVENANCE EXPECTED_REVISION
# Empty revisions retain the legacy cache identity. A non-empty revision is a
# product build-recipe identity and must match exactly before a cached provider
# can be reused.
provider_recipe_current() {
  _provider_provenance=$1
  _provider_expected_revision=$2
  [ -z "$_provider_expected_revision" ] && return 0
  [ -f "$_provider_provenance" ] || return 1
  awk -F '\t' -v want="$_provider_expected_revision" \
    '$1 == "recipe_revision" && $2 == want { found=1 } END { exit !found }' \
    "$_provider_provenance"
}

# provider_man_cache_current DEST
#
# A v2 man cache is one unit: executable, apropos companion, and self-manual.
# Do the same digest checks at the build cache-hit boundary as the Go resolver,
# so the resolver's "rebuild it" recovery instruction actually repairs a
# missing or tampered payload instead of accepting the executable alone.
provider_man_cache_current() {
  _provider_man_dest=$1
  _provider_man_provenance=$_provider_man_dest/provenance.tsv
  _provider_man_binary=$_provider_man_dest/man
  _provider_man_apropos=$_provider_man_dest/apropos
  _provider_man_manual=$_provider_man_dest/share/man/man1/man.1
  [ -f "$_provider_man_binary" ] && [ ! -L "$_provider_man_binary" ] &&
    [ -x "$_provider_man_binary" ] || return 1
  [ -f "$_provider_man_apropos" ] && [ ! -L "$_provider_man_apropos" ] &&
    [ -x "$_provider_man_apropos" ] || return 1
  [ -f "$_provider_man_manual" ] && [ ! -L "$_provider_man_manual" ] &&
    [ -s "$_provider_man_manual" ] || return 1
  _provider_man_binary_want=$(awk -F '\t' '$1 == "built_sha256" { print $2; exit }' "$_provider_man_provenance")
  _provider_man_apropos_want=$(awk -F '\t' '$1 == "companion_apropos_sha256" { print $2; exit }' "$_provider_man_provenance")
  _provider_man_manual_want=$(awk -F '\t' '$1 == "manual_man1_sha256" { print $2; exit }' "$_provider_man_provenance")
  [ ${#_provider_man_binary_want} -eq 64 ] &&
    [ ${#_provider_man_apropos_want} -eq 64 ] &&
    [ ${#_provider_man_manual_want} -eq 64 ] || return 1
  [ "$(provider_sha256 "$_provider_man_binary")" = "$_provider_man_binary_want" ] &&
    [ "$(provider_sha256 "$_provider_man_apropos")" = "$_provider_man_apropos_want" ] &&
    [ "$(provider_sha256 "$_provider_man_manual")" = "$_provider_man_manual_want" ]
}

# provider_download_verified URL SHA256 DEST
#
# Prints the URL that supplied the verified bytes. Every endpoint and attempt
# has a wall-clock bound. A candidate only wins after its bytes match the
# manifest digest; partial or mismatched files never become the archive.
provider_download_verified() {
  _provider_url=$1
  _provider_sha=$2
  _provider_dest=$3
  _provider_attempts=${POSIX_PROVIDER_DOWNLOAD_ATTEMPTS:-2}
  _provider_connect_timeout=${POSIX_PROVIDER_DOWNLOAD_CONNECT_TIMEOUT:-10}
  _provider_max_time=${POSIX_PROVIDER_DOWNLOAD_MAX_TIME:-600}

  provider_positive_integer "$_provider_attempts" || {
    printf 'posix-provider: invalid download attempt count: %s\n' "$_provider_attempts" >&2
    return 2
  }
  provider_positive_integer "$_provider_connect_timeout" || {
    printf 'posix-provider: invalid download connect timeout: %s\n' "$_provider_connect_timeout" >&2
    return 2
  }
  provider_positive_integer "$_provider_max_time" || {
    printf 'posix-provider: invalid download max time: %s\n' "$_provider_max_time" >&2
    return 2
  }

  if [ -f "$_provider_dest" ]; then
    _provider_actual=$(provider_sha256 "$_provider_dest")
    if [ "$_provider_actual" = "$_provider_sha" ]; then
      if [ -f "$_provider_dest.retrieved-url" ]; then
        sed -n '1p' "$_provider_dest.retrieved-url"
      else
        # Archives created by the pre-fallback recipe have only the canonical
        # manifest URL as their available transport provenance.
        printf '%s\n' "$_provider_url"
      fi
      return 0
    fi
    printf 'posix-provider: cached digest mismatch; refetching %s\n' "$_provider_url" >&2
    rm -f "$_provider_dest" "$_provider_dest.retrieved-url"
  fi

  rm -f "$_provider_dest.part"
  for _provider_candidate in $(provider_download_candidates "$_provider_url"); do
    _provider_try=1
    while [ "$_provider_try" -le "$_provider_attempts" ]; do
      printf 'posix-provider: download attempt %s/%s: %s\n' \
        "$_provider_try" "$_provider_attempts" "$_provider_candidate" >&2
      rm -f "$_provider_dest.part"
      if curl --fail --location --silent --show-error \
          --connect-timeout "$_provider_connect_timeout" \
          --max-time "$_provider_max_time" \
          --output "$_provider_dest.part" "$_provider_candidate"; then
        _provider_actual=$(provider_sha256 "$_provider_dest.part")
        if [ "$_provider_actual" = "$_provider_sha" ]; then
          mv "$_provider_dest.part" "$_provider_dest"
          printf '%s\n' "$_provider_candidate" > "$_provider_dest.retrieved-url"
          printf '%s\n' "$_provider_candidate"
          return 0
        fi
        printf 'posix-provider: digest mismatch from %s: got %s want %s\n' \
          "$_provider_candidate" "$_provider_actual" "$_provider_sha" >&2
      fi
      _provider_try=$((_provider_try + 1))
    done
  done
  rm -f "$_provider_dest.part"
  return 1
}

# Focused tests source this self-contained recipe to exercise only retrieval.
# Production invocations never set this variable and continue into the build.
if [ "${POSIX_PROVIDER_DOWNLOAD_LIBRARY_ONLY:-}" = 1 ]; then
  return 0 2>/dev/null || exit 0
fi
# The manifest is CANONICAL at pkg/posixprovider/manifest.tsv, because the Go
# library embeds it (//go:embed cannot reach outside its own package directory)
# and exactly one copy of the pins may exist. This script reads that same file so
# the recipe and the resolver can never disagree about what is pinned.
manifest=${POSIX_PROVIDER_MANIFEST:-$here/../../pkg/posixprovider/manifest.tsv}
# The posix-providers applet always passes BASHY_BIN_CACHE after deriving it
# from the authenticated OS account. default_cache is only the direct-script
# fallback; callers that need the resolver's authenticated default should use
# the applet. A provider built elsewhere is intentionally invisible at runtime.
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
recipe_revision=$(printf '%s' "$row" | cut -f7)

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
if [ -x "$target" ] &&
   provider_recipe_current "$dest/provenance.tsv" "$recipe_revision" &&
   { [ "$cmd" != man ] || provider_man_cache_current "$dest"; }; then
  say "cached: $target ($license)"; printf '%s\n' "$target"; exit 0
fi
[ ! -x "$target" ] || say "stale build recipe: $target ($license); rebuilding"
[ "$check_only" != "--check" ] || fail "$cmd $version is not provisioned (cache miss)"

need() { command -v "$1" >/dev/null 2>&1 || fail "required build tool not found: $1"; }
for t in curl tar make; do need "$t"; done
provider_jobs=${POSIX_PROVIDER_JOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)}
provider_positive_integer "$provider_jobs" ||
  fail "invalid provider build job count: $provider_jobs"

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
# and builds cleanly under zig cc, as does CUPS 2.4.7 (lp), also run-tested on
# darwin; binutils and vim are not yet verified.
#
# "zig cc" is TWO WORDS, and that is a trap. Autotools tolerates a $CC with a
# space; a build system that treats $CC as a single word does not, and the
# failure can surface far from the cause. The two-word form is therefore
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
say "fetching upstream $cmd $version ($license)"
retrieved_url=$(provider_download_verified "$url" "$sha" "$archive") ||
  fail "download failed from every approved source: $url"
say "digest verified"

# Some providers need MORE than one pinned input. fetch_verified applies the
# same rule to each of them: download, hash, compare, and only then let the
# bytes be used. A secondary input with no digest is refused exactly like a
# primary one -- otherwise the weakest link decides the provenance.
fetch_verified() { # url sha256 dest
  _u=$1; _s=$2; _d=$3
  [ ${#_s} -eq 64 ] || fail "secondary input $_u has no full sha256 pin"
  _ru=$(provider_download_verified "$_u" "$_s" "$_d") ||
    fail "download failed from every approved source: $_u"
  # A provider assembled from several inputs is only attributable if ALL of them
  # are named. The manifest row can hold one url and one digest, so the rest are
  # recorded here and folded into provenance.tsv beside the primary source.
  printf 'extra_source\t%s\t%s\n' "$_u" "$_s" >> "$work/extra-$cmd.tsv"
  printf 'extra_retrieved_url\t%s\n' "$_ru" >> "$work/extra-$cmd.tsv"
}

src=$work/src-$cmd-$version
rm -rf "$src"; mkdir -p "$src"
case "$archive" in
  *.tar.gz|*.tgz) tar --extract --gzip --directory="$src" --strip-components=1 --file="$archive" ;;
  *.tar.xz)       tar --extract --xz --directory="$src" --strip-components=1 --file="$archive" ;;
  *.tar.lz)       command -v lzip >/dev/null 2>&1 || fail "lzip is required for $cmd"
                  lzip -dc "$archive" | tar --extract --directory="$src" --strip-components=1 --file=- ;;
  *) fail "unsupported archive form: $archive" ;;
esac

build_dir=$work/build-$cmd-$version
rm -rf "$build_dir"; mkdir -p "$build_dir"
rm -f "$work/extra-$cmd.tsv"
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
    #
    # TLS IS NOT OPTIONAL HERE, though POSIX lp(1) has nothing to do with it.
    # CUPS 2.4.7 defaults --with-tls=yes and hard-fails configure when no TLS
    # library is present; and --with-tls=no does not save you either, because the
    # no-TLS path is broken upstream -- cups/hash.c includes <gnutls/crypto.h>
    # unconditionally and the compile dies there. Both measured on a minimal
    # ubuntu:24.04. darwin is unaffected: configure finds the Security framework.
    # So the dependency is real, and the only useful thing this recipe can do is
    # say so BEFORE spending three minutes in configure.
    if [ "$host_os" = linux ]; then
      _tp=$work/tls-probe.c
      printf '#include <openssl/ssl.h>\nint main(void){return 0;}\n' > "$_tp"
      $CC -fsyntax-only "$_tp" 2>/dev/null || {
        printf '#include <gnutls/crypto.h>\nint main(void){return 0;}\n' > "$_tp"
        $CC -fsyntax-only "$_tp" 2>/dev/null ||
          fail "lp needs a TLS library's headers (CUPS 2.4.7 cannot build without one, even with --with-tls=no): install libssl-dev or libgnutls28-dev"
      }
    fi
    provider_prepare_lp_source "$src" "$work/extra-$cmd.tsv" ||
      fail "cannot prepare lp $version for POSIX signal inheritance"
    (cd "$src" && ./configure --disable-shared --disable-dbus --disable-pam \
       --disable-libusb --without-rcdir --without-systemd >/dev/null &&
       make -C cups libcups.a >/dev/null &&
       make -C systemv lp LIBZ= >/dev/null)
    found=$src/systemv/lp
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
    need git
    _h=$work/ldef-$version
    _pd=$work/ldef-patches-$version
    rm -rf "$_h"; mkdir -p "$_h" "$_pd"
    fetch_verified \
      https://codeload.github.com/kraj/localedef/tar.gz/cba02c503d7c853a38ccfb83c57e343ca5ecd7e5 \
      358b131aceb9c4b7914598242bfa34120ad547acfca4799e5392a62a905db901 \
      "$_pd/localedef-harness.tar.gz"
    tar --extract --gzip --directory="$_h" --strip-components=1 \
      --file="$_pd/localedef-harness.tar.gz"

    _oe=543550522f831479f07d332a40ba343c53ae1065
    _oebase=https://raw.githubusercontent.com/openembedded/openembedded-core/$_oe/meta/recipes-core/glibc/glibc
    while read -r _pf _psha; do
      [ -n "$_pf" ] || continue
      fetch_verified "$_oebase/$_pf" "$_psha" "$_pd/$_pf"
      # POSIX patch(1) selects the old /dev/null name for git-format file
      # creation hunks. GNU patch's friendlier second-name heuristic is
      # intentionally disabled by POSIXLY_CORRECT, so use git's native patch
      # reader while retaining the global POSIX environment for the process.
      (cd "$src" && git apply --check "$_pd/$_pf" && git apply "$_pd/$_pf") ||
        fail "localedef: patch $_pf did not apply to glibc $version"
    done <<'PATCHES'
0001-localedef-Add-hardlink-resolver-from-util-linux.patch e77e2e527674ab7cadecbe503388e718dc6e4e95c925627b5214c9f5ce6aeb92
0002-localedef-fix-ups-hardlink-to-make-it-compile.patch 9faa11e6e2d386f2dd945e63f29a1b77c20a211e0a12ff42b0c9d483b19345d3
0013-eglibc-Forward-port-cross-locale-generation-support.patch cbe2e5e881af37de43015632753327de631ee2a5763a9cb9ba36f85305a9e7ef
0014-localedef-add-to-archive-uses-a-hard-coded-locale-pa.patch 6d7ff33fc45d1fc7225c67f0a1a0a329dcf87b8f7c2e75d0bfc36e1585bc1f55
PATCHES

    provider_prepare_localedef_source "$src" "$work/extra-$cmd.tsv" ||
      fail "cannot prepare localedef $version for POSIX diagnostics"

    # The harness ships an eglibc-era config.sub that does not know aarch64 and
    # aborts configure with "machine not recognized". glibc's own scripts/ has
    # current copies, from the same pinned tarball, so no extra input is needed.
    cp "$src/scripts/config.sub" "$src/scripts/config.guess" "$_h/"

    # -DIS_IN(x)=0 is what tells glibc's sources they are NOT being compiled into
    # libc; without it intl/l10nflist.c fails on "#if IS_IN (libc)". The parens
    # are escaped because make hands this string to the shell, and the escaping
    # cannot move to configure time -- an unescaped define there fails the
    # compiler sanity check and configure exits 77.
    (cd "$_h" && ./configure --prefix=/usr --with-glibc="$src" >/dev/null &&
       provider_prepare_localedef_makefile Makefile &&
       make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" \
         CFLAGS='-O2 -fgnu89-inline -std=gnu99 -DIS_IN\(x\)=0' >/dev/null)
    found=$_h/localedef
    ;;
  m4)
    need git
    provider_prepare_m4_source "$src" "$work/extra-$cmd.tsv" ||
      fail "cannot prepare m4 $version for POSIX semantics"
    (cd "$build_dir" && "$src/configure" --disable-nls >/dev/null 2>&1 ||
       "$src/configure" >/dev/null)
    # Building the top-level `all` target regenerates m4.1 whenever the
    # corrected executable is newer than the shipped page, needlessly adding
    # help2man to the runtime provider toolchain. The provider installs only
    # the executable, so build its two actual dependency directories.
    (cd "$build_dir/lib" && make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" >/dev/null)
    (cd "$build_dir/src" && make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" m4 >/dev/null)
    found=$build_dir/src/m4
    ;;
  ctags)
    # The pin is a git-tag archive (archive/refs/tags/), which ships no
    # generated configure -- Universal Ctags produces it with autogen.sh, so the
    # default branch's "$src/configure" simply does not exist and the build died
    # with exit 127 "No such file or directory".
    #
    # The optional format backends are disabled deliberately: each would pull an
    # unpinned system library (libxml2, jansson, libyaml) into a provider whose
    # whole point is being built from pinned bytes, and none of them affects the
    # POSIX ctags(1) behaviour under test.
    (cd "$src" && ./autogen.sh >"$build_dir/ctags-autogen.log" 2>&1 ||
       { sed -n '1,40p' "$build_dir/ctags-autogen.log" >&2
         fail 'ctags autogen.sh failed (needs autoconf, automake, pkg-config)'; })
    (cd "$src" && ./configure --disable-nls --disable-xml --disable-json \
       --disable-yaml --disable-seccomp >"$build_dir/ctags-configure.log" 2>&1 ||
       { sed -n '1,40p' "$build_dir/ctags-configure.log" >&2
         fail 'ctags configure failed'; })
    (cd "$src" && make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)" >/dev/null)
    found=$src/ctags
    ;;
  man)
    # man-db's top-level SUBDIRS includes docs, whose man_db.ps target wants a
    # full TeX toolchain (texi2dvi/dvips) to typeset a manual the provider never
    # ships. It only activates at all because texinfo is installed for the
    # binutils providers, so the docs failure is a side effect of an unrelated
    # dependency. Build the program's own directories instead.
    #
    # A SUBDIRS= override on the top-level make is NOT equivalent: make passes it
    # down to every recursive invocation, so gl/lib then tries to enter
    # gl/lib/gl/lib and dies with "cd: gl/lib: No such file or directory".
    #
    # With shared libraries disabled, src/man is the real ELF binary rather
    # than libtool's build-tree wrapper. The cached artifact must be independent
    # of that build tree: with shared libraries enabled the ELF records
    # libmandb-<version>.so, but this recipe
    # installs only man itself. Such an artifact passes the cache/provenance
    # checks and then dies in the dynamic loader. Link man-db's private
    # libraries statically while leaving ordinary system libraries dynamic.
    # The certification host installs man-db as a declared prerequisite and
    # therefore owns the native manpath file. Point the upstream build at that
    # file instead of its /usr/local default, which is absent on the host image.
    case "$host_os" in
      linux) _man_config=/etc/manpath.config ;;
      darwin) _man_config=/etc/man.conf ;;
      *) fail "man has no configured manpath file for $host_os" ;;
    esac
    [ -r "$_man_config" ] ||
      fail "man needs the host manpath configuration: $_man_config"
    # `man -k` is specified by POSIX.  man-db implements it by execing its
    # separately-built `apropos` program, using the configure-time bindir.
    # Keep that companion beside the cached provider. The Go dispatch boundary
    # resolves it relative to the verified man binary at run time; embedding
    # $dest here would make an otherwise valid cache impossible to relocate.
    (cd "$build_dir" && "$src/configure" --disable-nls --disable-shared \
       --with-config-file="$_man_config" >/dev/null 2>&1 ||
       "$src/configure" --disable-shared \
       --with-config-file="$_man_config" >/dev/null)
    for _d in gl/lib lib libdb src; do
      (cd "$build_dir/$_d" &&
         make -j"$provider_jobs" >/dev/null) ||
        fail "man build failed in $_d"
    done
    # A locally-built provider must carry the manual for the standard utility
    # it implements.  Minimal certification hosts deliberately omit native
    # documentation payloads, so relying on /usr/share/man/man1/man.1 makes
    # `man man` host-image-dependent even though the executable is pinned.
    # Build the page from the same verified man-db source.  The runtime adapter
    # publishes this relocatable cache directory without replacing an explicit
    # caller MANPATH or man-db's configured default roots.
    (cd "$build_dir/man" &&
       make -j"$provider_jobs" >/dev/null) ||
      fail "man manual build failed"
    found=$build_dir/src/man
    companion=$build_dir/src/apropos
    provider_manual=$build_dir/man/man1/man.1
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

if [ "$cmd" = m4 ]; then
  [ -x "$here/m4-semantic-test.sh" ] || fail 'm4 executable semantic gate is missing'
  "$here/m4-semantic-test.sh" "$found" || fail 'm4 executable semantic gate failed'
fi

install -m 0755 "$found" "$target"
if [ "$cmd" = man ]; then
  [ -x "${companion:-}" ] || fail 'man build produced no apropos companion'
  [ -s "${provider_manual:-}" ] || fail 'man build produced no man(1) page'
  install -m 0755 "$companion" "$dest/apropos"
  install -d -m 0755 "$dest/share/man/man1"
  install -m 0644 "$provider_manual" "$dest/share/man/man1/man.1"
fi

# Provenance sidecar: what was built, from which pinned bytes, by which
# compiler. Without the compiler line a provider is unattributable.
{
  printf 'command\t%s\n' "$cmd"
  printf 'version\t%s\n' "$version"
  printf 'license\t%s\n' "$license"
  printf 'source_url\t%s\n' "$url"
  printf 'retrieved_url\t%s\n' "$retrieved_url"
  printf 'source_sha256\t%s\n' "$sha"
  [ -z "$recipe_revision" ] || printf 'recipe_revision\t%s\n' "$recipe_revision"
  [ -f "$work/extra-$cmd.tsv" ] && cat "$work/extra-$cmd.tsv"
  printf 'compiler\t%s\n' "$CC_LABEL"
  printf 'built_sha256\t%s\n' "$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$target" | awk '{print $1}'; else shasum -a 256 "$target" | awk '{print $1}'; fi)"
  if [ "$cmd" = man ]; then
    printf 'companion_apropos_sha256\t%s\n' "$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$dest/apropos" | awk '{print $1}'; else shasum -a 256 "$dest/apropos" | awk '{print $1}'; fi)"
    printf 'manual_man1_sha256\t%s\n' "$(if command -v sha256sum >/dev/null 2>&1; then sha256sum "$dest/share/man/man1/man.1" | awk '{print $1}'; else shasum -a 256 "$dest/share/man/man1/man.1" | awk '{print $1}'; fi)"
  fi
  printf 'distributed\tno (built locally; provider binaries are never republished)\n'
} > "$dest/provenance.tsv"

say "PASS $cmd $version -> $target ($license, built locally from pinned upstream source)"
printf '%s\n' "$target"
