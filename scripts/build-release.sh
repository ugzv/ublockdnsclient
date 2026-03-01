#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/dist"
VERSION="${VERSION:-dev}"

mkdir -p "${OUT_DIR}"
rm -f "${OUT_DIR}"/ublockdns-* "${OUT_DIR}"/SHA256SUMS

build() {
  local goos="$1"
  local goarch="$2"
  local output="${OUT_DIR}/ublockdns-${goos}-${goarch}"

  GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${output}" "${ROOT_DIR}"
}

build linux amd64
build linux arm64

(
  cd "${OUT_DIR}"
  shasum -a 256 ublockdns-* > SHA256SUMS
)

echo "Release artifacts written to ${OUT_DIR}"
