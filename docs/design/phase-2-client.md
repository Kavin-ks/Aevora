# Phase 2 — The client

Phase 2 builds the consumer app, on the [rev 0.2 client model](decisions/0001-architecture-baseline.md):
one **shared Rust core** plus, per platform, a **native tunnel provider** and a
**native UI**. All four platforms are first-class; the MVP brings up **Android +
Apple** first (macOS rides the shared Apple codebase), then Windows.

```
client/
  core/        aevora-core — shared Rust library (this increment, 2a)
  apple/       SwiftUI app + NetworkExtension provider (iOS + macOS)   [2b]
  android/     Jetpack Compose app + VpnService provider              [2b]
  windows/     WinUI app + WireGuard-NT service                        [3]
```

## 2a — shared core (`client/core`, crate `aevora-core`)

The platform-independent brain. It contains **all** the logic that does not
touch the OS, so it is written and tested once:

| Module | Responsibility |
|--------|----------------|
| `model` | Wire types matching the control-plane API |
| `transport` | `Transport` trait (HTTP) + real `ureq` impl (feature `net`) |
| `api` | Typed client: enroll, refresh, locations, connect, disconnect, stats |
| `keys` | WireGuard X25519 keypair generation (base64) |
| `state` | The connection state machine (disconnected→connecting→connected→…) |
| `tunnel` | `TunnelConfig` + `TunnelProvider` trait + config assembly |
| `client` | `VpnClient` façade: the one object the UI drives |

**Two seams the platform fills natively:**

- `Transport` — HTTP. The core ships a `ureq` implementation behind the `net`
  feature; a platform may instead supply its own (e.g. `URLSession`).
- `TunnelProvider` — `up(config)` / `down()` / `stats()` over the OS VPN
  framework. The core never touches the network stack itself.

The private key is generated in the core and returned inside `Session` for the
platform to store in its keystore; **only the public key is ever transmitted.**

### Testing

Everything is unit-tested against a fake `Transport` and a fake `TunnelProvider`
— no network, no OS — mirroring the Go control-plane's approach:

```bash
cd client/core && cargo test
```

Covers: key generation + public-from-private derivation, the API client
(request shape, bearer auth, error mapping), the state machine (valid/invalid
transitions), tunnel-config assembly, and the full enroll → connect → disconnect
flow including the failure path.

### Bindings (UniFFI) — feature `ffi`

The façade is exposed to Swift and Kotlin via UniFFI. Generating bindings is a
build step run by the platform projects:

```bash
cargo build --features ffi --release
# then uniffi-bindgen generates aevora_core.swift / aevora_core.kt
```

## 2b — native shims (handoff)

`client/apple` and `client/android` are scaffolded with the integration
contract and build instructions. They require Xcode / Android Studio, signing,
and a device or simulator to build and run, so they are developed on the
respective platforms rather than in this repo's CI-less core loop. Each wraps
`aevora-core`:

- **Apple:** a SwiftUI app + a NetworkExtension **Packet Tunnel Provider** that
  implements `TunnelProvider` using WireGuardKit; iOS and macOS share the code.
- **Android:** a Jetpack Compose app + a **VpnService** that implements
  `TunnelProvider` using `wireguard-android`.

Both call the same `VpnClient`: fetch locations → user picks a country →
`connect` → the native provider brings the tunnel up → show CONNECTED with
country, server, latency, up/down speed, and duration.
