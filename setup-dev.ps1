# Aevora VPN - Developer Setup Script (runs without admin)
# Run this AFTER setup-admin.ps1 has been executed as Administrator.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File setup-dev.ps1
#
# What this does:
#   1. Verifies all dependencies are available
#   2. Builds the Rust core DLL (aevora_core.dll)
#   3. Copies WireGuard DLLs to the native directory
#   4. Starts the Docker backend (PostgreSQL + control plane)
#   5. Generates an invite code for enrollment
#   6. Launches the Aevora Windows app

$ErrorActionPreference = "Continue"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path

# Add dotnet to PATH if needed
$dotnetPaths = @(
    "$env:LOCALAPPDATA\Microsoft\dotnet",
    "C:\Program Files\dotnet"
)
foreach ($p in $dotnetPaths) {
    if (Test-Path "$p\dotnet.exe") {
        $env:PATH = "$p;$env:PATH"
        Write-Host "dotnet found at: $p"
        break
    }
}

# Add Rust/cargo to PATH if needed
if (Test-Path "$env:USERPROFILE\.cargo\bin") {
    $env:PATH = "$env:USERPROFILE\.cargo\bin;$env:PATH"
}

Write-Host "=== Aevora Developer Setup ===" -ForegroundColor Cyan
Write-Host ""

# 1. Check dependencies
Write-Host "--- Checking Dependencies ---" -ForegroundColor Yellow
$deps = @(
    @{Name="git"; Cmd="git --version"},
    @{Name="dotnet"; Cmd="dotnet --version"},
    @{Name="cargo"; Cmd="cargo --version"},
    @{Name="docker"; Cmd="docker --version"}
)
$allOk = $true
foreach ($d in $deps) {
    try {
        $ver = Invoke-Expression $d.Cmd 2>&1 | Select-Object -First 1
        Write-Host "  [OK] $($d.Name): $ver" -ForegroundColor Green
    } catch {
        Write-Host "  [MISSING] $($d.Name)" -ForegroundColor Red
        $allOk = $false
    }
}

if (-not $allOk) {
    Write-Warning "Some dependencies are missing. Run setup-admin.ps1 as Administrator first."
}

# 2. Build Rust core DLL
Write-Host "`n--- Building Rust Core DLL ---" -ForegroundColor Yellow
$buildScript = "$here\client\windows\build-core.ps1"
if (Test-Path $buildScript) {
    Write-Host "Running build-core.ps1..."
    & $buildScript
    $nativeDll = "$here\client\windows\Aevora\native\aevora_core.dll"
    if (Test-Path $nativeDll) {
        Write-Host "  aevora_core.dll built!" -ForegroundColor Green
    } else {
        Write-Warning "  aevora_core.dll NOT found. Check Rust/MSVC setup."
    }
} else {
    Write-Warning "build-core.ps1 not found at: $buildScript"
}

# 3. Copy WireGuard DLLs
Write-Host "`n--- Copying WireGuard DLLs ---" -ForegroundColor Yellow
$nativeDir = "$here\client\windows\Aevora\native"
$wgDir = "C:\Program Files\WireGuard"
$dlls = @("tunnel.dll", "wireguard.dll")
foreach ($dll in $dlls) {
    $src = Join-Path $wgDir $dll
    $dst = Join-Path $nativeDir $dll
    if (Test-Path $src) {
        Copy-Item $src $dst -Force
        Write-Host "  Copied: $dll" -ForegroundColor Green
    } elseif (Test-Path $dst) {
        Write-Host "  Already present: $dll" -ForegroundColor Green
    } else {
        Write-Warning "  Missing: $dll (WireGuard for Windows must be installed)"
    }
}

# 4. Rebuild WPF app with native DLLs
Write-Host "`n--- Building WPF Application ---" -ForegroundColor Yellow
$proj = "$here\client\windows\Aevora\Aevora.csproj"
& dotnet publish $proj -c Release -r win-x64 --self-contained false -o "$here\client\windows\publish" 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "  WPF app published!" -ForegroundColor Green
    Write-Host "  Executable: $here\client\windows\publish\Aevora.exe"
} else {
    Write-Warning "  WPF build failed"
}

# 5. Start Docker backend
Write-Host "`n--- Starting Backend (Docker Compose) ---" -ForegroundColor Yellow
$deployDir = "$here\deploy"
try {
    docker compose -f "$deployDir\docker-compose.yml" up -d --build 2>&1
    Write-Host "  Docker Compose started" -ForegroundColor Green
    Write-Host "  Waiting 15s for backend to initialize..."
    Start-Sleep 15
    # Test health
    $health = Invoke-WebRequest -Uri "http://localhost:8080/healthz" -UseBasicParsing -ErrorAction SilentlyContinue
    if ($health.StatusCode -eq 200) {
        Write-Host "  Control plane is healthy!" -ForegroundColor Green
    }
    # Get locations
    $locs = Invoke-RestMethod -Uri "http://localhost:8080/v1/locations" -ErrorAction SilentlyContinue
    if ($locs) {
        Write-Host "  Locations available: $($locs.locations.Count)" -ForegroundColor Green
    }
} catch {
    Write-Warning "  Docker Compose failed: $_"
    Write-Host "  Ensure Docker Desktop is running, then try: docker compose -f deploy\docker-compose.yml up -d"
}

# 6. Generate invite code
Write-Host "`n--- Generating Invite Code ---" -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Method POST `
        -Uri "http://localhost:8080/v1/invites" `
        -Headers @{ Authorization = "Bearer dev-admin-token-local-only" } `
        -ContentType "application/json" `
        -Body '{"note":"local-dev-invite","ttl":"720h"}' `
        -ErrorAction Stop
    Write-Host "  Invite code: $($response.code)" -ForegroundColor Green -BackgroundColor DarkGreen
    Write-Host "  Use this code when you first open Aevora!" -ForegroundColor Yellow
    $inviteCode = $response.code
} catch {
    Write-Warning "  Could not generate invite: $_ (is the backend running?)"
}

# 7. Launch the app
Write-Host "`n--- Summary ---" -ForegroundColor Cyan
Write-Host "Executable: $here\client\windows\publish\Aevora.exe"
Write-Host "Backend:    http://localhost:8080"
if ($inviteCode) {
    Write-Host "Invite:     $inviteCode" -ForegroundColor Yellow
}
Write-Host ""
Write-Host "To launch the app (as Administrator, for WireGuard tunnel):"
Write-Host "  Start-Process '$here\client\windows\publish\Aevora.exe' -Verb RunAs"
Write-Host ""
Write-Host "=== Setup Complete ===" -ForegroundColor Cyan
