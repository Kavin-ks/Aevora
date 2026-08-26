# Aevora — Windows

Native Windows client (.NET 8 / WPF) around the shared Rust core. The core (via a
C ABI) owns all API/auth/session/selection/lease/key/stats logic; the real VPN is
carried by the **official WireGuardNT** embeddable tunnel (`tunnel.dll` +
`wireguard.dll`), the same engine WireGuard for Windows uses. No custom protocol,
no mock.

> **Cannot be built in this repo's core loop** (needs Windows + the .NET 8 SDK +
> Visual Studio/`dotnet`, plus WireGuard for Windows for the native DLLs). The
> **Rust C ABI is real and compiled/verified** here; the WPF app is complete
> source that you build on Windows.

## What's here

```
windows/
  build-core.ps1                 Build aevora_core.dll (Rust, feature capi)
  Aevora/
    Aevora.csproj                WPF app, net8.0-windows, x64, requires admin
    app.manifest                 requireAdministrator (service install + adapter)
    AppConfig.cs                 Control-plane URL from AEVORA_CONTROL_URL (not hardcoded)
    AevoraCore.cs                P/Invoke to the Rust core's C ABI (the shared brain)
    TunnelService.cs             Real WireGuardNT: tunnel.dll (up/down) + wireguard.dll (stats)
    ServiceManager.cs            Installs/starts/stops the tunnel Windows service
    SecureStore.cs               Session + private key encrypted with DPAPI
    MainViewModel.cs             State + orchestration + 3s stats loop
    App.xaml(.cs)                Entry point; "/service <conf>" runs the tunnel loop
    MainWindow.xaml(.cs)         Consumer UI: map, connect/disconnect, state, stats
    Converters.cs                Inverse bool→visibility
    native/                      Git-ignored: aevora_core.dll, tunnel.dll, wireguard.dll
```

## How the tunnel works (real)

1. The UI runs `AevoraCore.PrepareConnection(country)` → the core selects a
   gateway, leases an address, and returns the WireGuard config.
2. `TunnelService.Connect(config)` writes a wg-quick `.conf` and installs+starts a
   Windows service (`ServiceManager`) whose image is `Aevora.exe /service <conf>`.
3. That service process calls `TunnelService.RunServiceEntryPoint` →
   `WireGuardTunnelService(conf)` in **tunnel.dll**, which brings up the
   **WireGuardNT** adapter and routes traffic.
4. Stats: `wireguard.dll`'s `WireGuardGetConfiguration` is read every 3s for real
   rx/tx bytes, fed to the core (which computes rates and measures latency).
5. Disconnect stops+removes the service; the core releases the lease and the
   gateway peer is removed.

## Build & run (on Windows, as Administrator)

```powershell
# 1. Build the Rust core DLL
rustup target add x86_64-pc-windows-msvc
./build-core.ps1

# 2. Provide WireGuardNT DLLs: install "WireGuard for Windows", then copy
#    tunnel.dll and wireguard.dll into Aevora/native/ (see the WireGuard embeddable
#    DLL distribution / wireguard-windows). These are NOT committed.

# 3. Build & run the app (elevated)
$env:AEVORA_CONTROL_URL = "https://control.example.com"
dotnet build Aevora/Aevora.csproj -c Release
# run the produced Aevora.exe elevated
```

You also need a running control plane with a real WireGuard gateway + node agent,
and an invite code.

## Security

- Device private key generated in the core, stored via **DPAPI** (CurrentUser);
  only the public key is transmitted.
- The app requires administrator rights (service + adapter). Do not relax this.
- No signing certificate, credentials, endpoints, or native DLLs are committed.

## Remaining local verification

The C ABI compiles here; the WPF/native integration must be compiled and run on
Windows. The `wireguard.dll` stats struct offsets should be confirmed against the
installed WireGuardNT version. Code-sign the executable for distribution.
