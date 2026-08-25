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
| `POST /v1/enroll` | invite | Create user + first device | 1b |
| `POST /v1/devices` | user | Register a device public key | 1b |
| `POST /v1/gateways/register` | enrollment secret | Agent registers a gateway, gets a node token | ✅ 1c |
| `POST /v1/gateways/heartbeat` | node token | Agent reports load/health, marks healthy | ✅ 1c |
| `POST /v1/gateways/deregister` | node token | Clean shutdown: mark gateway disabled | ✅ 1c |
| `GET /v1/gateways` | admin token | Fleet listing with online/offline + load | ✅ 1c |
| `POST /v1/connections` | user+device | Select gateway → lease /32 → program peer → return config | 1d |
| `DELETE /v1/connections/{id}` | user+device | Tear down: release lease, remove peer | 1d |
| `POST /v1/connections/{id}/stats` | user+device | Client posts throughput/latency samples | 1d |

## Server selection (increment 1d)

Pure, testable scoring (`internal/selection`): among **healthy** gateways in the
requested location, pick the one with the most spare capacity (lowest
`active_peers / capacity`), later weighted by client latency probes. Unhealthy or
stale-heartbeat gateways are excluded outright.

## Increment plan

- **1a — foundation (this increment):** module, config, Postgres schema +
  embedded migrator, store layer, `GET /healthz` + `GET /v1/locations`,
  docker-compose dev stack, unit tests (handlers + selection) that need no DB.
- **1b — identity:** invite enrollment, users, device registration, JWT.
- **1c — fleet (done):** gateway self-registration (issues a hashed node token),
  metadata (country/city/region/endpoint/capacity + geo &amp; bandwidth hints for
  future selection), heartbeat, health status, a reaper enforcing the heartbeat
  TTL (online/offline), admin fleet listing, deregistration, multiple gateways
  per country (new countries created implicitly on register), and
  `SelectableGateways` — the healthy-in-location candidate query feeding the
  pure `selection` policy. Registration/admin are disabled unless their secrets
  are configured. Verified against Postgres 15 (migrations, seed, full
  register→heartbeat→reaper→deregister flow); unit tests are DB-free.
- **1d — connections:** the connect/lease/peer lifecycle + the node agent that
  applies peers via `wgctrl`, wired to the Phase 0 gateway.

## Local dev

```bash
cd deploy && docker compose up --build      # Postgres + controld on :8080
curl localhost:8080/healthz
curl localhost:8080/v1/locations            # seeded dev data
```
