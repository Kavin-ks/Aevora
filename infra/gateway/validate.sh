#!/usr/bin/env bash
#
# Aevora — validate the gateway network model (Phase 0)
# -----------------------------------------------------
#   sudo ./validate.sh
#
# Checks the invariants the full-tunnel data plane depends on, and reports
# live peer handshakes. Run on the GATEWAY after bootstrap.sh. Exits non-zero
# if any hard check fails, so it can gate a CI/provisioning step later.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="${SCRIPT_DIR}/config.env"

[ -f "$CONFIG" ] || { echo "missing config.env"; exit 1; }
# shellcheck source=/dev/null
. "$CONFIG"

pass=0; fail=0
ok()   { printf '  \033[0;32m✔\033[0m %s\n' "$*"; pass=$((pass+1)); }
bad()  { printf '  \033[0;31m✗\033[0m %s\n' "$*"; fail=$((fail+1)); }

echo "Aevora gateway checks (${WG_INTERFACE})"

# 1. IPv4 forwarding
[ "$(sysctl -n net.ipv4.ip_forward)" = "1" ] \
  && ok "IPv4 forwarding enabled" || bad "IPv4 forwarding OFF (net.ipv4.ip_forward != 1)"

# 2. IPv6 forwarding
[ "$(sysctl -n net.ipv6.conf.all.forwarding)" = "1" ] \
  && ok "IPv6 forwarding enabled" || bad "IPv6 forwarding OFF"

# 3. Interface up
if wg show "$WG_INTERFACE" >/dev/null 2>&1; then
  ok "WireGuard interface ${WG_INTERFACE} is up"
else
  bad "WireGuard interface ${WG_INTERFACE} is DOWN (systemctl status wg-quick@${WG_INTERFACE})"
fi

# 4. Listening on the configured port
if wg show "$WG_INTERFACE" listen-port 2>/dev/null | grep -qx "$SERVER_PORT"; then
  ok "Listening on UDP ${SERVER_PORT}"
else
  bad "Not listening on expected port ${SERVER_PORT}"
fi

# 5. NAT masquerade rule present
if nft list table inet aevora 2>/dev/null | grep -q masquerade; then
  ok "NAT masquerade rule present"
else
  bad "NAT masquerade rule missing (check /etc/wireguard/${WG_INTERFACE}.nft)"
fi

# 6. MSS clamp present
if nft list table inet aevora 2>/dev/null | grep -q "maxseg size set rt mtu"; then
  ok "TCP MSS clamp present"
else
  bad "MSS clamp missing — large packets may black-hole on some paths"
fi

# --- Peer / handshake report (informational) --------------------------------
echo
echo "Peers:"
peers="$(wg show "$WG_INTERFACE" peers 2>/dev/null || true)"
if [ -z "$peers" ]; then
  echo "  (none yet — add one with ./add-peer.sh <name>)"
else
  wg show "$WG_INTERFACE" | awk '
    /^peer:/            {print "  peer   " $2}
    /latest handshake/  {sub(/^ +/,""); print "    " $0}
    /transfer:/         {sub(/^ +/,""); print "    " $0}
  '
fi

echo
echo "Result: ${pass} passed, ${fail} failed."
[ "$fail" -eq 0 ] || exit 1
