#!/bin/sh
set -e

# ─────────────────────────────────────────────────────────────
# uBlockDNS Installer
# Works on: Linux (all distros), macOS
# Usage:    curl -sSf <url>/install.sh | sh -s -- <profile-id> [account-token]
# ─────────────────────────────────────────────────────────────

REPO="ugzv/ublockdnsclient"
BINARY="ublockdns"
INSTALL_DIR="/usr/local/bin"
TMP_BIN=""
TMP_SUMS=""

# ── Terminal colors ──────────────────────────────────────────

setup_colors() {
    if [ -t 1 ]; then
        RED='\033[31m'    GREEN='\033[32m'  YELLOW='\033[33m'
        BLUE='\033[34m'   CYAN='\033[36m'   BOLD='\033[1m'
        DIM='\033[2m'     RESET='\033[0m'
    else
        RED='' GREEN='' YELLOW='' BLUE='' CYAN='' BOLD='' DIM='' RESET=''
    fi
}

info()    { printf "${BLUE}${BOLD}==>${RESET} %s\n" "$*"; }
success() { printf "${GREEN}${BOLD}==>${RESET}${GREEN} %s${RESET}\n" "$*"; }
warn()    { printf "${YELLOW}${BOLD}==>${RESET}${YELLOW} %s${RESET}\n" "$*"; }
error()   { printf "${RED}${BOLD}==>${RESET}${RED} %s${RESET}\n" "$*"; }

# ── Helpers ──────────────────────────────────────────────────

has()         { command -v "$1" >/dev/null 2>&1; }
run_as_root() { if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi; }

cleanup() {
    [ -n "$TMP_BIN" ] && [ -f "$TMP_BIN" ] && rm -f "$TMP_BIN"
    [ -n "$TMP_SUMS" ] && [ -f "$TMP_SUMS" ] && rm -f "$TMP_SUMS"
}

# ── Download with retries ───────────────────────────────────

download() {
    url="$1" dest="$2" attempts=3 i=1

    while [ "$i" -le "$attempts" ]; do
        info "Downloading (attempt ${i}/${attempts})..."

        if has curl; then
            curl -fsSL --connect-timeout 10 "$url" -o "$dest" && return 0
        elif has wget; then
            wget -qO "$dest" "$url" && return 0
        else
            error "Either curl or wget is required."; return 1
        fi

        [ "$i" -lt "$attempts" ] && { warn "Retrying in 2s..."; sleep 2; }
        i=$((i + 1))
    done

    return 1
}

verify_sha256() {
    file="$1"
    expected="$2"

    if has sha256sum; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif has shasum; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    elif has openssl; then
        actual=$(openssl dgst -sha256 "$file" | awk '{print $NF}')
    else
        error "No SHA-256 tool found (need sha256sum, shasum, or openssl)."
        return 1
    fi

    [ "$actual" = "$expected" ]
}

# ── DNS validation ──────────────────────────────────────────

# Run a command silently, suppressing even shell-level crash messages
quiet_run() { ("$@" >/dev/null 2>&1) 2>/dev/null; }

validate_dns() {
    found_tool=0

    # Try tools that can query 127.0.0.1 directly
    if has dig; then
        found_tool=1
        quiet_run dig @127.0.0.1 example.com +short +time=2 +tries=1 && return 0
    fi
    if has nslookup; then
        found_tool=1
        quiet_run nslookup -timeout=2 example.com 127.0.0.1 && return 0
    fi
    if has host; then
        found_tool=1
        quiet_run host -W 2 example.com 127.0.0.1 && return 0
    fi
    if has drill; then
        found_tool=1
        quiet_run drill @127.0.0.1 example.com && return 0
    fi

    # Fallback: use system resolver after install has pointed it at 127.0.0.1
    if has getent; then
        found_tool=1
        quiet_run getent hosts example.com && return 0
    fi

    # macOS
    if has dscacheutil; then
        found_tool=1
        quiet_run dscacheutil -q host -a name example.com && return 0
    fi

    # No DNS tools installed at all — can't validate, assume ok
    if [ "$found_tool" -eq 0 ]; then
        warn "No DNS lookup tools found — skipping validation."
        return 0
    fi

    return 1
}

wait_for_dns() {
    info "Waiting for DNS proxy to become ready..."
    i=1
    while [ "$i" -le 15 ]; do
        if validate_dns; then
            success "DNS proxy is responding."
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    return 1
}

# ── Write /etc/resolv.conf safely ───────────────────────────

write_resolv_conf() {
    # Remove immutable flag if set (from a previous install)
    if has chattr; then
        run_as_root chattr -i /etc/resolv.conf 2>/dev/null || true
    fi

    # Remove symlink if present (common with systemd-resolved stub)
    if [ -L /etc/resolv.conf ]; then
        run_as_root rm -f /etc/resolv.conf
    fi

    run_as_root tee /etc/resolv.conf >/dev/null <<EOF
# Managed by uBlockDNS — do not edit
nameserver 127.0.0.1
EOF

    # Prevent other services from overwriting resolv.conf
    if has chattr; then
        run_as_root chattr +i /etc/resolv.conf 2>/dev/null || true
    fi
}

# ── Linux DNS configuration (all distros) ───────────────────

configure_linux_dns() {
    # Backup original resolv.conf (first install only)
    if [ -f /etc/resolv.conf ] && [ ! -f /etc/resolv.conf.ublockdns.bak ]; then
        info "Backing up /etc/resolv.conf"
        if [ -L /etc/resolv.conf ]; then
            run_as_root cp -P /etc/resolv.conf /etc/resolv.conf.ublockdns.bak
        else
            run_as_root cp /etc/resolv.conf /etc/resolv.conf.ublockdns.bak
        fi
    fi

    dns_manager="none"

    # Detect what manages DNS on this system
    if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
        dns_manager="systemd-resolved"
    elif systemctl is-active --quiet NetworkManager 2>/dev/null; then
        dns_manager="networkmanager"
    elif systemctl is-active --quiet connman 2>/dev/null; then
        dns_manager="connman"
    elif [ -d /etc/dhclient.d ] || [ -f /etc/dhcp/dhclient.conf ] || [ -f /etc/dhclient.conf ]; then
        dns_manager="dhclient"
    elif has resolvconf; then
        dns_manager="resolvconf"
    fi

    info "Detected DNS manager: ${dns_manager}"

    case "$dns_manager" in

        systemd-resolved)
            run_as_root mkdir -p /etc/systemd/resolved.conf.d
            run_as_root tee /etc/systemd/resolved.conf.d/ublockdns.conf >/dev/null <<EOF
[Resolve]
DNS=127.0.0.1
DNSStubListener=no
EOF
            run_as_root systemctl restart systemd-resolved
            write_resolv_conf
            success "Configured systemd-resolved."
            ;;

        networkmanager)
            # Tell NetworkManager to stop managing /etc/resolv.conf
            run_as_root mkdir -p /etc/NetworkManager/conf.d
            run_as_root tee /etc/NetworkManager/conf.d/ublockdns.conf >/dev/null <<EOF
[main]
dns=none
EOF
            # Write resolv.conf BEFORE restarting NM — dns=none means NM won't touch it
            write_resolv_conf
            run_as_root systemctl restart NetworkManager 2>/dev/null || true
            success "Configured NetworkManager."
            ;;

        connman)
            run_as_root mkdir -p /etc/connman
            if [ -f /etc/connman/main.conf ]; then
                if grep -q '^\[General\]' /etc/connman/main.conf 2>/dev/null; then
                    if ! grep -q 'DNSProxy' /etc/connman/main.conf 2>/dev/null; then
                        run_as_root sed -i '/^\[General\]/a DNSProxy=none' /etc/connman/main.conf
                    fi
                else
                    printf '[General]\nDNSProxy=none\n' | run_as_root tee -a /etc/connman/main.conf >/dev/null
                fi
            else
                printf '[General]\nDNSProxy=none\n' | run_as_root tee /etc/connman/main.conf >/dev/null
            fi
            write_resolv_conf
            run_as_root systemctl restart connman 2>/dev/null || true
            success "Configured ConnMan."
            ;;

        dhclient)
            dhclient_conf=""
            for f in /etc/dhcp/dhclient.conf /etc/dhclient.conf; do
                [ -f "$f" ] && dhclient_conf="$f" && break
            done
            if [ -n "$dhclient_conf" ]; then
                if ! grep -q 'supersede domain-name-servers 127.0.0.1' "$dhclient_conf" 2>/dev/null; then
                    printf '\n# uBlockDNS\nsupersede domain-name-servers 127.0.0.1;\n' | run_as_root tee -a "$dhclient_conf" >/dev/null
                fi
            fi
            write_resolv_conf
            success "Configured dhclient."
            ;;

        resolvconf)
            run_as_root mkdir -p /etc/resolvconf/resolv.conf.d
            printf 'nameserver 127.0.0.1\n' | run_as_root tee /etc/resolvconf/resolv.conf.d/head >/dev/null
            run_as_root resolvconf -u 2>/dev/null || true
            write_resolv_conf
            success "Configured resolvconf."
            ;;

        none)
            write_resolv_conf
            success "Configured /etc/resolv.conf directly."
            ;;
    esac
}

# ── Main ─────────────────────────────────────────────────────

main() {
    trap cleanup EXIT
    setup_colors

    # ── Parse arguments ──────────────────────────────────────

    PROFILE_ID="${1:-}"
    ACCOUNT_TOKEN="${2:-}"

    if [ -z "$PROFILE_ID" ]; then
        printf "\n"
        printf "${BOLD}${CYAN}uBlockDNS${RESET}\n"
        printf "\n"
        error "Missing profile ID."
        printf "\n"
        printf "  ${BOLD}Usage:${RESET}\n"
        printf "    curl -sSf https://github.com/${REPO}/releases/latest/download/install.sh | sh -s -- ${DIM}<profile-id>${RESET}\n"
        printf "\n"
        printf "  Get your profile ID at ${BOLD}https://ublockdns.com${RESET}\n"
        printf "\n"
        exit 1
    fi

    # ── Detect OS and architecture ───────────────────────────

    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        linux|darwin) ;;
        *) error "Unsupported OS: $OS (supported: linux, macOS)"; exit 1 ;;
    esac

    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64" ;;
        aarch64|arm64)  ARCH="arm64" ;;
        armv7*|armhf)   ARCH="armv7" ;;
        *) error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    # ── Fetch latest release tag ─────────────────────────────

    TAG=""
    if has curl; then
        TAG=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | cut -d'"' -f4) || true
    elif has wget; then
        TAG=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | head -1 | cut -d'"' -f4) || true
    fi

    if [ -n "$TAG" ]; then
        URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY}-${OS}-${ARCH}"
        SUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/SHA256SUMS"
    else
        TAG="latest"
        URL="https://github.com/${REPO}/releases/latest/download/${BINARY}-${OS}-${ARCH}"
        SUMS_URL="https://github.com/${REPO}/releases/latest/download/SHA256SUMS"
    fi

    # ── Check for existing installation ──────────────────────

    EXISTING=""
    [ -x "${INSTALL_DIR}/${BINARY}" ] && EXISTING="true"

    # ── Print banner ─────────────────────────────────────────

    printf "\n"
    printf "${BOLD}${CYAN}uBlockDNS Installer${RESET}\n"
    printf "${DIM}────────────────────────${RESET}\n"
    printf "  ${BOLD}Version${RESET}  %s\n" "$TAG"
    printf "  ${BOLD}OS${RESET}       %s/%s\n" "$OS" "$ARCH"
    printf "  ${BOLD}Profile${RESET}  %s\n" "$PROFILE_ID"
    [ -n "$ACCOUNT_TOKEN" ] && printf "  ${BOLD}Token${RESET}    provided\n"
    if [ -n "$EXISTING" ]; then
        CURRENT_VER=$("${INSTALL_DIR}/${BINARY}" version 2>/dev/null | head -1 || echo "unknown")
        printf "  ${BOLD}Current${RESET}  %s ${DIM}(reinstalling)${RESET}\n" "$CURRENT_VER"
    fi
    printf "\n"

    # ── Download binary (while DNS still works) ──────────────

    TMP_BIN="$(mktemp "/tmp/${BINARY}.XXXXXX")"
    TMP_SUMS="$(mktemp "/tmp/${BINARY}.sums.XXXXXX")"
    if ! download "$URL" "$TMP_BIN"; then
        error "Download failed: ${URL}"
        exit 1
    fi
    if ! download "$SUMS_URL" "$TMP_SUMS"; then
        error "Download failed: ${SUMS_URL}"
        exit 1
    fi

    ASSET_NAME="${BINARY}-${OS}-${ARCH}"
    EXPECTED_SHA256=$(awk -v asset="$ASSET_NAME" '$2==asset || $2=="*"asset {print $1; exit}' "$TMP_SUMS")
    if [ -z "$EXPECTED_SHA256" ]; then
        error "Could not find checksum for ${ASSET_NAME} in SHA256SUMS."
        exit 1
    fi
    if ! verify_sha256 "$TMP_BIN" "$EXPECTED_SHA256"; then
        error "SHA-256 verification failed for ${ASSET_NAME}."
        exit 1
    fi
    success "Checksum verified for ${ASSET_NAME}."
    chmod +x "$TMP_BIN"

    # ── Stop existing service ────────────────────────────────

    if [ -n "$EXISTING" ]; then
        info "Stopping existing service..."
        run_as_root "${INSTALL_DIR}/${BINARY}" stop 2>/dev/null || true
    fi

    # Unlock resolv.conf so the binary can write to it during install
    if has chattr; then
        run_as_root chattr -i /etc/resolv.conf 2>/dev/null || true
    fi

    # ── Install binary ───────────────────────────────────────

    info "Installing to ${INSTALL_DIR}/${BINARY}..."
    run_as_root mkdir -p "$INSTALL_DIR"
    run_as_root mv "$TMP_BIN" "${INSTALL_DIR}/${BINARY}"
    TMP_BIN=""

    # ── Register system service ──────────────────────────────

    info "Setting up system service..."
    install_args="-profile $PROFILE_ID"
    [ -n "$ACCOUNT_TOKEN" ] && install_args="$install_args -token $ACCOUNT_TOKEN"

    if ! run_as_root "${INSTALL_DIR}/${BINARY}" install $install_args; then
        error "Service installation failed."
        [ -n "$EXISTING" ] && warn "Tip: run 'sudo ublockdns uninstall' first, then retry."
        exit 1
    fi

    # ── Configure system DNS ─────────────────────────────────

    if [ "$OS" = "linux" ]; then
        configure_linux_dns
    fi

    # ── Verify DNS is working ────────────────────────────────

    if ! wait_for_dns; then
        warn "uBlockDNS did not validate local DNS resolution."
        warn "Current machine status:"
        run_as_root "${INSTALL_DIR}/${BINARY}" status -json || true
        printf "  Check:     ${BOLD}ublockdns status${RESET}\n"
        printf "  Uninstall: ${BOLD}sudo ublockdns uninstall${RESET}\n"
        printf "\n"
        exit 1
    fi

    # ── Done ─────────────────────────────────────────────────

    printf "\n"
    if [ -n "$EXISTING" ]; then
        success "uBlockDNS reinstalled with profile ${PROFILE_ID}."
    else
        success "uBlockDNS is now active."
    fi
    printf "\n"
    printf "  ${BOLD}Status${RESET}     ublockdns status\n"
    printf "  ${BOLD}Uninstall${RESET}  sudo ublockdns uninstall\n"
    printf "\n"
}

main "$@"
