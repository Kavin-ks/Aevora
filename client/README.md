# Aevora client

The consumer app: one **shared Rust core** with a **native tunnel + UI** per
platform (see [Phase 2 design](../docs/design/phase-2-client.md)).

```
client/
  core/      aevora-core — shared Rust library (built + tested)
  apple/     SwiftUI + NetworkExtension (iOS + macOS)   — scaffold, build on macOS
  android/   Jetpack Compose + VpnService               — scaffold, build in Android Studio
```

## Core

```bash
cd core
cargo test                 # logic tests (no network, no OS)
cargo build --features net # include the real HTTP transport
```

The platform apps link `aevora-core` and provide two native pieces: an HTTP
`Transport` (or use the built-in `ureq` one) and a `TunnelProvider` over the OS
VPN framework. Everything else — enroll, tokens, key generation, gateway
selection calls, the connection state machine, tunnel-config assembly — is in
the core. The device private key never leaves the device.
