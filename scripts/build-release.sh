#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/dist"
VERSION="${VERSION:-dev}"
HOST_OS="$(uname -s)"

mkdir -p "${OUT_DIR}"
rm -f "${OUT_DIR}"/ublockdns-* "${OUT_DIR}"/SHA256SUMS

build() {
  local goos="$1"
  local goarch="$2"
  local goarm="${3:-}"
  local suffix=""
  local output=""
  local cgo=0

  if [ -n "${goarm}" ]; then
    suffix="v${goarm}"
  fi

  if [ "${goos}" = "darwin" ]; then
    # nextdns/host uses cgo for Darwin syslog integration
    cgo=1
    if [ "${HOST_OS}" != "Darwin" ]; then
      echo "Skipping ${goos}/${goarch}${suffix}: requires Darwin host for CGO_ENABLED=1"
      return
    fi
  fi

  output="${OUT_DIR}/ublockdns-${goos}-${goarch}${suffix}"
  if [ "${goos}" = "windows" ]; then
    output="${output}.exe"
  fi

  echo "Building ${goos}/${goarch}${suffix} -> ${output}"
  GOOS="${goos}" GOARCH="${goarch}" GOARM="${goarm}" CGO_ENABLED="${cgo}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${output}" "${ROOT_DIR}"
}

build linux amd64
build linux arm64
build linux arm 7
build windows amd64
build windows arm64
build freebsd amd64
build darwin amd64
build darwin arm64

(
  cd "${OUT_DIR}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ublockdns-* > SHA256SUMS
  else
    shasum -a 256 ublockdns-* > SHA256SUMS
  fi
)

echo "Release artifacts written to ${OUT_DIR}"
