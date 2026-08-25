#!/usr/bin/env bash
#
# Aevora — remove a client peer (Phase 0)
# ---------------------------------------
#   sudo ./remove-peer.sh <name>
#
# Removes the peer from the running interface and from the on-disk config,
# and deletes the generated client config if present. Frees its tunnel IP.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="${SCRIPT_DIR}/config.env"

log()  { printf '\033[0;36m[aevora]\033[0m %s\n' "$*"; }
die()  { printf '\033[0;31m[aevora] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "must run as root (use sudo)."
[ -f "$CONFIG" ] || die "missing config.env."
# shellcheck source=/dev/null
. "$CONFIG"

NAME="${1:-}"
[ -n "$NAME" ] || die "usage: remove-peer.sh <name>"
WG_CONF="/etc/wireguard/${WG_INTERFACE}.conf"

# Find the public key for the [Peer] block whose "# name:" matches.
PUBKEY="$(awk -v n="$NAME" '
  /^\[Peer\]/ {inblk=1; blkkey=""; match_=0}
  inblk && $0 ~ ("# name: " n " ") {match_=1}
  inblk && /^PublicKey/ {blkkey=$3}
  inblk && match_ && blkkey!="" {print blkkey; exit}
' "$WG_CONF" || true)"

[ -n "$PUBKEY" ] || die "no peer named '${NAME}' found in ${WG_CONF}."

# Remove from running interface.
wg set "$WG_INTERFACE" peer "$PUBKEY" remove
log "Removed '${NAME}' (${PUBKEY}) from running interface."

# Rewrite config without that [Peer] block.
tmp="$(mktemp)"
awk -v key="$PUBKEY" '
  /^\[Peer\]/ {
    # Buffer a peer block; decide to keep once we see its PublicKey.
    block=$0"\n"; inblk=1; drop=0; next
  }
  inblk {
    block=block $0 "\n"
    if ($1=="PublicKey" && $3==key) drop=1
    if ($0 ~ /^$/) { if(!drop) printf "%s", block; inblk=0; block="" }
    next
  }
  { print }
  END { if (inblk && !drop) printf "%s", block }
' "$WG_CONF" >"$tmp"
mv "$tmp" "$WG_CONF"
chmod 600 "$WG_CONF"
log "Rewrote ${WG_CONF}."

rm -f "${SCRIPT_DIR}/clients/${NAME}.conf"
log "Done. Tunnel IP freed."
