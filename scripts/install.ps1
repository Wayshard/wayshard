#!/usr/bin/env pwsh
# Wayshard installer for Windows (x64).
#
#   powershell -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/Wayshard/wayshard/main/scripts/install.ps1 | iex"
#
# Resolves the latest stable GitHub release, verifies the downloads against the
# release SHA256SUMS.txt (and minisign when the tool is present), installs the
# CLI, TUI companion and server atomically into a user-owned bin directory, and
# adds that directory to the user PATH idempotently.
#
# Environment overrides (all optional):
#   WAYSHARD_VERSION, WAYSHARD_INSTALL_DIR, WAYSHARD_NO_MODIFY_PATH=1,
#   WAYSHARD_MINISIGN=auto|require|skip, WAYSHARD_MINISIGN_PUB,
#   WAYSHARD_REPO, WAYSHARD_API_BASE, WAYSHARD_DOWNLOAD_BASE, WAYSHARD_RAW_BASE
$ErrorActionPreference = "Stop"

$Repo = if ($env:WAYSHARD_REPO) { $env:WAYSHARD_REPO } else { "Wayshard/wayshard" }
$Version = $env:WAYSHARD_VERSION
$ApiBase = if ($env:WAYSHARD_API_BASE) { $env:WAYSHARD_API_BASE } else { "https://api.github.com" }
$DownloadBase = if ($env:WAYSHARD_DOWNLOAD_BASE) { $env:WAYSHARD_DOWNLOAD_BASE } else { "https://github.com/$Repo/releases/download" }
$RawBase = if ($env:WAYSHARD_RAW_BASE) { $env:WAYSHARD_RAW_BASE } else { "https://raw.githubusercontent.com/$Repo/main" }
$InstallDir = if ($env:WAYSHARD_INSTALL_DIR) { $env:WAYSHARD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Wayshard\bin" }
$ModifyPath = -not ($env:WAYSHARD_NO_MODIFY_PATH -eq "1")
$MinisignMode = if ($env:WAYSHARD_MINISIGN) { $env:WAYSHARD_MINISIGN } else { "auto" }
$MinisignPub = if ($env:WAYSHARD_MINISIGN_PUB) { $env:WAYSHARD_MINISIGN_PUB } else { "$RawBase/keys/wayshard-release.minisign.pub" }

function Write-Info([string]$Message) { Write-Host $Message }

function Fetch-Text([string]$Url) {
  if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
    $out = & curl.exe -fsSL $Url
    if ($LASTEXITCODE -ne 0) { throw "failed to fetch $Url" }
    return ($out -join "`n")
  }
  return (Invoke-WebRequest -UseBasicParsing -Uri $Url).Content
}

function Fetch-File([string]$Url, [string]$Destination) {
  if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
    & curl.exe -fsSL $Url -o $Destination
    if ($LASTEXITCODE -ne 0) { throw "failed to download $Url" }
  }
  else {
    Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $Destination
  }
}

function Main {
  if ($env:OS -ne "Windows_NT") { throw "this installer is for Windows only" }

  switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $arch = "amd64" }
    "ARM64" { throw "no native Windows arm64 build is published; run the x64 archive under emulation or use the CLI+TUI archive" }
    default { throw "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
  }

  if (-not $Version) {
    Write-Info "Resolving the latest stable release of $Repo ..."
    $json = Fetch-Text "$ApiBase/repos/$Repo/releases/latest" | ConvertFrom-Json
    $Version = $json.tag_name
    if (-not $Version) { throw "could not parse the latest release tag from the API response" }
  }

  $archive = "wayshard-$Version-windows-$arch.zip"
  $server = "wayshard-server-$Version-windows-$arch.exe"
  $sums = "SHA256SUMS.txt"
  $sig = "$sums.minisig"

  $work = Join-Path ([System.IO.Path]::GetTempPath()) ("wayshard-install-" + [guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Path $work | Out-Null
  $stage = $null
  try {
    Write-Info "Installing Wayshard $Version (windows-$arch) ..."
    Fetch-File "$DownloadBase/$Version/$archive" (Join-Path $work $archive)
    Fetch-File "$DownloadBase/$Version/$server" (Join-Path $work $server)
    Fetch-File "$DownloadBase/$Version/$sums" (Join-Path $work $sums)

    if ($MinisignMode -ne "skip" -and (Get-Command minisign -ErrorAction SilentlyContinue)) {
      $haveSig = $true
      try { Fetch-File "$DownloadBase/$Version/$sig" (Join-Path $work $sig) } catch { $haveSig = $false }
      if ($haveSig) {
        $pubPath = Join-Path $work "pubkey"
        if ($MinisignPub -match "://") { Fetch-File $MinisignPub $pubPath }
        else {
          if (-not (Test-Path $MinisignPub)) { throw "minisign public key not found: $MinisignPub" }
          Copy-Item $MinisignPub $pubPath
        }
        & minisign -V -p $pubPath -m (Join-Path $work $sums) | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "minisign signature verification failed for $sums" }
        Write-Info "minisign signature verified"
      }
      elseif ($MinisignMode -eq "require") { throw "missing $sig but WAYSHARD_MINISIGN=require" }
      else { Write-Info "note: $sig not published; continuing with SHA-256 verification only" }
    }
    elseif ($MinisignMode -eq "require") { throw "minisign is required but was not found on PATH" }
    else { Write-Info "note: minisign not installed; verifying SHA-256 checksums only" }

    $sumLines = Get-Content -Path (Join-Path $work $sums)
    foreach ($name in @($archive, $server)) {
      $line = $sumLines | Where-Object { ($_ -split '\s+')[1] -eq $name } | Select-Object -First 1
      if (-not $line) { throw "no checksum entry for $name in $sums" }
      $expected = ($line -split '\s+')[0].ToLower()
      $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $work $name)).Hash.ToLower()
      if ($expected -ne $actual) { throw "checksum mismatch for $name (expected $expected, got $actual)" }
      Write-Info "checksum ok: $name"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $stage = Join-Path $InstallDir (".wayshard-stage-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $stage | Out-Null
    Expand-Archive -Path (Join-Path $work $archive) -DestinationPath $stage -Force

    $cli = Join-Path $stage "wayshard.exe"
    $tui = Join-Path $stage "wayshard-tui.exe"
    if (-not (Test-Path $cli)) { throw "archive is missing wayshard.exe" }
    if (-not (Test-Path $tui)) { throw "archive is missing wayshard-tui.exe" }

    Move-Item -Force -Path $cli -Destination (Join-Path $InstallDir "wayshard.exe")
    Move-Item -Force -Path $tui -Destination (Join-Path $InstallDir "wayshard-tui.exe")
    Move-Item -Force -Path (Join-Path $work $server) -Destination (Join-Path $InstallDir "wayshard-server.exe")

    if ($ModifyPath) {
      $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
      if (-not $userPath) { $userPath = "" }
      $parts = @($userPath -split ";" | Where-Object { $_ -ne "" })
      if ($parts -notcontains $InstallDir) {
        $newPath = (@($parts) + $InstallDir) -join ";"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Info "added $InstallDir to the user PATH"
      }
      else { Write-Info "$InstallDir is already on the user PATH" }
    }
    else { Write-Info "skipping PATH modification (WAYSHARD_NO_MODIFY_PATH=1)" }

    Write-Info ""
    Write-Info "Wayshard $Version installed to $InstallDir:"
    Write-Info "  $InstallDir\wayshard.exe        start the interactive client"
    Write-Info "  $InstallDir\wayshard-tui.exe    terminal client companion"
    Write-Info "  $InstallDir\wayshard-server.exe local server daemon"
    Write-Info ""
    Write-Info "Open a new terminal, then run 'wayshard-server' and 'wayshard'."
  }
  finally {
    if ($stage -and (Test-Path $stage)) { Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue }
    if (Test-Path $work) { Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue }
  }
}

try { Main }
catch {
  [Console]::Error.WriteLine("wayshard-install: " + $_.Exception.Message)
  exit 1
}
