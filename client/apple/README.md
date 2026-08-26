# Aevora — Apple (macOS + iOS)

Native app around `aevora-core`. The app runs the shared Rust core (API, auth,
session, device identity, gateway selection, key generation, lease management,
tunnel-config assembly); a **NEPacketTunnelProvider** extension establishes the
real WireGuard tunnel via **WireGuardKit**. macOS is implemented first; iOS
reuses the same code (see below).

> **This target cannot be built in the repo's CI-less core loop.** It requires a
> Mac with **full Xcode**, an **Apple Developer account** (for the Network
> Extension capability, App Groups, and signing), and a **Go toolchain** (to
> build WireGuardKit's `wireguard-go`). Steps below are what you run locally.

## What's here

```
apple/
  build-core.sh               Builds AevoraCore.xcframework + Swift bindings (run first)
  project.yml                 XcodeGen spec (generates Aevora.xcodeproj)
  Aevora/                     App target: SwiftUI UI (ContentView, WorldMapView),
                              AppModel, NETunnelProviderManager driver, Keychain
  PacketTunnel/               Packet-tunnel extension (WireGuardKit) — the real tunnel
  Shared/                     Keychain helper shared by app + extension
  build/                      Generated (git-ignored): xcframework + bindings
```

The tunnel is genuine: `PacketTunnelProvider` parses the WireGuard config with
`TunnelConfiguration(fromWgQuickConfig:)` and starts `WireGuardAdapter`. There is
no mock and no custom protocol.

## Build & run (macOS)

The UI (world map via MapKit, connect/disconnect, live stats) targets **macOS
14+**. Availability and the location list come from the control plane; only the
map marker positions use a static coordinate lookup (`WorldMapView.swift`).

Prerequisites: full Xcode selected, plus `brew install xcodegen go`.

```bash
sudo xcode-select -s /Applications/Xcode.app/Contents/Developer

cd client/apple
./build-core.sh          # 1. build the Rust core xcframework + Swift bindings
xcodegen generate        # 2. generate Aevora.xcodeproj from project.yml
open Aevora.xcodeproj     # 3. open in Xcode
```

In Xcode, before running:

1. **Signing** — select your Team for both the `Aevora` and `PacketTunnel`
   targets (or set `DEVELOPMENT_TEAM` in `project.yml` / an untracked xcconfig).
2. **Capabilities / App ID** — on the Apple Developer portal (or via automatic
   signing) enable **Network Extensions**, **App Groups**
   (`group.com.aevora.Aevora`), and **Keychain Sharing**
   (`com.aevora.Aevora`) for both App IDs. Adjust the bundle IDs / group / group
   prefix to your own reverse-domain if you change them (they appear in the
   entitlements and `Support.swift`).
3. **Control-plane URL** — set `AEVORA_CONTROL_URL` (build setting) to your
   control plane, e.g. a staging URL or, for local testing, a reachable address
   of your dev `controld`. It is **not** hardcoded in source.
4. Run the `Aevora` scheme. macOS will prompt to **allow the VPN
   configuration** the first time — approve it.

Then in the app: enter an invite code + email → **Enroll** → pick a country →
**Connect**. macOS shows the VPN as active; verify egress with
`curl https://api.ipify.org` (should be the gateway's IP).

### You need a running backend

The app talks to the Phase 1 control plane. Bring it up (see `deploy/`) with a
reachable gateway, and mint an invite (`POST /v1/invites`, admin token). For a
real tunnel the gateway must be a genuine WireGuard endpoint (the Phase 0 kit)
running the node agent, not the dev-seed placeholder.

## iOS

iOS uses the **same** Swift sources and the same `PacketTunnelProvider`
(NEPacketTunnelProvider is identical on iOS). To add it, duplicate the two
targets in `project.yml` with `platform: iOS` (the xcframework already includes
iOS device + simulator slices from `build-core.sh`). The only differences are
the deployment target and that iOS always requires a provisioning profile with
the Network Extensions entitlement. No app logic changes — the core is shared.

## Connection statistics

Duration is live. Latency / download / upload are shown as `—` until wired to
the extension's real counters: the app sends a `"stats"` provider message
(`handleAppMessage` returns WireGuard's runtime config with `rx`/`tx` bytes),
which the app parses into rates. This is intentionally not faked.

## Security notes

- The device **private key never leaves the device** and is stored in the
  keychain (see `Shared/Keychain.swift`); only the public key is sent to the
  control plane.
- The WireGuard config is passed to the extension by a **keychain persistent
  reference**, not in the NE preferences plist.
- No signing identity, provisioning profile, Team ID, or endpoint is committed.
- Hardened runtime + app sandbox are enabled; the VPN entitlements are the
  minimum required. Do not relax them.
