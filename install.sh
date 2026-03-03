#!/bin/sh
set -e

# uBlockDNS installer
# Usage: curl -sSf https://github.com/ugzv/ublockdnsclient/releases/latest/download/install.sh | sh -s -- <profile-id> [account-token]

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

download_binary() {
    attempts=3
    i=1
    while [ "$i" -le "$attempts" ]; do
        info "Download attempt ${i}/${attempts}..."
        if command -v curl >/dev/null 2>&1; then
            if curl -fsSL --connect-timeout 10 "$URL" -o "$TMP_BIN"; then
                return 0
            fi
        elif command -v wget >/dev/null 2>&1; then
            if wget -qO "$TMP_BIN" "$URL"; then
                return 0
            fi
        else
            error "curl or wget required"
            return 1
        fi
        if [ "$i" -lt "$attempts" ]; then
            info "Download failed, retrying in 2s..."
            sleep 2
        fi
        i=$((i + 1))
    done
    return 1
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
        info "Checking local DNS proxy readiness (${i}/${attempts})..."
        if validate_dns; then
            info "Local DNS proxy is responding."
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
    ACCOUNT_TOKEN="${2:-}"
    if [ -z "$PROFILE_ID" ]; then
        error "Usage: curl -sSf https://github.com/ugzv/ublockdnsclient/releases/latest/download/install.sh | sh -s -- <profile-id> [account-token]"
        printf "\n"
        info "Get your profile ID at https://ublockdns.com"
        exit 1
    fi

    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        linux|darwin) ;;
        *)
            error "Unsupported OS: $OS (supported: linux, darwin)"
            exit 1
            ;;
    esac
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    TAG=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    if [ -n "$TAG" ]; then
        URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY}-${OS}-${ARCH}"
    else
        TAG="latest"
        URL="https://github.com/${REPO}/releases/latest/download/${BINARY}-${OS}-${ARCH}"
    fi

    # Detect existing installation
    EXISTING=""
    if [ -x "${INSTALL_DIR}/${BINARY}" ]; then
        EXISTING="true"
    fi

    printf "\n"
    printf "${BOLD}${CYAN}uBlockDNS Installer${RESET}\n"
    printf "${DIM}========================${RESET}\n"
    printf " ${BOLD}Version${RESET} : %s\n" "$TAG"
    printf " ${BOLD}OS${RESET}      : %s\n" "$OS"
    printf " ${BOLD}Arch${RESET}    : %s\n" "$ARCH"
    printf " ${BOLD}Profile${RESET} : %s\n" "$PROFILE_ID"
    if [ -n "$ACCOUNT_TOKEN" ]; then
        printf " ${BOLD}Account token${RESET}: provided (instant rules updates enabled)\n"
    fi
    if [ -n "$EXISTING" ]; then
        CURRENT_VER=$(${INSTALL_DIR}/${BINARY} version 2>/dev/null | head -1 || echo "unknown")
        printf " ${BOLD}Current${RESET} : %s (reinstalling)\n" "$CURRENT_VER"
    fi
    printf "\n"

    # Download FIRST while DNS still works (existing service may be serving DNS)
    info "Downloading ${BINARY}..."
    TMP_BIN="$(mktemp "/tmp/${BINARY}.XXXXXX")"
    if ! download_binary; then
        error "Download failed for ${URL}"
        exit 1
    fi
    chmod +x "$TMP_BIN"

    # Now stop existing service — download already complete, safe to break DNS briefly
    if [ -n "$EXISTING" ]; then
        info "Stopping existing service..."
        run_as_root "${INSTALL_DIR}/${BINARY}" stop 2>/dev/null || true
    fi

    info "Installing to ${INSTALL_DIR}/${BINARY}..."
    run_as_root mv "$TMP_BIN" "${INSTALL_DIR}/${BINARY}"
    TMP_BIN=""

    info "Setting up system service..."
    if [ -n "$ACCOUNT_TOKEN" ]; then
        if ! run_as_root "${INSTALL_DIR}/${BINARY}" install -profile "$PROFILE_ID" -token "$ACCOUNT_TOKEN"; then
            error "Service install failed."
            if [ -n "$EXISTING" ]; then
                printf "${YELLOW}  Tip: try 'sudo ublockdns uninstall' then re-run the installer.${RESET}\n"
            fi
            exit 1
        fi
    elif ! run_as_root "${INSTALL_DIR}/${BINARY}" install -profile "$PROFILE_ID"; then
        error "Service install failed."
        if [ -n "$EXISTING" ]; then
            printf "${YELLOW}  Tip: try 'sudo ublockdns uninstall' then re-run the installer.${RESET}\n"
        fi
        exit 1
    fi

    info "Waiting for local DNS proxy to become ready..."
    if ! wait_for_dns_ready; then
        error "DNS validation timed out - the service may still be starting."
        printf "  Check with: ${BOLD}ublockdns status${RESET}\n"
        printf "  If broken:  ${BOLD}sudo ublockdns uninstall${RESET}\n"
        printf "\n"
        exit 1
    fi

    printf "\n"
    if [ -n "$EXISTING" ]; then
        success "Done! uBlockDNS reinstalled with profile ${PROFILE_ID}."
    else
        success "Done! uBlockDNS is now active."
    fi
    printf "  ${BOLD}Next:${RESET}      Protection is active. Run a status check.\n"
    printf "  ${BOLD}Status:${RESET}    ublockdns status\n"
    printf "  ${BOLD}Uninstall:${RESET} sudo ublockdns uninstall\n"
    printf "\n"
}

main "$@"
