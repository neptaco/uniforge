[CmdletBinding()]
param(
    [string]$Version = $(if ($env:UNIFORGE_VERSION) { $env:UNIFORGE_VERSION } else { "latest" }),
    [string]$InstallDir = $(if ($env:UNIFORGE_INSTALL_DIR) { $env:UNIFORGE_INSTALL_DIR } else { Join-Path $HOME ".local\bin" })
)

$ErrorActionPreference = "Stop"
$Repository = "neptaco/uniforge"
if ($Version -ne "latest" -and $Version -notmatch '^v\d+\.\d+\.\d+$') {
    throw "Invalid version: $Version (expected latest or vX.Y.Z)"
}
if (-not [Environment]::Is64BitOperatingSystem) {
    throw "32-bit Windows is not supported"
}
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    throw "Windows ARM64 is not supported"
}

$BaseUrl = if ($Version -eq "latest") {
    "https://github.com/$Repository/releases/latest/download"
} else {
    "https://github.com/$Repository/releases/download/$Version"
}
$Archive = "uniforge_windows_amd64.tar.gz"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("uniforge-install-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $ArchivePath = Join-Path $TempDir $Archive
    $ChecksumPath = Join-Path $TempDir "checksums.txt"
    Write-Host "Downloading UniForge $Version for windows/amd64..."
    Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumPath

    $Line = Get-Content $ChecksumPath | Where-Object { $_ -match "\s\*?$([regex]::Escape($Archive))$" } | Select-Object -First 1
    if (-not $Line) { throw "Checksum not found for $Archive" }
    $Expected = ($Line -split '\s+')[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($Expected -ne $Actual) { throw "Checksum mismatch for $Archive" }

    tar -xzf $ArchivePath -C $TempDir uniforge.exe
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $Target = Join-Path $InstallDir "uniforge.exe"
    $Staged = Join-Path $InstallDir (".uniforge-install-" + [Guid]::NewGuid() + ".exe")
    Copy-Item (Join-Path $TempDir "uniforge.exe") $Staged
    Move-Item -Force $Staged $Target

    Write-Host "Installed UniForge to $Target"
    & $Target --version
    if (($env:PATH -split ';') -notcontains $InstallDir) {
        Write-Host "Add $InstallDir to PATH to run uniforge."
    }
} finally {
    Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
}
