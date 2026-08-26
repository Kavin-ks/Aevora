# Builds aevora-core as a Windows DLL (C ABI) for the .NET app to P/Invoke.
# Run on Windows with Rust + the MSVC toolchain:
#
#   rustup target add x86_64-pc-windows-msvc
#   ./build-core.ps1
#
# Output (git-ignored): Aevora/native/aevora_core.dll

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$core = Join-Path $here "..\core"
$outDir = Join-Path $here "Aevora\native"

Write-Host "==> Building aevora-core (release, feature capi) for windows-msvc"
Push-Location $core
cargo build --release --features capi --target x86_64-pc-windows-msvc
Pop-Location

New-Item -ItemType Directory -Force -Path $outDir | Out-Null
Copy-Item (Join-Path $core "target\x86_64-pc-windows-msvc\release\aevora_core.dll") $outDir -Force
Write-Host "  Done -> $outDir\aevora_core.dll"
Write-Host "  Also place WireGuardNT's tunnel.dll and wireguard.dll in $outDir (see README)."
