# Aevora Windows Build & Run — Agent Prompt

> Copy everything below this line and paste it to Antigravity AI (or any other AI agent) running on Windows.

---

## Task: Build and run the Aevora Windows VPN client

You are on a **Windows machine**. Your job is to build and run the Aevora Windows VPN client from source, enroll it against a live control plane, and verify the UI works end-to-end.

### What Aevora Windows is

- `.NET 8 / WPF` desktop app (`client/windows/Aevora/`)
- Shared **Rust core** via C ABI (`aevora_core.dll`) — handles all API, auth, key generation, session management, server selection
- Real VPN via **WireGuardNT** (`tunnel.dll` + `wireguard.dll`) — the official embeddable WireGuard engine
- No mock tunnel. No fake speeds. No simulated data.

### Repo

```
https://github.com/Kavin-ks/Aevora
```

Clone to `C:\Aevora` or any path without spaces.

```powershell
git clone https://github.com/Kavin-ks/Aevora.git C:\Aevora
cd C:\Aevora
```

---

## Prerequisites — install these first

| Tool | Where to get | Required for |
|------|-------------|-------------|
| **Rust stable (MSVC toolchain)** | https://rustup.rs → choose `x86_64-pc-windows-msvc` | Building aevora_core.dll |
| **.NET 8 SDK** | https://dotnet.microsoft.com/download/dotnet/8.0 | Building WPF app |
| **Visual Studio Build Tools** (or full VS 2022) | https://aka.ms/vs/17/release/vs_BuildTools.exe — select "Desktop development with C++" | Linker for Rust MSVC target |
| **WireGuard for Windows** | https://download.wireguard.com/windows-client/wireguard-installer.exe | Provides tunnel.dll + wireguard.dll |

After installing WireGuard for Windows, add the Rust target:
```powershell
rustup target add x86_64-pc-windows-msvc
```

---

## Step 1 — Build the Rust core DLL

Run **PowerShell as Administrator**:

```powershell
cd C:\Aevora\client\windows
.\build-core.ps1
```

This runs `cargo build --release --features capi --target x86_64-pc-windows-msvc` and copies `aevora_core.dll` into `Aevora\native\`.

If `build-core.ps1` is not present, run manually:
```powershell
cd C:\Aevora\client\core
$env:GOTOOLCHAIN = "local"
cargo build --release --features capi --target x86_64-pc-windows-msvc
# then copy the output:
New-Item -ItemType Directory -Force "C:\Aevora\client\windows\Aevora\native"
Copy-Item "target\x86_64-pc-windows-msvc\release\aevora_core.dll" `
          "C:\Aevora\client\windows\Aevora\native\"
```

---

## Step 2 — Copy WireGuardNT DLLs

WireGuard for Windows installs `tunnel.dll` and `wireguard.dll`. Copy them:

```powershell
# Default install path — adjust if WireGuard is elsewhere
$wgDir = "C:\Program Files\WireGuard"
$dest  = "C:\Aevora\client\windows\Aevora\native"

Copy-Item "$wgDir\tunnel.dll"    $dest
Copy-Item "$wgDir\wireguard.dll" $dest
```

Verify:
```powershell
dir C:\Aevora\client\windows\Aevora\native
# Should show: aevora_core.dll  tunnel.dll  wireguard.dll
```

---

## Step 3 — Set the control-plane URL

The control plane is running at:

```
http://YOUR_CONTROL_PLANE_IP:8099
```

> **For local testing**: if the Aevora control plane is running on another machine on the same network, use its LAN IP. If it's on the same Windows machine, use `http://127.0.0.1:8099`.

Set the environment variable **before** building or running:

```powershell
$env:AEVORA_CONTROL_URL = "http://YOUR_CONTROL_PLANE_IP:8099"
```

---

## Step 4 — Build the WPF app

```powershell
cd C:\Aevora\client\windows\Aevora
dotnet build Aevora.csproj -c Release -r win-x64 --self-contained false
```

The output is at:
```
C:\Aevora\client\windows\Aevora\bin\Release\net8.0-windows\win-x64\Aevora.exe
```

---

## Step 5 — Run as Administrator

The app requires Administrator rights (it installs a Windows service for the WireGuard tunnel). **Right-click → Run as administrator**, or:

```powershell
Start-Process powershell -Verb RunAs -ArgumentList `
  "-NoExit", "-Command", `
  "cd 'C:\Aevora\client\windows\Aevora'; `
   `$env:AEVORA_CONTROL_URL='http://YOUR_CONTROL_PLANE_IP:8099'; `
   .\bin\Release\net8.0-windows\win-x64\Aevora.exe"
```

---

## Step 6 — Enroll the device

When the app opens you will see **"Enroll this device"**.

- **Invite code**: `FhktcR5D6_iuPMTVrIWhfqI3V13Hb1WbmyxEDUBQjQQ`
- **Email**: any email address (e.g. `test@aevora.local`)

Click **Enroll**. The app will:
1. Generate a WireGuard keypair on-device (private key never leaves the machine)
2. Register the device with the control plane
3. Show the main UI with a world map and country list

> ⚠️ **Invite codes are single-use.** If enrollment fails, ask for a new one.

---

## Step 7 — Connect to a server

After enrollment the app shows a country list. Select any **available** country (green dot) and click **Connect**.

> **Note:** Connecting requires a real WireGuard gateway running the Aevora node agent on a Linux VPS. Without a gateway, enrollment and the UI work fine, but the tunnel will fail to bring up. The UI itself, stats display, and all API flows work independently of a live gateway.

---

## What to verify / report back

1. ✅ `aevora_core.dll` builds without errors
2. ✅ `dotnet build` succeeds
3. ✅ App launches (no crash at startup)
4. ✅ Enroll screen appears; invite code accepted
5. ✅ Country list loads (14 locations: US, UK, DE, FR, NL, SE, JP, SG, AU, CA, BR, IN, AE, ZA)
6. ⚠️  Connect attempt — report the exact error if it fails (expected without a live gateway)
7. Report any `System.DllNotFoundException` or P/Invoke errors — those need DLL path fixes

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `System.DllNotFoundException: aevora_core` | `aevora_core.dll` not in `native\` — redo Step 1 |
| `System.DllNotFoundException: tunnel` | `tunnel.dll` not in `native\` — redo Step 2 |
| `Access is denied` when connecting | App must run as Administrator (Step 5) |
| Enrollment fails with HTTP error | Check `AEVORA_CONTROL_URL` is correct and the control plane is reachable (`curl http://YOUR_IP:8099/healthz`) |
| Blank country list | Control plane reachable? Enrolled successfully? Try clicking Refresh |
| Rust build error `linker 'link.exe' not found` | Install Visual Studio Build Tools with "C++ build tools" |

---

## Files you will work with

```
C:\Aevora\client\windows\
  build-core.ps1                   ← Build Rust DLL
  Aevora\
    Aevora.csproj                  ← .NET 8 WPF project
    AevoraCore.cs                  ← P/Invoke bindings to Rust
    TunnelService.cs               ← WireGuardNT service wrapper
    MainViewModel.cs               ← All UI state + business logic
    MainWindow.xaml                ← UI layout
    AppConfig.cs                   ← Reads AEVORA_CONTROL_URL env var
    native\                        ← Place DLLs here (gitignored)
```

---

## Architecture summary (for context)

```
┌─────────────────────────────────┐
│  WPF UI (MainWindow.xaml)       │  ← You see this
│  MainViewModel.cs (state)       │
└────────────┬────────────────────┘
             │ P/Invoke
┌────────────▼────────────────────┐
│  aevora_core.dll (Rust)         │  ← Shared logic: enroll, connect,
│  C ABI: enroll, locations,      │    select gateway, WireGuard config,
│  prepare_connection, stats…     │    key generation, token refresh
└────────────┬────────────────────┘
             │ WireGuard config string
┌────────────▼────────────────────┐
│  WireGuardNT (tunnel.dll)       │  ← Real kernel-level VPN tunnel
│  as a Windows service           │    (same engine as WireGuard for Windows)
└─────────────────────────────────┘
```

The Rust core communicates with:
```
http://YOUR_CONTROL_PLANE_IP:8099   ← Aevora control plane (Go + Postgres)
```

---

*Generated by Claude Sonnet 4.6 for the Aevora project — https://github.com/Kavin-ks/Aevora*
