# Aevora VPN - Administrator Setup Script
# Run this script as ADMINISTRATOR to complete the remaining installations.
# This script handles everything that requires elevated privileges.
#
# Usage (as Administrator):
#   powershell -ExecutionPolicy Bypass -File setup-admin.ps1
#
# What this installs:
#   1. Docker Desktop (for the control plane backend)
#   2. WireGuard for Windows (for tunnel.dll + wireguard.dll)
#   3. VS Build Tools / MSVC (for building the Rust core DLL)
#
# After running this script, run: setup-dev.ps1 (as normal user)

$ErrorActionPreference = "Stop"

# Verify elevated
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "This script MUST be run as Administrator. Right-click PowerShell > Run as Administrator."
    exit 1
}

Write-Host "=== Aevora Admin Setup ===" -ForegroundColor Cyan
Write-Host "Running as: $([Security.Principal.WindowsIdentity]::GetCurrent().Name)"
Write-Host ""

# 1. Docker Desktop
Write-Host "--- Installing Docker Desktop ---" -ForegroundColor Yellow
$dockerInstaller = "$env:TEMP\DockerDesktopInstaller.exe"
if (-not (Test-Path $dockerInstaller)) {
    Write-Host "Downloading Docker Desktop..."
    Invoke-WebRequest -Uri "https://desktop.docker.com/win/main/amd64/Docker%20Desktop%20Installer.exe" `
        -OutFile $dockerInstaller -UseBasicParsing
}
Write-Host "Installing Docker Desktop..."
$proc = Start-Process $dockerInstaller -ArgumentList "install", "--quiet", "--accept-license", "--backend=wsl-2" -Wait -PassThru
if ($proc.ExitCode -ne 0) {
    Write-Warning "Docker Desktop installer returned exit code $($proc.ExitCode). May need reboot."
} else {
    Write-Host "Docker Desktop installed!" -ForegroundColor Green
}

# 2. WireGuard for Windows
Write-Host "`n--- Installing WireGuard for Windows ---" -ForegroundColor Yellow
$curl = "C:\Program Files\Git\mingw64\bin\curl.exe"
$wgMsi = "$env:TEMP\wireguard.msi"
if (-not (Test-Path $wgMsi)) {
    Write-Host "Downloading WireGuard..."
    if (Test-Path $curl) {
        & $curl -L "https://download.wireguard.com/windows-client/wireguard-amd64-0.5.3.msi" -o $wgMsi --ssl-revoke-best-effort -L
    } else {
        Invoke-WebRequest -Uri "https://download.wireguard.com/windows-client/wireguard-amd64-0.5.3.msi" -OutFile $wgMsi
    }
}
if (Test-Path $wgMsi) {
    Write-Host "Installing WireGuard..."
    $proc = Start-Process "msiexec.exe" -ArgumentList "/i", "`"$wgMsi`"", "/quiet", "/qn" -Wait -PassThru
    if ($proc.ExitCode -ne 0) {
        Write-Warning "WireGuard install returned: $($proc.ExitCode)"
    } else {
        Write-Host "WireGuard installed!" -ForegroundColor Green
    }
}

# 3. VS Build Tools (MSVC linker for Rust)
Write-Host "`n--- Installing VS Build Tools (MSVC linker for Rust) ---" -ForegroundColor Yellow
$vsInstaller = "$env:TEMP\vs_buildtools.exe"
if (-not (Test-Path $vsInstaller)) {
    Write-Host "Downloading VS Build Tools..."
    Invoke-WebRequest -Uri "https://aka.ms/vs/17/release/vs_buildtools.exe" -OutFile $vsInstaller -UseBasicParsing
}
Write-Host "Installing MSVC C++ compiler (this takes several minutes)..."
$proc = Start-Process $vsInstaller -ArgumentList `
    "--quiet", "--wait", "--norestart", "--nocache",
    "--add", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
    "--add", "Microsoft.VisualStudio.Component.VC.Redist.14.Latest",
    "--add", "Microsoft.VisualStudio.Component.Windows11SDK.26100" `
    -Wait -PassThru
if ($proc.ExitCode -eq 0 -or $proc.ExitCode -eq 3010) {
    Write-Host "VS Build Tools installed!" -ForegroundColor Green
} else {
    Write-Warning "VS Build Tools install returned: $($proc.ExitCode)"
}

# 4. Enable Hyper-V and WSL2 (needed for Docker Desktop)
Write-Host "`n--- Enabling Hyper-V and WSL2 ---" -ForegroundColor Yellow
Enable-WindowsOptionalFeature -Online -FeatureName "VirtualMachinePlatform" -All -NoRestart
Enable-WindowsOptionalFeature -Online -FeatureName "Microsoft-Hyper-V" -All -NoRestart
Write-Host "Note: A reboot may be required after enabling Hyper-V/WSL2" -ForegroundColor Yellow

# 5. Copy WireGuard DLLs to Aevora native directory
Write-Host "`n--- Copying WireGuard DLLs ---" -ForegroundColor Yellow
$nativeDir = "c:\Users\tashw\Aevora\client\windows\Aevora\native"
New-Item -ItemType Directory -Force -Path $nativeDir | Out-Null
$wgInstallDir = "C:\Program Files\WireGuard"
if (Test-Path $wgInstallDir) {
    $dlls = @("tunnel.dll", "wireguard.dll")
    foreach ($dll in $dlls) {
        $src = Join-Path $wgInstallDir $dll
        if (Test-Path $src) {
            Copy-Item $src $nativeDir -Force
            Write-Host "  Copied: $dll" -ForegroundColor Green
        } else {
            Write-Warning "  DLL not found: $src"
        }
    }
} else {
    Write-Warning "WireGuard not found at $wgInstallDir"
}

Write-Host "`n=== Admin Setup Complete ===" -ForegroundColor Cyan
Write-Host "Next steps:"
Write-Host "  1. Reboot if prompted (for Hyper-V/WSL2)"
Write-Host "  2. Start Docker Desktop and wait for it to initialize"
Write-Host "  3. Run: setup-dev.ps1 (as normal user) to build the Rust DLL and start the backend"
