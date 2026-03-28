# PKV Installer for Windows
# Usage: irm https://raw.githubusercontent.com/shichao402/pkv/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "shichao402/pkv"
$InstallDir = "$env:LOCALAPPDATA\pkv"

function Write-Info  { param($Msg) Write-Host "[INFO] $Msg" -ForegroundColor Green }
function Write-Warn  { param($Msg) Write-Host "[WARN] $Msg" -ForegroundColor Yellow }
function Write-Err   { param($Msg) Write-Host "[ERROR] $Msg" -ForegroundColor Red; exit 1 }

function Get-Arch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64"   { return "amd64" }
        "Arm64" { return "arm64" }
        default { Write-Err "Unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    Write-Info "Fetching latest release..."
    $url = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $release = Invoke-RestMethod -Uri $url -UseBasicParsing
        return $release.tag_name
    } catch {
        Write-Err "Failed to fetch latest version: $_"
    }
}

function Download-Binary {
    param($Version, $Arch)

    $assetName = "pkv_windows_${Arch}.exe"
    $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$assetName"

    Write-Info "Downloading $assetName..."

    $tmpFile = [System.IO.Path]::GetTempFileName() + ".exe"
    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tmpFile -UseBasicParsing
    } catch {
        Write-Err "Download failed: $_"
    }

    return $tmpFile
}

function Install-Binary {
    param($TmpFile)

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $dest = Join-Path $InstallDir "pkv.exe"
    Move-Item -Path $TmpFile -Destination $dest -Force
    Write-Info "Installed pkv to $dest"
}

function Add-ToPath {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -notlike "*$InstallDir*") {
        Write-Warn "$InstallDir is not in your PATH."
        $reply = Read-Host "Add it to your user PATH? (Y/n)"
        if ($reply -eq "" -or $reply -eq "Y" -or $reply -eq "y") {
            [Environment]::SetEnvironmentVariable("Path", "$InstallDir;$currentPath", "User")
            $env:Path = "$InstallDir;$env:Path"
            Write-Info "Added $InstallDir to user PATH. Restart your terminal to take effect."
        } else {
            Write-Warn "Skipped. Add manually:"
            Write-Host ""
            Write-Host "  [Environment]::SetEnvironmentVariable('Path', '$InstallDir;' + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
            Write-Host ""
        }
    }
}

Write-Host "=== PKV Installer ===" -ForegroundColor Cyan
Write-Host ""

$arch = Get-Arch
Write-Info "Platform: windows/$arch"

$version = Get-LatestVersion
Write-Info "Latest version: $version"

$tmpFile = Download-Binary -Version $version -Arch $arch
Install-Binary -TmpFile $tmpFile
Add-ToPath

Write-Host ""
Write-Info "Done! Run 'pkv --version' to verify."
