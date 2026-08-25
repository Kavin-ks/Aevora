# Phase 1 — Control plane + node agent

The control plane is the brain: it authenticates clients, keeps the registry of
gateways, scores them for selection, and manages the peer/lease lifecycle. It is
**never in the packet path** (see [ADR 0001](../decisions/0001-architecture-baseline.md)).

This document is the working spec for Phase 1. It is built incrementally; each
increment is demoable and tested.

## Components

| Component | Binary | Role |
|-----------|--------|------|
| Control plane API | `aevora-controld` | Go HTTP/JSON service over Postgres |
| Node agent | `aevora-agent` | Runs on each gateway: register, heartbeat, apply peers |
| Database | PostgreSQL | Source of truth |

## Data model (Postgres)

```
users        ── invited people
  └─ devices ── one per client install; holds the WireGuard PUBLIC key
locations    ── a "country" label users pick (de, sg, …)
  └─ gateways ── exit nodes; MANY per location; live load/health columns
        └─ leases ── an active connection: a peer + a leased /32 on a gateway
invite_codes ── simple invite-only onboarding
```

Key invariants:

- Only the client **public** key is ever stored (`devices.public_key`,
  `leases.client_public_key`). Private keys never reach the server.
- A gateway's live fitness (`active_peers`, `cpu_pct`, `rx/tx_bps`,
  `last_heartbeat_at`, `status`) is updated by the agent heartbeat.
- One active lease per `(gateway_id, assigned_ip_v4)` — enforced by a partial
  unique index, so IP allocation cannot double-assign.
- A stale lease is reclaimed by `expires_at` (TTL), which self-heals crashed
  clients and dead phones.

## API surface (v1)

| Method &amp; path | Auth | Purpose | Increment |
|---------------|------|---------|-----------|
| `GET /healthz` | none | Liveness/readiness | ✅ 1a |
| `GET /v1/locations` | user | Countries + availability (healthy server count) | ✅ 1a |
| `POST /v1/enroll` | invite | Create user + first device, return tokens | ✅ 1b |
| `POST /v1/auth/refresh` | refresh token | Exchange for a new access token | ✅ 1b |
| `POST /v1/devices` | user | Register another device, return its refresh token | ✅ 1b |
| `GET /v1/devices` | user | List the caller's devices | ✅ 1b |
| `DELETE /v1/devices/{id}` | user | Revoke a device (+ its refresh tokens) | ✅ 1b |
| `POST /v1/invites` | admin token | Mint an invite code | ✅ 1b |
| `POST /v1/gateways/register` | enrollment secret | Agent registers a gateway, gets a node token | ✅ 1c |
| `POST /v1/gateways/heartbeat` | node token | Agent reports load/health, marks healthy | ✅ 1c |
| `POST /v1/gateways/deregister` | node token | Clean shutdown: mark gateway disabled | ✅ 1c |
| `GET /v1/gateways` | admin token | Fleet listing with online/offline + load | ✅ 1c |
| `POST /v1/connections` | user | Select gateway → lease /32 → return tunnel config | ✅ 1d |
| `DELETE /v1/connections/{id}` | user | Disconnect: release the lease | ✅ 1d |
| `POST /v1/connections/{id}/stats` | user | Client stats + lease keep-alive (renew) | ✅ 1d |
| `GET /v1/gateways/peers` | node token | Agent fetches its desired peer set to reconcile | ✅ 1d |

## Server selection (increment 1d)

Pure, testable scoring (`internal/selection`): among **healthy** gateways in the
requested location, pick the one with the most spare capacity (lowest
`active_peers / capacity`), later weighted by client latency probes. Unhealthy or
stale-heartbeat gateways are excluded outright.

## Increment plan

- **1a — foundation (this increment):** module, config, Postgres schema +
  embedded migrator, store layer, `GET /healthz` + `GET /v1/locations`,
  docker-compose dev stack, unit tests (handlers + selection) that need no DB.
- **1b — identity (done):** invite minting (admin), invite-gated enrollment
  (creates user + first device), short-lived **HS256 access JWT** (stdlib, no
  dependency; `alg` pinned) plus **refresh tokens** (stored hashed) for session
  continuity, device registration/listing/revocation (revoking a device revokes
  its refresh tokens). All user endpoints require a valid access token; the
  whole feature is disabled unless `AEVORA_JWT_SECRET` is set. Verified against
  Postgres 15: enroll → refresh → add/list/revoke device, plus used-invite (403),
  duplicate device key (409), and revoked-token (401) paths. Unit tests DB-free.
- **1c — fleet (done):** gateway self-registration (issues a hashed node token),
  metadata (country/city/region/endpoint/capacity + geo &amp; bandwidth hints for
  future selection), heartbeat, health status, a reaper enforcing the heartbeat
  TTL (online/offline), admin fleet listing, deregistration, multiple gateways
  per country (new countries created implicitly on register), and
  `SelectableGateways` — the healthy-in-location candidate query feeding the
  pure `selection` policy. Registration/admin are disabled unless their secrets
  are configured. Verified against Postgres 15 (migrations, seed, full
  register→heartbeat→reaper→deregister flow); unit tests are DB-free.
- **1d — connections (done):** `POST /v1/connections` selects the least-loaded
  healthy gateway in the country (load = active lease count), leases a free
  `/32`+`/128` under a per-gateway row lock (no double-allocation), and returns
  the tunnel config (server endpoint/public key, assigned IPs, DNS, full-tunnel
  allowed-IPs). `stats` renews the lease (keep-alive); a lease reaper expires
  stale leases so their peers are removed. The **node agent** (`agent/`, its own
  module) self-registers, heartbeats, fetches `GET /v1/gateways/peers`, and
  reconciles the WireGuard interface by shelling out to `wg` — a **pull model**
  (no inbound to gateways, no agent secret stored). The reconcile diff and the
  `wg` dump parser are pure and unit-tested; the client is tested against an
  httptest server. Control-plane flow verified against Postgres 15 (connect →
  peers → renew → disconnect → reaper), including two-device IP allocation and
  the no-gateway (503) path.

### Peer programming: pull/reconcile (not push)

The control plane records intent (leases); the agent reconciles reality. It is
the DB's `active` leases, exposed at `GET /v1/gateways/peers`, that the agent
diffs against the live interface. This needs no inbound connectivity to gateways
(NAT/firewall friendly) and stores no agent callback credentials. The trade-off
is up to one sync interval of setup latency; WireGuard's handshake retry covers
it, and a future long-poll/stream can cut it further.

## Local dev

```bash
cd deploy && docker compose up --build      # Postgres + controld on :8080
curl localhost:8080/healthz
curl localhost:8080/v1/locations            # seeded dev data
```
