# Aevora

A self-hosted, subscription-free VPN with a clean consumer experience and native
apps for **macOS, Windows, iOS, and Android**. Open the app, pick a country,
connect — a real WireGuard tunnel routes your traffic through an Aevora gateway.

> **Status.** The backend, gateway node agent, and shared Rust client core are
> **implemented and tested** (Go + Rust, verified against PostgreSQL). The native
> apps are **implemented in real code** (SwiftUI + NetworkExtension + WireGuardKit;
> Kotlin + VpnService + wireguard-android) but must be **built on their platform
> toolchains** (Xcode / Android Studio + signing) — they cannot be compiled in a
> headless CI-less environment. Nothing is mocked; see per-platform READMEs for
> exact local requirements.

## Architecture in one idea

A small central **control plane** decides *who connects to which gateway*; the
**data plane** (WireGuard) carries traffic **directly** from device → gateway →
internet, never through the control plane. That split is why it's fast, scalable,
fault-isolated, and privacy-preserving. Full dossier:
[architecture ADR](docs/decisions/0001-architecture-baseline.md).

```
device (aevora-core) ──HTTPS──▶ control plane (Go + Postgres)   selection, leases, peers
      │                                     │
      └──────────── WireGuard UDP ──────────▶ gateway (wg + node agent) ──▶ internet
```

## Repository layout

```
control-plane/   Go API + Postgres — identity, auth, gateways, selection, leases, peers, metrics
agent/           Go node agent for gateways — register, heartbeat, real load, reconcile wg peers
client/
  core/          aevora-core — shared Rust brain (API, keys, state, tunnel config, stats) + UniFFI
  apple/         macOS + iOS: SwiftUI + NEPacketTunnelProvider + WireGuardKit
  android/       Kotlin + VpnService + wireguard-android GoBackend
  windows/       WireGuardNT integration notes
infra/gateway/   Phase 0 WireGuard gateway kit (bootstrap, peers, validate)
deploy/          Dev docker-compose; production compose + Caddy (TLS); systemd unit
docs/            Architecture, design, deployment, operations, security, runbooks
```

## Quickstart (development)

```bash
# Backend + Postgres (dev seed)
cd deploy && docker compose up --build
curl localhost:8080/v1/locations

# Tests
cd control-plane && go test ./...
cd agent && go test ./...
cd client/core && cargo test
```

The Rust core has been verified end-to-end against a live control plane
(`enroll → locations → connect → stats → disconnect`) via
`client/core/examples/smoke.rs`.

## Documentation

- [Architecture (ADR 0001)](docs/decisions/0001-architecture-baseline.md) ·
  [Phase 1 control plane](docs/design/phase-1-control-plane.md) ·
  [Phase 2 client](docs/design/phase-2-client.md)
- [Deployment](docs/DEPLOYMENT.md) — dev vs prod, TLS, env vars, checklist
- [Operations](docs/OPERATIONS.md) — add a country/gateway, monitoring, troubleshooting
- [Security](docs/SECURITY.md) — trust boundaries, keys, checklist
- Platform build guides: [apple](client/apple/README.md) ·
  [android](client/android/README.md) · [windows](client/windows/README.md) ·
  [gateway kit](docs/runbooks/phase-0-gateway.md)

## What works today (verified) vs. needs local tooling

| Area | State |
|------|-------|
| Control plane (identity, auth+argon2, devices, gateways, selection, leases, peers, failover, metrics, rate limiting) | ✅ implemented + tested (Go, Postgres) |
| Node agent (register, heartbeat, real CPU/bandwidth load, reconcile peers) | ✅ implemented + tested (cross-builds linux) |
| Shared Rust core (API, keys, state machine, tunnel config, real stats, UniFFI) | ✅ implemented + tested; live smoke |
| macOS / iOS apps (SwiftUI + NEPacketTunnelProvider + WireGuardKit) | ⚙️ real code — build in Xcode (full Xcode + Apple Developer acct) |
| Android app (VpnService + wireguard-android) | ⚙️ real code — build in Android Studio (NDK) |
| Windows client | 📝 integration notes (WireGuardNT) |
| World-map UI | ⚙️ in progress |

See each platform README for the exact toolchain, entitlements, and signing
requirements. No component claims to establish a tunnel that doesn't.
