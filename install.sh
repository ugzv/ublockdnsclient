#!/bin/sh
set -e

# uBlock DNS CLI installer
# Usage: curl -sSf https://ublockdns.com/install.sh | sh -s -- <profile-id>

REPO="ugzv/ublockdnsclient"
BINARY="ublockdns"
INSTALL_DIR="/usr/local/bin"
TMP_BIN=""

setup_color() {
    # Only use colors if connected to a terminal
    if [ -t 1 ]; then
        RED=$(printf '\033[31m')
        GREEN=$(printf '\033[32m')
        YELLOW=$(printf '\033[33m')
        BLUE=$(printf '\033[34m')
        CYAN=$(printf '\033[36m')
        BOLD=$(printf '\033[1m')
        DIM=$(printf '\033[2m')
        RESET=$(printf '\033[0m')
    else
        RED=""
        GREEN=""
        YELLOW=""
        BLUE=""
        CYAN=""
        BOLD=""
        DIM=""
        RESET=""
    fi
}

info() {
    printf "%s==>%s %s\n" "${BLUE}${BOLD}" "${RESET}" "$*"
}

success() {
    printf "%s==>%s %s%s%s\n" "${GREEN}${BOLD}" "${RESET}" "${GREEN}" "$*" "${RESET}"
}

error() {
    printf "%s==>%s %s%s%s\n" "${RED}${BOLD}" "${RESET}" "${RED}" "$*" "${RESET}"
}

run_as_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    else
        sudo "$@"
    fi
}

cleanup() {
    if [ -n "$TMP_BIN" ] && [ -f "$TMP_BIN" ]; then
        rm -f "$TMP_BIN"
    fi
}

validate_dns() {
    if command -v dig >/dev/null 2>&1; then
        dig @127.0.0.1 example.com +short +time=3 +tries=1 >/dev/null 2>&1
        return $?
    fi

    if command -v dscacheutil >/dev/null 2>&1; then
        dscacheutil -q host -a name example.com >/dev/null 2>&1
        return $?
    fi

    return 0
}

wait_for_dns_ready() {
    attempts=20
    i=1
    while [ "$i" -le "$attempts" ]; do
        if validate_dns; then
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    return 1
}

main() {
    trap cleanup EXIT

    setup_color

    PROFILE_ID="${1:-}"
    if [ -z "$PROFILE_ID" ]; then
        error "Usage: curl -sSf https://ublockdns.com/install.sh | sh -s -- <profile-id>"
        printf "\n"
        info "Get your profile ID at https://ublockdns.com"
        exit 1
    fi

    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    TAG=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    if [ -z "$TAG" ]; then
        TAG="v0.1.10"
    fi
    URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY}-${OS}-${ARCH}"

    printf "\n"
    printf "${BOLD}${CYAN}uBlock DNS CLI Installer${RESET}\n"
    printf "${DIM}========================${RESET}\n"
    printf " ${BOLD}Version${RESET} : %s\n" "$TAG"
    printf " ${BOLD}OS${RESET}      : %s\n" "$OS"
    printf " ${BOLD}Arch${RESET}    : %s\n" "$ARCH"
    printf " ${BOLD}Profile${RESET} : %s\n" "$PROFILE_ID"
    printf "\n"

    info "Downloading ${BINARY}..."
    TMP_BIN="$(mktemp "/tmp/${BINARY}.XXXXXX")"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --retry 3 --connect-timeout 10 "$URL" -o "$TMP_BIN"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$TMP_BIN" "$URL"
    else
        error "curl or wget required"
        exit 1
    fi
    chmod +x "$TMP_BIN"

    info "Installing to ${INSTALL_DIR}/${BINARY}..."
    run_as_root mv "$TMP_BIN" "${INSTALL_DIR}/${BINARY}"
    TMP_BIN=""
    
    info "Setting up system service..."
    run_as_root "${INSTALL_DIR}/${BINARY}" install -profile "$PROFILE_ID"

    info "Waiting for local DNS proxy to become ready..."
    if ! wait_for_dns_ready; then
        error "DNS validation failed after install. Rolling back..."
        run_as_root "${INSTALL_DIR}/${BINARY}" uninstall || true
        exit 1
    fi

    printf "\n"
    success "Done! uBlock DNS is now active."
    printf "  ${BOLD}Status:${RESET}    ublockdns status\n"
    printf "  ${BOLD}Uninstall:${RESET} sudo ublockdns uninstall\n"
    printf "\n"
}

main "$@"
