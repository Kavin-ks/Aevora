# Deployment

How to run Aevora, from local development to production. Development and
production configuration are kept clearly separate.

## Components

| Component | Runs on | Purpose |
|-----------|---------|---------|
| `controld` | 1 small VM (or N behind a LB) | Control-plane API + Postgres |
| PostgreSQL | with controld, or managed | Source of truth |
| `aevora-agent` | each gateway VM | Registers, heartbeats, programs WireGuard peers |
| WireGuard | each gateway VM | The data plane (Phase 0 kit) |
| Caddy | with controld | TLS termination + reverse proxy |

## Local development

```bash
cd deploy
docker compose up --build          # Postgres + controld on :8080 (dev seed)
curl localhost:8080/v1/locations
```

`deploy/docker-compose.yml` is **development** only: it enables `AEVORA_DEV_SEED`
(placeholder locations/gateways) and uses a throwaway Postgres password. Do not
use it in production.

Rust core tests and the control plane/agent tests:

```bash
cd control-plane && go test ./...
cd agent && go test ./...
cd client/core && cargo test
```

## Production control plane

Prerequisites: a small Linux VM with Docker, a domain pointing at it (A/AAAA),
ports 80 + 443 open.

```bash
cd deploy/production
cp .env.prod.example .env
# Fill in: AEVORA_DOMAIN, POSTGRES_PASSWORD, AEVORA_JWT_SECRET,
#          AEVORA_ENROLLMENT_SECRET, AEVORA_ADMIN_TOKEN
#   generate secrets with:  openssl rand -base64 32
docker compose -f docker-compose.prod.yml up -d --build
```

Caddy provisions a Let's Encrypt certificate automatically and reverse-proxies
to `controld` (which runs with `AEVORA_TRUST_PROXY=true`, so rate limiting sees
the real client IP). `controld` is not published to the host; only Caddy is
public. `/metrics` is blocked at the proxy — scrape it from the private network.

### Scaling the control plane

`controld` is stateless (all state is in Postgres), so it scales horizontally:
run N replicas behind a load balancer and point them at a shared (managed or
replicated) Postgres. The only per-instance state is the in-memory rate limiter,
which is a best-effort first line of defence; a shared limiter (Redis) can
replace it later.

## Gateways

Each gateway is an ordinary Linux VM on any provider (Hetzner, Vultr, …). Two
steps:

1. **WireGuard** — provision it with the Phase 0 kit:
   see [`infra/gateway`](../infra/gateway) and
   [`runbooks/phase-0-gateway.md`](runbooks/phase-0-gateway.md).
2. **Node agent** — build and install it:

   ```bash
   cd agent && GOOS=linux GOARCH=amd64 go build -o aevora-agent ./cmd/aevora-agent
   scp aevora-agent root@GATEWAY:/usr/local/bin/
   # on the gateway:
   install -m600 deploy/systemd/aevora-agent.env.example /etc/aevora/agent.env  # edit it
   cp deploy/systemd/aevora-agent.service /etc/systemd/system/
   systemctl enable --now aevora-agent
   ```

The agent self-registers using `AEVORA_ENROLLMENT_SECRET`, caches its node token
at `AEVORA_NODE_TOKEN_FILE`, then heartbeats and reconciles peers. See
[OPERATIONS](OPERATIONS.md) for adding countries/gateways.

## Environment variables

### Control plane (`controld`)

| Variable | Default | Notes |
|----------|---------|-------|
| `AEVORA_LISTEN_ADDR` | `:8080` | Listen address |
| `AEVORA_DB_URL` | — | Postgres DSN (required) |
| `AEVORA_JWT_SECRET` | — | Signs access tokens. **Required for user auth** |
| `AEVORA_ENROLLMENT_SECRET` | — | Gateway self-registration. Empty ⇒ registration disabled |
| `AEVORA_ADMIN_TOKEN` | — | Admin endpoints. Empty ⇒ admin views disabled |
| `AEVORA_ACCESS_TTL` | `15m` | Access-token lifetime |
| `AEVORA_REFRESH_TTL` | `720h` | Refresh-token lifetime |
| `AEVORA_LEASE_TTL` | `5m` | Connection lease before renewal |
| `AEVORA_HEARTBEAT_TTL` | `30s` | Gateway offline threshold |
| `AEVORA_REAPER_INTERVAL` | `10s` | Reaper cadence |
| `AEVORA_CLIENT_DNS` | `9.9.9.9,149.112.112.112` | DNS pushed to clients |
| `AEVORA_AUTH_RATE_PER_MIN` | `10` | Per-IP auth rate |
| `AEVORA_AUTH_BURST` | `5` | Per-IP auth burst |
| `AEVORA_TRUST_PROXY` | `false` | Behind a proxy: honor `X-Forwarded-For` |
| `AEVORA_DEV_SEED` | `false` | **Dev only** — insert placeholder data |

### Node agent — see [`deploy/systemd/aevora-agent.env.example`](../deploy/systemd/aevora-agent.env.example).

### Clients — control-plane URL is a build setting, never hardcoded:
Apple `AEVORA_CONTROL_URL` (Info.plist), Android `-PaevoraControlUrl` (BuildConfig).

## Database

- **Dev:** Postgres in the compose stack (ephemeral volume).
- **Prod:** the compose Postgres with a persistent volume, **or** a managed
  Postgres (recommended for HA + backups). Migrations run automatically on
  `controld` startup (embedded, idempotent).
- **Backups:** `pg_dump` on a schedule (the DB holds only metadata — users,
  devices, locations, gateways, leases — no traffic). Test restores.

## Production readiness checklist

- [ ] TLS enforced (Caddy) for all client ↔ control-plane traffic.
- [ ] Strong random `JWT`, `ENROLLMENT`, `ADMIN` secrets; not in git.
- [ ] `AEVORA_DEV_SEED` unset; no placeholder gateways in the DB.
- [ ] `AEVORA_TRUST_PROXY=true` only behind a proxy you control.
- [ ] Postgres persisted + backed up; restore tested.
- [ ] `/metrics` not publicly reachable; Prometheus scrapes privately.
- [ ] At least 2 gateways per popular country (failover).
- [ ] Gateway provider allows VPN egress; bandwidth monitored.
- [ ] Node token files (`/etc/aevora/node.token`) are `chmod 600`.
- [ ] Client apps signed/notarised (Apple) / signed (Windows) / Play-signed.
- [ ] Acceptable-use policy in place (exit-node responsibility).
