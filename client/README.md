# Aevora client

One **shared Rust core** with a **native tunnel + UI per platform**
(see [Phase 2 design](../docs/design/phase-2-client.md)). No VPN, auth, API, or
selection logic is duplicated per platform — it all lives in `core/`.

```
client/
  core/      aevora-core — shared Rust library (built + tested here)
  apple/     macOS + iOS: SwiftUI app + NEPacketTunnelProvider (WireGuardKit)
  android/   Jetpack/VpnService app + wireguard-android GoBackend
  windows/   structure + integration notes (WireGuardNT)
```

## Core (`core/`)

```bash
cd core
cargo test                       # 16 logic tests (no network, no OS)
cargo build --features ffi       # UniFFI bindings + built-in HTTP transport
```

The core exposes an `AevoraClient` (via UniFFI) to Swift and Kotlin:
`enroll → locations → prepareConnection → (native tunnel up) → markConnected →
keepAlive → disconnect`. `prepareConnection` returns the WireGuard
`TunnelConfig`; the platform establishes the real OS tunnel from it. The device
private key is generated in the core and never transmitted.

## Platforms

| Platform | Tunnel | Buildable here? |
|----------|--------|-----------------|
| macOS | NEPacketTunnelProvider + WireGuardKit | No — needs full Xcode + Apple Developer acct. See [apple/README](apple/README.md) |
| iOS | Same as macOS (shared code) | No — needs Xcode + provisioning |
| Android | VpnService + wireguard-android GoBackend | No — needs Android Studio + NDK. See [android/README](android/README.md) |
| Windows | WireGuardNT | No — needs Windows SDK. See [windows/README](windows/README.md) |

The **Rust core and its Swift/Kotlin bindings are built and verified in this
repo**; the native apps require their platform toolchains, code signing, and
entitlements, and are built on the respective OSes (each README lists the exact
requirements). Nothing here fakes a tunnel.
