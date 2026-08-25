#!/usr/bin/env bash
#
# Aevora — add a client peer to the gateway (Phase 0)
# ---------------------------------------------------
#   sudo ./add-peer.sh <name> [client_public_key]
#
# Two modes:
#   • Convenience (no key given): generates the client keypair ON THE GATEWAY,
#     writes a ready-to-use clients/<name>.conf, and prints a QR code.
#     Fast for the manual spike — but the private key briefly exists here.
#
#   • Correct (key given): you generate the keypair on the CLIENT device and
#     pass only its public key. The private key never touches the gateway.
#     This is the model the real control plane will use. Prints the [Interface]
#     block to paste into the client, minus the private key you already hold.
#
# Assigns the next free /32 (+ /128) from the tunnel subnet and adds the peer
# both to the running interface and to the on-disk config so it survives reboot.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="${SCRIPT_DIR}/config.env"
STATE="${SCRIPT_DIR}/.gateway-state"
KEY_DIR="/etc/wireguard/keys"

log()  { printf '\033[0;36m[aevora]\033[0m %s\n' "$*"; }
die()  { printf '\033[0;31m[aevora] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "must run as root (use sudo)."
[ -f "$CONFIG" ] || die "missing config.env."
[ -f "$STATE" ]  || die "missing .gateway-state — run bootstrap.sh first."
# shellcheck source=/dev/null
. "$CONFIG"
# shellcheck source=/dev/null
. "$STATE"

NAME="${1:-}"
CLIENT_PUB_IN="${2:-}"
[ -n "$NAME" ] || die "usage: add-peer.sh <name> [client_public_key]"
[[ "$NAME" =~ ^[a-zA-Z0-9_-]+$ ]] || die "name must be [a-zA-Z0-9_-]."

WG_CONF="/etc/wireguard/${WG_INTERFACE}.conf"
BASE_V4="${WG_SUBNET_V4%.*}"          # e.g. 10.7.0
BASE_V6="${WG_SUBNET_V6%%::*}"        # e.g. fd07:0007
V4_PREFIX="${WG_SUBNET_V4##*/}"

# --- Find next free host octet (2..254; .1 is the gateway) -------------------
used="$(wg show "$WG_INTERFACE" allowed-ips 2>/dev/null | grep -oE "${BASE_V4//./\\.}\.[0-9]+" | awk -F. '{print $NF}' || true)"
octet=""
for i in $(seq 2 254); do
  if ! grep -qx "$i" <<<"$used"; then octet="$i"; break; fi
done
[ -n "$octet" ] || die "tunnel subnet full (max 253 peers on a /24)."
CLIENT_V4="${BASE_V4}.${octet}"
CLIENT_V6="${BASE_V6}::${octet}"
log "Assigning ${CLIENT_V4}/32, ${CLIENT_V6}/128 to '${NAME}'."

# --- Optional pre-shared key for post-quantum-ish extra layer ----------------
PSK="$(wg genpsk)"

# --- Determine client public key ---------------------------------------------
umask 077
mkdir -p "${SCRIPT_DIR}/clients"
if [ -n "$CLIENT_PUB_IN" ]; then
  MODE="bring-your-own-key"
  CLIENT_PUB="$CLIENT_PUB_IN"
  CLIENT_PRIV=""
else
  MODE="gateway-generated"
  CLIENT_PRIV="$(wg genkey)"
  CLIENT_PUB="$(wg pubkey <<<"$CLIENT_PRIV")"
fi

# --- Add peer to running interface + persist ---------------------------------
wg set "$WG_INTERFACE" peer "$CLIENT_PUB" \
  preshared-key <(printf '%s' "$PSK") \
  allowed-ips "${CLIENT_V4}/32,${CLIENT_V6}/128"

{
  printf '\n[Peer]\n'
  printf '# name: %s (added %s)\n' "$NAME" "$(date -u +%FT%TZ)"
  printf 'PublicKey = %s\n' "$CLIENT_PUB"
  printf 'PresharedKey = %s\n' "$PSK"
  printf 'AllowedIPs = %s/32, %s/128\n' "$CLIENT_V4" "$CLIENT_V6"
} >>"$WG_CONF"
log "Peer added to running interface and ${WG_CONF}."

# --- Emit client config ------------------------------------------------------
CLIENT_CONF="${SCRIPT_DIR}/clients/${NAME}.conf"
{
  printf '[Interface]\n'
  if [ "$MODE" = "gateway-generated" ]; then
    printf 'PrivateKey = %s\n' "$CLIENT_PRIV"
  else
    printf '# PrivateKey = <stays on your device — you already have it>\n'
  fi
  printf 'Address = %s/32, %s/128\n' "$CLIENT_V4" "$CLIENT_V6"
  printf 'DNS = %s\n' "$CLIENT_DNS"
  printf 'MTU = %s\n\n' "$CLIENT_MTU"
  printf '[Peer]\n'
  printf 'PublicKey = %s\n' "$GATEWAY_PUBLIC_KEY"
  printf 'PresharedKey = %s\n' "$PSK"
  printf 'Endpoint = %s\n' "$GATEWAY_ENDPOINT"
  printf 'AllowedIPs = 0.0.0.0/0, ::/0\n'
  printf 'PersistentKeepalive = %s\n' "$CLIENT_KEEPALIVE"
} >"$CLIENT_CONF"
chmod 600 "$CLIENT_CONF"

log "Wrote ${CLIENT_CONF}  (mode: ${MODE})."
if [ "$MODE" = "gateway-generated" ] && command -v qrencode >/dev/null; then
  echo
  qrencode -t ansiutf8 <"$CLIENT_CONF"
fi

cat <<EOF

  ✔ Peer '${NAME}' ready.
    Tunnel IP     ${CLIENT_V4} / ${CLIENT_V6}
    Client key    ${CLIENT_PUB}
    Mode          ${MODE}

  Import ${NAME}.conf into the WireGuard app (full-tunnel: AllowedIPs 0.0.0.0/0, ::/0).
EOF
if [ "$MODE" = "gateway-generated" ]; then
  echo "  NOTE: this file holds a private key — transfer it securely and delete it here after."
fi
