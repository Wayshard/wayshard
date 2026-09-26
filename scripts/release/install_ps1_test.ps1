#!/usr/bin/env pwsh
# Fixture-backed test for scripts/install.ps1.
#
# Builds a fake Windows release (zip, server exe, SHA256SUMS.txt, GitHub "latest
# release" API document) in a temp directory and runs the real installer against
# it with file:// bases. It never contacts the live GitHub release.
$ErrorActionPreference = "Stop"

$Root = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$Work = Join-Path ([System.IO.Path]::GetTempPath()) ("wayshard-install-test-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $Work | Out-Null
$Version = "v9.9.9"

function Make-Release([string]$Dest, [string]$Tag) {
  $download = Join-Path $Dest "download/$Tag"
  $api = Join-Path $Dest "api/repos/Wayshard/wayshard/releases"
  $stage = Join-Path $Dest "stage"
  New-Item -ItemType Directory -Force -Path $download, $api, $stage | Out-Null

  Set-Content -Path (Join-Path $stage "wayshard.exe") -Value "Wayshard CLI fixture"
  Set-Content -Path (Join-Path $stage "wayshard-tui.exe") -Value "Wayshard TUI fixture"
  Copy-Item (Join-Path $Root "LICENSE") $stage
  Copy-Item (Join-Path $Root "NOTICE") $stage
  Copy-Item (Join-Path $Root "THIRD_PARTY_NOTICES.md") $stage

  $archive = Join-Path $download "wayshard-$Tag-windows-amd64.zip"
  Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $archive -Force

  $server = Join-Path $download "wayshard-server-$Tag-windows-amd64.exe"
  Set-Content -Path $server -Value "Wayshard server fixture"

  $lines = foreach ($f in @($archive, $server)) {
    $h = (Get-FileHash -Algorithm SHA256 -Path $f).Hash.ToLower()
    "$h  $(Split-Path -Leaf $f)"
  }
  Set-Content -Path (Join-Path $download "SHA256SUMS.txt") -Value $lines

  Set-Content -Path (Join-Path $api "latest") -Value ("{`"tag_name`":`"$Tag`"}")
}

# Invoke-Installer <dest> <installdir> <modifyPath> <version|null>
function Invoke-Installer([string]$Dest, [string]$InstallDir, [bool]$ModifyPath, $VersionOrNull) {
  $base = "file:///" + ($Dest -replace "\\", "/")
  if ($null -ne $VersionOrNull -and $VersionOrNull -ne "") { $env:WAYSHARD_VERSION = $VersionOrNull }
  else { Remove-Item Env:WAYSHARD_VERSION -ErrorAction SilentlyContinue }
  $env:WAYSHARD_INSTALL_DIR = $InstallDir
  $env:WAYSHARD_API_BASE = "$base/api"
  $env:WAYSHARD_DOWNLOAD_BASE = "$base/download"
  $env:WAYSHARD_MINISIGN = "skip"
  if ($ModifyPath) { Remove-Item Env:WAYSHARD_NO_MODIFY_PATH -ErrorAction SilentlyContinue }
  else { $env:WAYSHARD_NO_MODIFY_PATH = "1" }
  & pwsh -NoProfile -NonInteractive -File (Join-Path $Root "scripts/install.ps1") | Out-Host
  return $LASTEXITCODE
}

function Cleanup {
  Remove-Item Env:WAYSHARD_VERSION -ErrorAction SilentlyContinue
  Remove-Item Env:WAYSHARD_INSTALL_DIR -ErrorAction SilentlyContinue
  Remove-Item Env:WAYSHARD_API_BASE -ErrorAction SilentlyContinue
  Remove-Item Env:WAYSHARD_DOWNLOAD_BASE -ErrorAction SilentlyContinue
  Remove-Item Env:WAYSHARD_MINISIGN -ErrorAction SilentlyContinue
  Remove-Item Env:WAYSHARD_NO_MODIFY_PATH -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}

try {
  $dest = Join-Path $Work "rel"
  Make-Release $dest $Version

  $installDir = Join-Path $Work "bin"
  $plainDir = Join-Path $Work "bin-plain"

  Write-Host "==> happy path (latest resolution, checksums)"
  $code = Invoke-Installer $dest $installDir $false $null
  if ($code -ne 0) { throw "installer exited $code" }

  foreach ($name in @("wayshard.exe", "wayshard-tui.exe", "wayshard-server.exe")) {
    if (-not (Test-Path (Join-Path $installDir $name))) { throw "$name not installed" }
  }
  if ((Get-Content (Join-Path $installDir "wayshard.exe") -Raw).Trim() -ne "Wayshard CLI fixture") {
    throw "installed CLI content unexpected"
  }

  Write-Host "==> explicit version + clean re-run"
  $code = Invoke-Installer $dest $plainDir $false $Version
  if ($code -ne 0) { throw "pinned-version installer exited $code" }
  if (-not (Test-Path (Join-Path $plainDir "wayshard.exe"))) { throw "pinned install missing" }

  Write-Host "==> checksum failure is rejected and installs nothing"
  $bad = Join-Path $Work "bad"
  Make-Release $bad $Version
  Add-Content -Path (Join-Path $bad "download/$Version/wayshard-$Version-windows-amd64.zip") -Value "corrupt"
  $badDir = Join-Path $Work "badbin"
  $code = Invoke-Installer $bad $badDir $false $Version
  if ($code -eq 0) { throw "installer accepted a corrupted archive" }
  if (Test-Path (Join-Path $badDir "wayshard.exe")) { throw "corrupted install wrote wayshard.exe" }

  if ($env:CI) {
    Write-Host "==> user PATH update is idempotent (CI only)"
    $orig = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathDir = Join-Path $Work "pathbin"
    try {
      $code = Invoke-Installer $dest $pathDir $true $Version
      if ($code -ne 0) { throw "PATH installer run 1 exited $code" }
      $code = Invoke-Installer $dest $pathDir $true $Version
      if ($code -ne 0) { throw "PATH installer run 2 exited $code" }
      $current = [Environment]::GetEnvironmentVariable("Path", "User")
      $count = @($current -split ";" | Where-Object { $_ -eq $pathDir }).Count
      if ($count -ne 1) { throw "PATH entry appears $count times (want 1)" }
    }
    finally {
      [Environment]::SetEnvironmentVariable("Path", $orig, "User")
    }
  }

  Write-Host "install_ps1_test ok"
}
catch {
  [Console]::Error.WriteLine("install_ps1_test: " + $_.Exception.Message)
  exit 1
}
finally {
  Cleanup
}
