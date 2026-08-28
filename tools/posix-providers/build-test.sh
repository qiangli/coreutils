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

printf 'recipe_revision\t1\n' > "$tmp/provenance.tsv"
if provider_recipe_current "$tmp/provenance.tsv" 2; then
  echo 'stale localedef recipe revision was accepted' >&2
  exit 1
fi
printf 'recipe_revision\t2\n' > "$tmp/provenance.tsv"
provider_recipe_current "$tmp/provenance.tsv" 2

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

# Exercise the production entry point, including extraction and provenance,
# with a tiny source archive whose configure script creates a fake m4 binary.
mkdir -p "$tmp/fake-m4"
cat > "$tmp/fake-m4/configure" <<'CONFIGURE'
#!/bin/sh
set -eu
printf '#!/bin/sh\nexit 0\n' > m4
chmod 0755 m4
printf 'all:\n\t@:\n' > Makefile
CONFIGURE
chmod 0755 "$tmp/fake-m4/configure"
tar -cJf "$tmp/fake-m4.tar.xz" -C "$tmp" fake-m4
fake_sha=$(provider_sha256 "$tmp/fake-m4.tar.xz")
printf 'm4\t9.9\tGPL-3.0\tlinux,darwin\t%s\t%s\n' \
  "$fake_sha" "$url" > "$tmp/manifest.tsv"
export MOCK_MODE=fallback MOCK_GOOD="$tmp/fake-m4.tar.xz"
rm -f "$MOCK_URLS" "$MOCK_ARGS"
POSIX_PROVIDER_MANIFEST="$tmp/manifest.tsv" \
POSIX_PROVIDER_WORK="$tmp/build-work" \
BASHY_BIN_CACHE="$tmp/cache" \
POSIX_PROVIDER_CC=/usr/bin/cc \
  "$here/build.sh" m4 >/dev/null
prov=$tmp/cache/m4/9.9/provenance.tsv
[ -x "$tmp/cache/m4/9.9/m4" ]
awk -F '\t' -v want="$url" '$1 == "source_url" && $2 == want { found=1 } END { exit !found }' "$prov"
awk -F '\t' '$1 == "retrieved_url" && $2 == "https://ftp.fau.de/gnu/m4/m4-1.4.19.tar.xz" { found=1 } END { exit !found }' "$prov"
awk -F '\t' -v want="$fake_sha" '$1 == "source_sha256" && $2 == want { found=1 } END { exit !found }' "$prov"

if POSIX_PROVIDER_DOWNLOAD_ATTEMPTS=0 \
    provider_download_verified "$url" "$good_sha" "$tmp/archive"; then
  echo 'expected invalid retry count to fail' >&2
  exit 1
fi

echo 'PASS posix provider deterministic download fallback'
