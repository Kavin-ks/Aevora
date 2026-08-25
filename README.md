# Aevora

A self-hosted, subscription-free desktop VPN for a small group — architected to
grow into a real multi-country network. Open the app, pick a country, connect.

> **Status:** Phase 0 — validating the network model on one real gateway.
> No application code yet. Architecture is approved; see the dossier below.

## Architecture

The full architecture dossier (technology picks, control/data-plane split, MVP,
roadmap, cost, risks) is published as an artifact and mirrored in
[`docs/decisions/0001-architecture-baseline.md`](docs/decisions/0001-architecture-baseline.md).

**The one idea to hold onto:** the **control plane** (a small central Go service
+ Postgres) decides *who connects to which server*; the **data plane**
(WireGuard, client → gateway → internet) carries the actual traffic and **never**
passes through the control plane. That split is why it's fast, scalable, and
fault-isolated.

## Repository layout

```
Aevora/
├─ docs/
│  ├─ decisions/            Architecture Decision Records (ADRs)
│  └─ runbooks/             Operational runbooks (start here for Phase 0)
└─ infra/
   └─ gateway/              Provider-agnostic WireGuard exit-gateway kit
      ├─ config.env.example Copy to config.env and edit
      ├─ bootstrap.sh       Provision one gateway (idempotent, run as root)
      ├─ add-peer.sh        Add a client, emit its .conf (+ QR)
      ├─ remove-peer.sh     Remove a client, free its IP
      └─ validate.sh        Check the network model is correct
```

Later phases add `control-plane/` (Go API), `agent/` (gateway node agent), and
`client/` — a **shared Rust core** (via UniFFI) with **native tunnel providers
and UI** per OS, targeting **macOS, Windows, Android and iOS** as first-class
platforms. See [ADR 0001](docs/decisions/0001-architecture-baseline.md).

## Phase 0 — run it

You need a fresh Debian/Ubuntu VPS on any provider (Hetzner/Vultr recommended
for VPN-tolerant, generous egress; see the runbook). Then:

```bash
scp -r infra/gateway user@YOUR_VPS:~/aevora-gateway
ssh user@YOUR_VPS
cd aevora-gateway
cp config.env.example config.env    # edit if you like; defaults are sane
sudo ./bootstrap.sh
sudo ./add-peer.sh my-laptop
sudo ./validate.sh
```

Full step-by-step, client setup for macOS/Linux, and pass/fail criteria:
[`docs/runbooks/phase-0-gateway.md`](docs/runbooks/phase-0-gateway.md).

## Security notes

- Private keys are generated on-device; only public keys reach the server (the
  `add-peer.sh` "bring-your-own-key" mode enforces this — the default
  convenience mode is a Phase-0-only shortcut).
- `config.env`, `clients/`, `.gateway-state`, and all `*.key`/`*.conf` files are
  git-ignored. Never commit secrets.
- No traffic, DNS, or browsing logs — operational metrics only.
