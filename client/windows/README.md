# Aevora — Windows (structure only)

> **Not implemented in this environment.** Windows requires the Windows SDK and
> Visual Studio (or the .NET SDK) to build, and a Windows machine to run. This
> file records the intended structure so the shared `aevora-core` is reused
> rather than reimplemented. No placeholder code that pretends to establish a
> tunnel is included.

## Approach

Same split as the other platforms: `aevora-core` owns all API/auth/session/
selection/key/lease/tunnel-config logic; a native layer establishes the real
WireGuard tunnel.

- **Core binding:** build `aevora-core` as a Windows DLL (`cargo build --release
  --features ffi --target x86_64-pc-windows-msvc`). Bind it from the app either
  via a UniFFI C#/.NET binding generator, or a thin C ABI + `DllImport`
  (P/Invoke). The core's `prepare_connection` returns the `TunnelConfig`.
- **Tunnel:** use **WireGuardNT** via the official embeddable tunnel library
  (`wireguard.dll` / `tunnel.dll` from WireGuard for Windows). The app runs the
  tunnel as a Windows service, handing it the WireGuard config from the core.
  Do not reimplement the protocol.
- **UI:** WinUI 3 / .NET (or a cross-platform .NET UI). Minimal first; the full
  consumer UI later.
- **Secure storage:** store the device private key with DPAPI / Windows
  Credential Manager. Only the public key is sent to the control plane.

## Suggested layout (to be created on Windows)

```
windows/
  Aevora/                 .NET app (UI + NETunnelProviderManager-equivalent)
  AevoraTunnelService/    Windows service wrapping WireGuardNT (tunnel.dll)
  build-core.ps1          Build aevora-core.dll + generate bindings
```

## Requirements to build (mark clearly)

- Windows 10/11, Visual Studio 2022 + Windows SDK, .NET 8.
- Rust with the `x86_64-pc-windows-msvc` target.
- WireGuard for Windows (for `tunnel.dll` / WireGuardNT).
- A code-signing certificate for the service/driver (EV cert recommended).
