# ADR 0001 — Architecture baseline

- **Status:** Accepted (2026-08-24); revised 2026-08-25 (rev 0.2 — mobile first-class)
- **Deciders:** Project lead + engineering
- **Supersedes:** —

> **Rev 0.2 (2026-08-25):** All four client platforms — macOS, Windows, Android,
> iOS — are now first-class from the start, not a later add-on. The desktop-only
> "Tauri app" pick is replaced by a **shared Rust core + native tunnel providers
> + native UI** model (see the Client row and Cross-platform notes below).

## Context

We are building Aevora, a subscription-free, Proton-style desktop VPN for a
small group, designed to scale to many countries, servers, and users. The full
reasoning, comparisons, diagrams, cost model, and risk register live in the
architecture dossier (published artifact). This ADR records the *decisions* so
the repository is the durable source of truth.

## Decision

**Core principle — separate the planes.** A small central **control plane**
decides who may connect and to which gateway; the **data plane** carries traffic
directly from client → gateway → internet and never traverses the control plane.

### Technology baseline

| Area | Decision |
|------|----------|
| VPN protocol | **WireGuard** (in-kernel on gateways; official per-OS libs on clients) |
| Client platforms | **macOS · Windows · Android · iOS**, all first-class |
| Client model | **Shared Rust core** (via UniFFI) + **native tunnel provider** + **native UI** per OS |
| Tunnel per OS | iOS/macOS NetworkExtension (WireGuardKit) · Android VpnService (wireguard-android) · Windows WireGuard-NT service |
| World map | **MapLibre Native** (iOS/Android SDKs) / **MapLibre GL** (desktop), self-hosted tiles |
| Backend / control plane | **Go** API service + **PostgreSQL** source of truth |
| Gateway | Plain Linux VPS: kernel WireGuard + `nftables` NAT + a Go **node agent** |
| Server discovery | Agents self-register; clients fetch the live location list from the API |
| Health / load | Agent heartbeat + TTL, Prometheus scrape; unhealthy nodes leave the pool |
| Server selection | Load + health score (server-side), latency probe tie-break (client-side) |
| Auth | Invite-key + email → short-lived JWT; per-device WireGuard keys; per-node creds |
| Key management | Client private key stays in the OS keystore; server stores only public keys |
| Egress / routing | `nftables` masquerade, IP forwarding, per-client `/32`, MTU 1420, MSS clamp |
| DNS | Tunnel-pushed resolver, forced through the tunnel (leak-free) |
| Observability | Prometheus + Grafana + alerting; privacy-preserving logs, no traffic logs |
| Scaling | Add gateways (linear); replicate the stateless API; Postgres is metadata-only |

### Rejected alternatives (summary)

- **OpenVPN / IPsec** over WireGuard — heavier, slower, harder to automate.
  OpenVPN retained as a *possible future* obfuscation/fallback transport.
- **Tauri / Electron desktop app** — cannot host an iOS Network Extension or
  Android VpnService, so it fails the mobile-first requirement. Replaced by the
  shared-Rust-core model (rev 0.2).
- **Flutter / React Native for everything** — one UI codebase, but still needs a
  native VPN-extension plugin per OS and fights the framework at the privileged
  boundary. Kept as a live alternative for the UI layer (open fork, see below).
- **Fully native per OS** — maximal nativeness but 3–4× duplicated client logic;
  the shared Rust core exists to avoid this.
- **Node/Rust** for the backend over Go — Go has the native WireGuard tooling
  (`wgctrl-go`), simplest single-binary ops, and the strongest health/metrics
  ecosystem for this workload.
- **Custom VPN/crypto** — explicitly forbidden; never roll our own.

## Consequences

- **Positive:** fault isolation (one bad gateway can't slow the fleet),
  horizontal scale by adding VMs, adding a country is a DB row not a release,
  strong privacy posture, and a client that stays a thin unprivileged UI over a
  small signed helper.
- **Costs / obligations:** we must ship code-signed/notarised installers (Apple
  + Windows cert), operate an exit node responsibly (provider ToS, abuse), and
  eventually add control-plane HA. These are tracked as roadmap/risk items.

## Rollout

Phased and incrementally demoable:

0. **Manual gateway spike** (kit built) — prove the pipe.
1. Control plane + node agent — automate one gateway (registry, peers, leases).
2. Shared Rust core + tunnel-provider interface + **first two platforms**
   (recommended Android + Apple; iOS and macOS share the SwiftUI/NetworkExtension
   code) — the one-button experience, on a phone and a desktop.
3. Remaining platforms (**Windows + second Apple target**, thin shims on the core)
   + multi-gateway selection + failover + dashboards.
4. Harden & scale — control-plane HA, App Store / Play / signed-installer
   submission, CI/CD across targets, alerting, obfuscation fallback.

## Resolved forks (2026-08-25)

- **UI strategy:** **native UI per platform** — SwiftUI (iOS + macOS shared),
  Jetpack Compose (Android), WinUI/.NET (Windows). ~3 UI codebases over the one
  shared Rust core. Chosen for nativeness and clean VPN-extension integration.
- **First MVP platform pair:** **Android + Apple** — covers the two hardest
  tunnel integrations (VpnService + NetworkExtension); macOS rides the shared
  Apple codebase. Windows + second Apple target follow in Phase 3.
