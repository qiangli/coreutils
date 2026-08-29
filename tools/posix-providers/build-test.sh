#!/bin/bash
set -euo pipefail

here=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
POSIX_PROVIDER_DOWNLOAD_LIBRARY_ONLY=1
. "$here/build.sh"
unset POSIX_PROVIDER_DOWNLOAD_LIBRARY_ONLY

tmp=$(mktemp -d "${TMPDIR:-/tmp}/posix-provider-download-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/localedef.Makefile" <<'EOF'
DEFINES = -DNO_SYSCONF \
	  -DNO_UNCOMPRESS \
	  -DLOCALEDIR='"/usr/lib/locale"'
EOF
provider_prepare_localedef_makefile "$tmp/localedef.Makefile"
if grep -q -- '-DNO_UNCOMPRESS' "$tmp/localedef.Makefile"; then
  echo 'localedef harness still disables compressed charmaps' >&2
  exit 1
fi
grep -q -- '-DNO_SYSCONF' "$tmp/localedef.Makefile" || {
  echo 'localedef harness rewrite removed an adjacent define' >&2
  exit 1
}
grep -q './configure --prefix=/usr --with-glibc=' "$here/build.sh" || {
  echo 'localedef harness no longer targets the distribution locale tree' >&2
  exit 1
}

# POSIX conformance verbosity belongs only to the charmap reader. Verify the
# pinned correction applies to the expected glibc 2.39 source shape, records
# its digest, preserves explicit --verbose, and fails closed on drift/reapply.
mkdir -p "$tmp/glibc/locale/programs"
cat > "$tmp/glibc/locale/programs/localedef.c" <<'EOF'
  if (delete_from_archive)
    return delete_locales_from_archive (argc - remaining, &argv[remaining]);

  /* POSIX.2 requires to be verbose about missing characters in the
     character map.  */
  verbose |= posix_conformance;

  if (argc - remaining != 1)
    {
      /* We need exactly one non-option parameter.  */

#endif

  /* Process charmap file.  */
  charmap = charmap_read (charmap_file, verbose, 1, be_quiet, 1);

  /* Add the first entry in the locale list.  */
  memset (&global, '\0', sizeof (struct localedef_t));
EOF
provider_prepare_localedef_source "$tmp/glibc" "$tmp/localedef-extra.tsv"
! grep -q 'verbose |= posix_conformance' "$tmp/glibc/locale/programs/localedef.c"
grep -q 'verbose || posix_conformance' "$tmp/glibc/locale/programs/localedef.c"
grep -q '^recipe_patch	patches/glibc-2.39-posix-verbosity.patch	b13314db242417133d9a769170cc348cb5ca4ca7b8e1cfcc9325ab73e7e199fd$' \
  "$tmp/localedef-extra.tsv"
if provider_prepare_localedef_source "$tmp/glibc" "$tmp/localedef-extra-2.tsv" >/dev/null 2>&1; then
  echo 'localedef source correction unexpectedly applied twice' >&2
  exit 1
fi

# The lp recipe must remove CUPS' process-wide SIGPIPE override, fail closed if
# the pinned source shape drifts, and record the exact local correction digest.
mkdir -p "$tmp/cups/cups"
: > "$tmp/cups/cups/http.c"
_lp_line=1
while [ "$_lp_line" -le 1528 ]; do
  printf '/* fixture padding */\n' >> "$tmp/cups/cups/http.c"
  _lp_line=$((_lp_line + 1))
done
cat >> "$tmp/cups/cups/http.c" <<'EOF'
void
httpInitialize(void)
{
#ifdef _WIN32
  WSAStartup(MAKEWORD(2,2), &winsockdata);

#elif !defined(SO_NOSIGPIPE)
 /*
  * Ignore SIGPIPE signals...
  */

#  ifdef HAVE_SIGSET
  sigset(SIGPIPE, SIG_IGN);

#  elif defined(HAVE_SIGACTION)
  struct sigaction	action;		/* POSIX sigaction data */


  memset(&action, 0, sizeof(action));
  action.sa_handler = SIG_IGN;
  sigaction(SIGPIPE, &action, NULL);

#  else
  signal(SIGPIPE, SIG_IGN);
#  endif /* !SO_NOSIGPIPE */
#endif /* _WIN32 */

#  ifdef HAVE_TLS
EOF
provider_prepare_lp_source "$tmp/cups" "$tmp/lp-extra.tsv"
! grep -q 'SIGPIPE, SIG_IGN' "$tmp/cups/cups/http.c"
grep -q '^recipe_patch	patches/cups-2.4.7-posix-sigpipe.patch	4846fba634296d6ec4cc642637bb4ee3d33d43bec7ad7e3ba9ba56762d3cff71$' \
  "$tmp/lp-extra.tsv"
if provider_prepare_lp_source "$tmp/cups" "$tmp/lp-extra-2.tsv" >/dev/null 2>&1; then
  echo 'lp source correction unexpectedly applied twice' >&2
  exit 1
fi

printf 'recipe_revision\t1\n' > "$tmp/provenance.tsv"
if provider_recipe_current "$tmp/provenance.tsv" 2; then
  echo 'stale localedef recipe revision was accepted' >&2
  exit 1
fi
printf 'recipe_revision\t2\n' > "$tmp/provenance.tsv"
provider_recipe_current "$tmp/provenance.tsv" 2

# A current man recipe is not a cache hit unless every runtime payload still
# matches its recorded identity. This keeps the resolver's rebuild instruction
# recoverable after deletion or tampering.
mkdir -p "$tmp/man/share/man/man1"
printf '#!/bin/sh\n' > "$tmp/man/man"
chmod 0755 "$tmp/man/man"
printf '#!/bin/sh\n' > "$tmp/man/apropos"
chmod 0755 "$tmp/man/apropos"
printf '.TH MAN 1\n' > "$tmp/man/share/man/man1/man.1"
binary_sha=$(provider_sha256 "$tmp/man/man")
apropos_sha=$(provider_sha256 "$tmp/man/apropos")
manual_sha=$(provider_sha256 "$tmp/man/share/man/man1/man.1")
{
  printf 'built_sha256\t%s\n' "$binary_sha"
  printf 'companion_apropos_sha256\t%s\n' "$apropos_sha"
  printf 'manual_man1_sha256\t%s\n' "$manual_sha"
} > "$tmp/man/provenance.tsv"
provider_man_cache_current "$tmp/man"
printf 'tampered\n' >> "$tmp/man/share/man/man1/man.1"
if provider_man_cache_current "$tmp/man"; then
  echo 'tampered man manual was accepted as a current cache' >&2
  exit 1
fi
printf '.TH MAN 1\n' > "$tmp/man/share/man/man1/man.1"
rm "$tmp/man/apropos"
if provider_man_cache_current "$tmp/man"; then
  echo 'missing man apropos was accepted as a current cache' >&2
  exit 1
fi

mkdir -p "$tmp/bin"
printf 'verified provider source\n' > "$tmp/good"
good_sha=$(provider_sha256 "$tmp/good")

cat > "$tmp/bin/curl" <<'MOCK'
#!/bin/bash
set -euo pipefail
out=
url=
printf '%s\n' "$*" >> "$MOCK_ARGS"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) out=$2; shift 2 ;;
    --connect-timeout|--max-time) shift 2 ;;
    --*) shift ;;
    *) url=$1; shift ;;
  esac
done
printf '%s\n' "$url" >> "$MOCK_URLS"
case "$MOCK_MODE:$url" in
  fallback:https://ftp.gnu.org/*) exit 22 ;;
  mismatch:https://ftp.gnu.org/*) printf 'wrong bytes\n' > "$out"; exit 0 ;;
  fail:*) exit 28 ;;
esac
cp "$MOCK_GOOD" "$out"
MOCK
chmod 0755 "$tmp/bin/curl"

export PATH="$tmp/bin:$PATH"
export MOCK_GOOD="$tmp/good" MOCK_ARGS="$tmp/args" MOCK_URLS="$tmp/urls"
export POSIX_PROVIDER_DOWNLOAD_ATTEMPTS=2
export POSIX_PROVIDER_DOWNLOAD_CONNECT_TIMEOUT=3
export POSIX_PROVIDER_DOWNLOAD_MAX_TIME=17

url=https://ftp.gnu.org/gnu/m4/m4-1.4.19.tar.xz
export MOCK_MODE=fallback
got=$(provider_download_verified "$url" "$good_sha" "$tmp/archive")
[ "$got" = https://ftp.fau.de/gnu/m4/m4-1.4.19.tar.xz ]
cmp "$tmp/good" "$tmp/archive"
[ "$(cat "$tmp/archive.retrieved-url")" = "$got" ]
cat > "$tmp/want-urls" <<'EOF'
https://ftp.gnu.org/gnu/m4/m4-1.4.19.tar.xz
https://ftp.gnu.org/gnu/m4/m4-1.4.19.tar.xz
https://ftp.fau.de/gnu/m4/m4-1.4.19.tar.xz
EOF
cmp "$tmp/want-urls" "$MOCK_URLS"
[ "$(grep -c -- '--connect-timeout 3 --max-time 17' "$MOCK_ARGS")" -eq 3 ]

# A responding mirror does not win unless its bytes match the manifest pin.
rm -f "$tmp/archive" "$tmp/archive.retrieved-url" "$MOCK_URLS" "$MOCK_ARGS"
export MOCK_MODE=mismatch
got=$(provider_download_verified "$url" "$good_sha" "$tmp/archive")
[ "$got" = https://ftp.fau.de/gnu/m4/m4-1.4.19.tar.xz ]
cmp "$tmp/good" "$tmp/archive"

# A verified cache hit performs no network request and retains retrieval origin.
rm -f "$MOCK_URLS"
got=$(provider_download_verified "$url" "$good_sha" "$tmp/archive")
[ "$got" = https://ftp.fau.de/gnu/m4/m4-1.4.19.tar.xz ]
[ ! -e "$MOCK_URLS" ]

# Non-GNU sources retry their canonical URL but never invent a mirror path.
rm -f "$tmp/archive" "$tmp/archive.retrieved-url" "$MOCK_URLS"
export MOCK_MODE=fail
if provider_download_verified https://example.invalid/source.tar.xz "$good_sha" "$tmp/archive"; then
  echo 'expected failed non-GNU download' >&2
  exit 1
fi
[ "$(wc -l < "$MOCK_URLS" | tr -d ' ')" -eq 2 ]
[ "$(sort -u "$MOCK_URLS")" = https://example.invalid/source.tar.xz ]
[ ! -e "$tmp/archive" ]
[ ! -e "$tmp/archive.part" ]

# Exercise the generic production entry point, including extraction and
# provenance, with a tiny synthetic provider. Do not call it m4: the real m4
# route deliberately applies and gates the pinned POSIX semantics correction.
mkdir -p "$tmp/fake-fixture"
cat > "$tmp/fake-fixture/configure" <<'CONFIGURE'
#!/bin/sh
set -eu
printf '#!/bin/sh\nexit 0\n' > fixture
chmod 0755 fixture
printf 'all:\n\t@:\n' > Makefile
CONFIGURE
chmod 0755 "$tmp/fake-fixture/configure"
tar -cJf "$tmp/fake-fixture.tar.xz" -C "$tmp" fake-fixture
fake_sha=$(provider_sha256 "$tmp/fake-fixture.tar.xz")
printf 'fixture\t9.9\tMIT\tlinux,darwin\t%s\t%s\n' \
  "$fake_sha" "$url" > "$tmp/manifest.tsv"
export MOCK_MODE=fallback MOCK_GOOD="$tmp/fake-fixture.tar.xz"
rm -f "$MOCK_URLS" "$MOCK_ARGS"
POSIX_PROVIDER_MANIFEST="$tmp/manifest.tsv" \
POSIX_PROVIDER_WORK="$tmp/build-work" \
BASHY_BIN_CACHE="$tmp/cache" \
POSIX_PROVIDER_CC=/usr/bin/cc \
  "$here/build.sh" fixture >/dev/null
prov=$tmp/cache/fixture/9.9/provenance.tsv
[ -x "$tmp/cache/fixture/9.9/fixture" ]
awk -F '\t' -v want="$url" '$1 == "source_url" && $2 == want { found=1 } END { exit !found }' "$prov"
awk -F '\t' '$1 == "retrieved_url" && $2 == "https://ftp.fau.de/gnu/m4/m4-1.4.19.tar.xz" { found=1 } END { exit !found }' "$prov"
awk -F '\t' -v want="$fake_sha" '$1 == "source_sha256" && $2 == want { found=1 } END { exit !found }' "$prov"

if POSIX_PROVIDER_DOWNLOAD_ATTEMPTS=0 \
    provider_download_verified "$url" "$good_sha" "$tmp/archive"; then
  echo 'expected invalid retry count to fail' >&2
  exit 1
fi

echo 'PASS posix provider deterministic download fallback'
