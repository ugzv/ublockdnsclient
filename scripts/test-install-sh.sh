#!/bin/sh
# Tests for the parts of install.sh that decide what gets run as root.
# Needs root (it assigns file ownership): sudo sh scripts/test-install-sh.sh
set -u

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck disable=SC2034  # read by install.sh
UBLOCKDNS_INSTALL_SOURCED=1
# shellcheck source=../install.sh
. "${ROOT_DIR}/install.sh"
set +e
setup_colors

if [ "$(id -u)" -ne 0 ]; then
    echo "must run as root" >&2
    exit 2
fi

FAILED=0
WORK="$(mktemp -d "/tmp/ublockdns-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

check() {
    label="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "ok    ${label}"
    else
        echo "FAIL  ${label}"
        FAILED=1
    fi
}
fails() { ! "$@"; }
root_file_0755() { [ -n "$(find "$1" -prune -type f -user 0 -perm 0755)" ]; }

# ── Binary placement ────────────────────────────────────────

printf 'binary' > "${WORK}/download"
chown 12345 "${WORK}/download"

# A root-only directory is used as is.
INSTALL_DIR="${WORK}/safe/bin"
SERVICE_DIR="${WORK}/unused/ublockdns"
install_binary "${WORK}/download" >/dev/null
check "root-only dir: binary goes there" [ "$BIN_PATH" = "${INSTALL_DIR}/${BINARY}" ]
check "root-only dir: owned by root, 0755, not the downloader" root_file_0755 "$BIN_PATH"
check "root-only dir: other location untouched" [ ! -e "$SERVICE_DIR" ]

# A directory someone else can write to only gets a link.
INSTALL_DIR="${WORK}/homebrew/bin"
SERVICE_DIR="${WORK}/opt/ublockdns"
mkdir -p "$INSTALL_DIR"
chown 12345 "$INSTALL_DIR"
printf 'old' > "${INSTALL_DIR}/${BINARY}"
install_binary "${WORK}/download" >/dev/null
check "writable dir: binary goes to the root-only location" [ "$BIN_PATH" = "${SERVICE_DIR}/${BINARY}" ]
check "writable dir: owned by root, 0755" root_file_0755 "$BIN_PATH"
check "writable dir: old file replaced by a link" [ -L "${INSTALL_DIR}/${BINARY}" ]
check "writable dir: link runs the new binary" [ "$(cat "${INSTALL_DIR}/${BINARY}")" = "binary" ]

# With no root-only location to fall back to, install still completes.
INSTALL_DIR="${WORK}/homebrew2/bin"
SERVICE_DIR="${WORK}/shared/ublockdns"
mkdir -p "$INSTALL_DIR" "${WORK}/shared"
chown 12345 "$INSTALL_DIR" "${WORK}/shared"
install_binary "${WORK}/download" >/dev/null
check "no safe location: falls back to the usual dir" [ "$BIN_PATH" = "${INSTALL_DIR}/${BINARY}" ]
check "no safe location: file still owned by root" root_file_0755 "$BIN_PATH"

# ── Release signature ───────────────────────────────────────

download() { cp "$SIG_SOURCE" "$2"; }

if has openssl && openssl pkeyutl -help 2>&1 | grep -q -- '-rawin' &&
    openssl genpkey -algorithm ed25519 -out "${WORK}/key.pem" 2>/dev/null; then
    # shellcheck disable=SC2034  # read by verify_manifest_signature
    RELEASE_PUBLIC_KEY="$(openssl pkey -in "${WORK}/key.pem" -pubout)"
    printf 'abc123  ublockdns-linux-amd64\n' > "${WORK}/sums"
    openssl pkeyutl -sign -inkey "${WORK}/key.pem" -rawin -in "${WORK}/sums" -out "${WORK}/sig.bin"
    openssl base64 -A -in "${WORK}/sig.bin" -out "${WORK}/sig"
    printf 'def456  ublockdns-linux-amd64\n' > "${WORK}/tampered"
    printf 'AAAA' > "${WORK}/badsig"

    TMP_DIR="${WORK}/tmp"
    mkdir -p "$TMP_DIR"
    SIG_SOURCE="${WORK}/sig"
    check "signature: genuine manifest accepted" verify_manifest_signature "${WORK}/sums" x
    check "signature: tampered manifest refused" fails verify_manifest_signature "${WORK}/tampered" x
    SIG_SOURCE="${WORK}/badsig"
    check "signature: wrong signature refused" fails verify_manifest_signature "${WORK}/sums" x
    SIG_SOURCE="${WORK}/missing"
    check "signature: missing signature refused" fails verify_manifest_signature "${WORK}/sums" x
    # A check that cannot be set up must not turn into a skipped check.
    TMP_DIR="${WORK}/does-not-exist"
    SIG_SOURCE="${WORK}/sig"
    check "signature: setup failure refused, not skipped" fails verify_manifest_signature "${WORK}/sums" x
else
    echo "skip  signature checks: this openssl has no Ed25519 support"
fi

exit "$FAILED"
