# PKV Installer for Windows
# Usage: irm https://raw.githubusercontent.com/shichao402/pkv/ReleaseLatest/install.ps1 | iex

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = "shichao402/pkv"
$InstallDir = "$env:LOCALAPPDATA\pkv"

function Write-Info { param($Msg) Write-Host "[INFO] $Msg" -ForegroundColor Green }
function Write-Warn { param($Msg) Write-Host "[WARN] $Msg" -ForegroundColor Yellow }
function Write-Err { param($Msg) Write-Host "[ERROR] $Msg" -ForegroundColor Red; exit 1 }

function Get-Arch {
    try {
        $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
        if ($arch) {
            switch ($arch.ToString()) {
                "X64" { return "amd64" }
                "Arm64" { return "arm64" }
            }
        }
    } catch {}

    $procArch = $env:PROCESSOR_ARCHITECTURE
    switch ($procArch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        "x86" {
            $wow64Arch = $env:PROCESSOR_ARCHITEW6432
            if ($wow64Arch) {
                switch ($wow64Arch) {
                    "AMD64" { return "amd64" }
                    "ARM64" { return "arm64" }
                }
            }
        }
    }

    try {
        $cpu = Get-WmiObject Win32_Processor | Select-Object -First 1
        if ($cpu.AddressWidth -eq 64) {
            return "amd64"
        }
    } catch {}

    Write-Err "Unsupported architecture: PROCESSOR_ARCHITECTURE=$procArch"
}

function Compare-Versions {
    param(
        [string]$Version1,
        [string]$Version2
    )

    $v1 = $Version1 -replace '^v', ''
    $v2 = $Version2 -replace '^v', ''
    $v1Parts = $v1 -split '\.'
    $v2Parts = $v2 -split '\.'

    for ($i = 0; $i -lt 3; $i++) {
        $v1Part = if ($i -lt $v1Parts.Length -and $v1Parts[$i]) { [int]$v1Parts[$i] } else { 0 }
        $v2Part = if ($i -lt $v2Parts.Length -and $v2Parts[$i]) { [int]$v2Parts[$i] } else { 0 }

        if ($v1Part -gt $v2Part) { return 1 }
        if ($v1Part -lt $v2Part) { return -1 }
    }

    return 0
}

function Get-LatestVersion {
    Write-Info "Fetching latest release..."
    $url = "https://github.com/$Repo/releases/latest"
    try {
        $response = Invoke-WebRequest -Uri $url -MaximumRedirection 0 -UseBasicParsing -ErrorAction SilentlyContinue 2>$null
    } catch {
        $response = $_.Exception.Response
    }
    $location = $response.Headers.Location
    if (-not $location) {
        try {
            $response = Invoke-WebRequest -Uri $url -UseBasicParsing
            $location = $response.BaseResponse.ResponseUri.ToString()
            if (-not $location) {
                $location = $response.BaseResponse.RequestMessage.RequestUri.ToString()
            }
        } catch {
            Write-Err "Failed to fetch latest version: $_"
        }
    }
    $version = ($location -split '/')[-1]
    if (-not $version) {
        Write-Err "Failed to determine latest version from redirect"
    }
    return $version
}

function Confirm-Install {
    param($Version)

    $binaryPath = Join-Path $InstallDir "pkv.exe"
    if (-not (Test-Path $binaryPath)) {
        return
    }

    try {
        $currentVersion = & $binaryPath --version 2>&1 | Select-String -Pattern 'v(\d+\.\d+\.\d+)' | ForEach-Object { $_.Matches[0].Value } | Select-Object -First 1
    } catch {
        $currentVersion = $null
    }

    if ($currentVersion) {
        Write-Info "Current installed version: $currentVersion"
        if ((Compare-Versions -Version1 $currentVersion -Version2 $Version) -ge 0) {
            Write-Info "Already up to date."
            exit 0
        }
        $reply = Read-Host "Update $currentVersion to $Version? (Y/n)"
    } else {
        Write-Warn "Existing pkv found, but its version could not be detected."
        $reply = Read-Host "Overwrite existing install? (Y/n)"
    }

    if ($reply -eq "n" -or $reply -eq "N") {
        Write-Info "Skipped."
        exit 0
    }
}

function Download-Binary {
    param($Version, $Arch)

    $assetName = "pkv_windows_${Arch}.exe"
    $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$assetName"
    $checksumsUrl = "https://github.com/$Repo/releases/download/$Version/checksums.sha256"

    Write-Info "Downloading $assetName..."

    $tmpFile = [System.IO.Path]::GetTempFileName() + ".exe"
    $checksumsFile = [System.IO.Path]::GetTempFileName()
    try {
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsFile -UseBasicParsing
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tmpFile -UseBasicParsing
        Verify-Checksum -FilePath $tmpFile -ChecksumsFile $checksumsFile -AssetName $assetName
    } catch {
        Remove-Item -Path $tmpFile -Force -ErrorAction SilentlyContinue
        Remove-Item -Path $checksumsFile -Force -ErrorAction SilentlyContinue
        Write-Err "Download or checksum verification failed: $_"
    }
    Remove-Item -Path $checksumsFile -Force -ErrorAction SilentlyContinue

    return $tmpFile
}

function Verify-Checksum {
    param($FilePath, $ChecksumsFile, $AssetName)

    Write-Info "Verifying checksum..."
    $line = Get-Content $ChecksumsFile | Where-Object { $_ -match "\s$([regex]::Escape($AssetName))$" } | Select-Object -First 1
    if (-not $line) {
        Write-Err "No checksum found for $AssetName"
    }

    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $FilePath).Hash.ToLowerInvariant()
    if ($expected -ne $actual) {
        Write-Err "Checksum verification failed. Expected: $expected Actual: $actual"
    }
    Write-Info "Checksum verified."
}

function Install-Binary {
    param($TmpFile)

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $dest = Join-Path $InstallDir "pkv.exe"
    Move-Item -Path $TmpFile -Destination $dest -Force
    Write-Info "Installed pkv to $dest"

    try {
        $installedVersion = & $dest --version 2>&1
        if ($installedVersion -notmatch 'v\d+\.\d+\.\d+') {
            Write-Err "Install failed: could not verify installed binary"
        }
        Write-Info "Installed version: $installedVersion"
    } catch {
        Write-Err "Install failed: could not run installed binary: $_"
    }
}

function Add-ToPath {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -notlike "*$InstallDir*") {
        Write-Warn "$InstallDir is not in your PATH."
        $reply = Read-Host "Add it to your user PATH? (Y/n)"
        if ($reply -eq "" -or $reply -eq "Y" -or $reply -eq "y") {
            $newPath = if ([string]::IsNullOrWhiteSpace($currentPath)) { $InstallDir } else { "$InstallDir;$currentPath" }
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
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

Confirm-Install -Version $version
$tmpFile = Download-Binary -Version $version -Arch $arch
Install-Binary -TmpFile $tmpFile
Add-ToPath

Write-Host ""
Write-Info "Done! Run 'pkv --version' to verify."
