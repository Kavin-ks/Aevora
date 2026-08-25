#!/usr/bin/env bash
#
# Aevora — gateway bootstrap (Phase 0)
# ------------------------------------
# Provisions ONE WireGuard exit gateway on a fresh Debian/Ubuntu VPS.
# Provider-agnostic and idempotent: safe to re-run. Run as root.
#
#   sudo ./bootstrap.sh
#
# What it does:
#   1. Installs WireGuard + nftables + qrencode.
#   2. Enables IPv4/IPv6 forwarding, persistently.
#   3. Generates the gateway keypair (once) into /etc/wireguard/keys.
#   4. Writes /etc/wireguard/<iface>.conf with NAT masquerade + MSS clamp.
#   5. Enables and starts wg-quick@<iface>.
#   6. Prints the gateway public key + endpoint for the control plane / clients.
#
# It does NOT add any client peers — use add-peer.sh for that.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="${SCRIPT_DIR}/config.env"
KEY_DIR="/etc/wireguard/keys"

log()  { printf '\033[0;36m[aevora]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[aevora] WARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[0;31m[aevora] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# --- Preconditions -----------------------------------------------------------
[ "$(id -u)" -eq 0 ] || die "must run as root (use sudo)."
[ -f "$CONFIG" ] || die "missing config.env — copy config.env.example to config.env and edit it."
# shellcheck source=/dev/null
. "$CONFIG"

: "${WG_INTERFACE:?set WG_INTERFACE in config.env}"
: "${SERVER_PORT:?set SERVER_PORT in config.env}"
: "${WG_SUBNET_V4:?set WG_SUBNET_V4 in config.env}"
: "${WG_SUBNET_V6:?set WG_SUBNET_V6 in config.env}"

# Gateway addresses are the .1 of each tunnel subnet.
GW_V4="${WG_SUBNET_V4%.*}.1/${WG_SUBNET_V4##*/}"
GW_V6="${WG_SUBNET_V6%%::*}::1/${WG_SUBNET_V6##*/}"

# --- Detect WAN interface ----------------------------------------------------
if [ -z "${WAN_INTERFACE:-}" ]; then
  WAN_INTERFACE="$(ip -4 route show default | awk '/default/ {print $5; exit}')"
  [ -n "$WAN_INTERFACE" ] || die "could not auto-detect WAN interface; set WAN_INTERFACE in config.env."
fi
log "WAN interface: ${WAN_INTERFACE}"

# --- Detect public endpoint --------------------------------------------------
if [ -z "${PUBLIC_ENDPOINT:-}" ]; then
  PUBLIC_IP="$(ip -4 addr show "$WAN_INTERFACE" | awk '/inet / {print $2; exit}' | cut -d/ -f1)"
  PUBLIC_ENDPOINT="$PUBLIC_IP"
  warn "PUBLIC_ENDPOINT not set; using detected IP ${PUBLIC_IP}. Behind NAT? set it manually."
fi
log "Public endpoint: ${PUBLIC_ENDPOINT}:${SERVER_PORT}"

# --- Install packages --------------------------------------------------------
log "Installing packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq wireguard wireguard-tools nftables qrencode iproute2 >/dev/null

# --- Enable forwarding (persistent) -----------------------------------------
log "Enabling IP forwarding..."
cat >/etc/sysctl.d/99-aevora-forward.conf <<'EOF'
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
sysctl -q --system

# --- Generate gateway keypair (once) ----------------------------------------
mkdir -p "$KEY_DIR"
chmod 700 "$KEY_DIR"
if [ ! -f "${KEY_DIR}/server.key" ]; then
  log "Generating gateway keypair..."
  umask 077
  wg genkey | tee "${KEY_DIR}/server.key" | wg pubkey >"${KEY_DIR}/server.pub"
  chmod 600 "${KEY_DIR}/server.key"
  chmod 644 "${KEY_DIR}/server.pub"
else
  log "Gateway keypair already present — keeping it."
fi
SERVER_PRIV="$(cat "${KEY_DIR}/server.key")"
SERVER_PUB="$(cat "${KEY_DIR}/server.pub")"

# --- Write wg config ---------------------------------------------------------
# PostUp/PostDown manage NAT + forwarding + MSS clamp via nftables. The rules
# live in their own table so teardown is a clean `delete table`.
WG_CONF="/etc/wireguard/${WG_INTERFACE}.conf"
if [ -f "$WG_CONF" ]; then
  log "Preserving existing peers from ${WG_CONF}."
  # Keep any [Peer] blocks that add-peer.sh appended; only rewrite [Interface].
  PEERS="$(awk '/^\[Peer\]/{p=1} p{print}' "$WG_CONF")"
else
  PEERS=""
fi

log "Writing ${WG_CONF}..."
umask 077
cat >"$WG_CONF" <<EOF
# Managed by aevora bootstrap.sh — [Interface] is regenerated on each run.
# [Peer] blocks below are managed by add-peer.sh / remove-peer.sh.
[Interface]
Address = ${GW_V4}, ${GW_V6}
ListenPort = ${SERVER_PORT}
PrivateKey = ${SERVER_PRIV}

PostUp = nft -f /etc/wireguard/${WG_INTERFACE}.nft
PostDown = nft delete table inet aevora 2>/dev/null || true
EOF

if [ -n "$PEERS" ]; then
  printf '\n%s\n' "$PEERS" >>"$WG_CONF"
fi
chmod 600 "$WG_CONF"

# --- nftables ruleset --------------------------------------------------------
# Masquerade tunnel traffic out of WAN, allow forwarding between tunnel<->WAN,
# and clamp TCP MSS to the path MTU to avoid black-holing large packets.
log "Writing nftables ruleset..."
cat >"/etc/wireguard/${WG_INTERFACE}.nft" <<EOF
table inet aevora {
  chain forward {
    type filter hook forward priority filter; policy accept;
    tcp flags syn tcp option maxseg size set rt mtu
  }
  chain postrouting {
    type nat hook postrouting priority srcnat; policy accept;
    ip  saddr ${WG_SUBNET_V4} oifname "${WAN_INTERFACE}" masquerade
    ip6 saddr ${WG_SUBNET_V6} oifname "${WAN_INTERFACE}" masquerade
  }
}
EOF

# --- Bring up ----------------------------------------------------------------
log "Enabling wg-quick@${WG_INTERFACE}..."
systemctl enable "wg-quick@${WG_INTERFACE}" >/dev/null 2>&1 || true
if systemctl is-active --quiet "wg-quick@${WG_INTERFACE}"; then
  # Apply config changes live without dropping existing peers.
  wg syncconf "$WG_INTERFACE" <(wg-quick strip "$WG_INTERFACE")
  nft -f "/etc/wireguard/${WG_INTERFACE}.nft"
  log "Reloaded running interface."
else
  systemctl restart "wg-quick@${WG_INTERFACE}"
  log "Started interface."
fi

# --- Persist endpoint for peer generator ------------------------------------
cat >"${SCRIPT_DIR}/.gateway-state" <<EOF
GATEWAY_PUBLIC_KEY=${SERVER_PUB}
GATEWAY_ENDPOINT=${PUBLIC_ENDPOINT}:${SERVER_PORT}
EOF

# --- Summary -----------------------------------------------------------------
cat <<EOF

  ✔ Gateway is up.

    Interface     ${WG_INTERFACE}  (${GW_V4}, ${GW_V6})
    Endpoint      ${PUBLIC_ENDPOINT}:${SERVER_PORT}/udp
    Public key    ${SERVER_PUB}

  Next:
    • Open UDP ${SERVER_PORT} in your provider's firewall/security group.
    • Add a client:   sudo ./add-peer.sh <name>
    • Verify:         sudo ./validate.sh
EOF
