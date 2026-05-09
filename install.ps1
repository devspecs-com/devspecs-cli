# DevSpecs CLI Installer for Windows
# Usage: irm https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "devspecs-com/devspecs-cli"
$BinaryName = "ds"
$InstallDir = if ($env:DEVSPECS_INSTALL_DIR) { $env:DEVSPECS_INSTALL_DIR } else { "$env:LOCALAPPDATA\DevSpecs\bin" }

function Get-LatestVersion {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    return $release.tag_name
}

function Install-DevSpecs {
    Write-Host "[INFO] Detecting system..." -ForegroundColor Green

    $Arch = if ([System.Environment]::Is64BitOperatingSystem) { "x86_64" } else { "i386" }
    Write-Host "[INFO] Architecture: $Arch" -ForegroundColor Green

    Write-Host "[INFO] Fetching latest version..." -ForegroundColor Green
    $Version = Get-LatestVersion
    if (-not $Version) {
        Write-Host "[ERROR] Could not determine latest version" -ForegroundColor Red
        exit 1
    }
    Write-Host "[INFO] Latest version: $Version" -ForegroundColor Green

    $VersionNum = $Version.TrimStart("v")
    $Filename = "devspecs_${VersionNum}_windows_${Arch}.zip"
    $DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$Filename"

    $TmpDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }

    Write-Host "[INFO] Downloading $Filename..." -ForegroundColor Green
    Invoke-WebRequest -Uri $DownloadUrl -OutFile "$TmpDir\$Filename"

    Write-Host "[INFO] Extracting..." -ForegroundColor Green
    Expand-Archive -Path "$TmpDir\$Filename" -DestinationPath $TmpDir -Force

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    Write-Host "[INFO] Installing to $InstallDir..." -ForegroundColor Green
    Copy-Item "$TmpDir\$BinaryName.exe" "$InstallDir\$BinaryName.exe" -Force

    Remove-Item $TmpDir -Recurse -Force

    # Add to PATH if not already there
    $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
        Write-Host "[INFO] Added $InstallDir to PATH. Restart your shell to use 'ds'." -ForegroundColor Yellow
    }

    Write-Host "[INFO] DevSpecs CLI installed successfully!" -ForegroundColor Green
    Write-Host "[INFO] Run 'ds --help' to get started" -ForegroundColor Green
}

Install-DevSpecs
