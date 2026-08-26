# Operations

Day-to-day running: adding countries and gateways, minting invites, monitoring,
and troubleshooting. All examples assume `$CP` is your control-plane base URL and
`$ADMIN` is `AEVORA_ADMIN_TOKEN`.

## Onboard a user

Mint an invite (admin), then the user enrolls from the app:

```bash
curl -sX POST $CP/v1/invites -H "Authorization: Bearer $ADMIN" -d '{"note":"dana"}'
# -> {"code":"<invite>"}
```

The client calls `enroll(invite, email, ...)`. Optionally the user sets a
password (`POST /v1/auth/password`) to log in on another device later
(`POST /v1/auth/login`).

## Add a new country

A country is created **implicitly** the first time a gateway registers with a
new `country_code`. So "adding a country" = bringing up a gateway there:

1. Provision a VM in the new country; run the Phase 0 WireGuard kit.
2. Configure the agent (`/etc/aevora/agent.env`) with the new
   `AEVORA_COUNTRY_CODE` / `AEVORA_COUNTRY_NAME` and endpoint/subnet.
3. `systemctl start aevora-agent`.

It appears in every client's location list automatically — **no client update**.

## Add another gateway to an existing country

Identical: provision a second VM, give it a unique `AEVORA_GATEWAY_NAME` (e.g.
`de-fra-2`) and a distinct `AEVORA_WG_SUBNET_V4`, and start the agent. The
control plane load-balances across gateways in a country by health + load +
bandwidth, and fails over if one goes unhealthy.

## Verify the fleet

```bash
curl -s $CP/v1/gateways -H "Authorization: Bearer $ADMIN" | jq
```

Each gateway shows `status`, `online`, `active_peers`/`capacity`, load, and
endpoint. A healthy, heartbeating gateway is selectable.

## Remove / retire a gateway

- **Graceful:** stop the agent (`systemctl stop aevora-agent`) — it deregisters
  (status `disabled`); the reaper expires its leases so clients reconnect
  elsewhere.
- **Hard failure:** stop heartbeating → the reaper marks it `unhealthy` after
  `AEVORA_HEARTBEAT_TTL` and expires its leases (failover). Then destroy the VM.

## Monitoring

- `/metrics` (Prometheus): `aevora_gateways{status}`, `aevora_active_sessions`,
  `aevora_connections_total{result}`, `aevora_auth_total{result}`. Scrape from a
  private network (it is blocked at the public proxy).
- Structured JSON logs from `controld` and the agent (never contain tokens,
  passwords, or private keys).
- Suggested alerts: healthy gateways in a country drops to 0; connection error
  rate rises; a gateway's active_peers approaches capacity.

## Troubleshooting

| Symptom | Likely cause | Check |
|--------|--------------|-------|
| Client can't connect, 503 "no gateway" | No healthy gateway in that country | `GET /v1/gateways`; is the agent heartbeating? |
| Tunnel comes up but no internet | Gateway NAT/forwarding | `infra/gateway/validate.sh` on the gateway |
| Connect succeeds, handshake never completes | Peer not programmed yet, or UDP blocked | Agent reconcile interval; provider firewall UDP 51820 |
| Login always 401 | No password set, or wrong | User must `POST /v1/auth/password` first |
| 429 on auth | Rate limit hit | Expected under bursty/abuse; tune `AEVORA_AUTH_*` |
| Gateway shows `unhealthy` | Missed heartbeats | Agent logs; clock skew; network to control plane |
| Orphaned peer suspected | — | Peers come only from active leases; reconnect/reaper clean up. `GET /v1/gateways/peers` (node token) shows the desired set |

## Rotating secrets

- **JWT secret:** rotating invalidates all live access tokens (clients refresh).
- **Enrollment secret:** rotate and update each gateway's `agent.env`; existing
  node tokens keep working (they are separate).
- **Admin token:** rotate freely; only affects admin tooling.
