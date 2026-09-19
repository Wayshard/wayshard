# Sign Wayshard Windows installers with a maintainer-generated self-signed PFX.
# Requires WAYSHARD_WINDOWS_PFX_BASE64 and WAYSHARD_WINDOWS_PFX_PASSWORD.
# Materializes the PFX only under $env:RUNNER_TEMP and deletes it afterward.
# No commercial CA, Azure Artifact Signing, Microsoft Store, or timestamping service.
$ErrorActionPreference = "Stop"

function Require-Env([string]$name) {
  $v = [Environment]::GetEnvironmentVariable($name)
  if ([string]::IsNullOrEmpty($v)) {
    throw "missing required GitHub Environment secret $name"
  }
}

Require-Env "WAYSHARD_WINDOWS_PFX_BASE64"
Require-Env "WAYSHARD_WINDOWS_PFX_PASSWORD"

$dest = $args[0]
if ([string]::IsNullOrEmpty($dest) -or -not (Test-Path $dest)) {
  throw "usage: windows-sign.ps1 <dir-of-windows-installers>"
}

$tmpRoot = $env:RUNNER_TEMP
if ([string]::IsNullOrEmpty($tmpRoot)) {
  $tmpRoot = [System.IO.Path]::GetTempPath()
}
$pfx = Join-Path $tmpRoot "wayshard-windows-codesign.pfx"
$bytes = [Convert]::FromBase64String($env:WAYSHARD_WINDOWS_PFX_BASE64)
[IO.File]::WriteAllBytes($pfx, $bytes)
$env:WAYSHARD_WINDOWS_PFX_FILE = $pfx

function Find-SignTool {
  $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $roots = @(
    "${env:ProgramFiles(x86)}\Windows Kits\10\bin",
    "${env:ProgramFiles}\Windows Kits\10\bin"
  )
  foreach ($root in $roots) {
    if (-not (Test-Path $root)) { continue }
    $found = Get-ChildItem -Path $root -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
      Where-Object { $_.DirectoryName -match '\\x64$' } |
      Sort-Object FullName -Descending |
      Select-Object -First 1
    if ($found) { return $found.FullName }
  }
  return $null
}

function Assert-Signed([string]$path) {
  $check = Get-AuthenticodeSignature -FilePath $path
  if ($null -eq $check.SignerCertificate) {
    throw "$path has no signer certificate after signing"
  }
  if ($check.Status -eq "NotSigned") {
    throw "$path reports NotSigned"
  }
  # Self-signed: Status is typically NotTrusted/UnknownError, not Valid.
  Write-Host "verified $path status=$($check.Status) subject=$($check.SignerCertificate.Subject)"
}

$cert = $null
try {
  $targets = @()
  $targets += @(Get-ChildItem -Path $dest -File -Filter *.msi -ErrorAction SilentlyContinue)
  $targets += @(Get-ChildItem -Path $dest -File -Filter *.exe -ErrorAction SilentlyContinue)
  if ($targets.Count -eq 0) { throw "no .msi/.exe in $dest to sign" }

  $secure = ConvertTo-SecureString $env:WAYSHARD_WINDOWS_PFX_PASSWORD -AsPlainText -Force
  $cert = Import-PfxCertificate -FilePath $pfx -CertStoreLocation Cert:\CurrentUser\My -Password $secure
  if (-not $cert) { throw "failed to import PFX" }
  Remove-Item -Force -ErrorAction SilentlyContinue $pfx

  $signtool = Find-SignTool
  if ($signtool) {
    Write-Host "using $signtool"
  } else {
    Write-Host "signtool.exe not found; falling back to Set-AuthenticodeSignature"
  }

  foreach ($f in $targets) {
    Write-Host "signing $($f.FullName)"
    if ($signtool) {
      & $signtool sign /sha1 $cert.Thumbprint /fd SHA256 $f.FullName
      if ($LASTEXITCODE -ne 0) {
        throw "signtool failed for $($f.Name) with exit $LASTEXITCODE"
      }
    } else {
      $sig = Set-AuthenticodeSignature -FilePath $f.FullName -Certificate $cert -HashAlgorithm SHA256
      if ($null -eq $sig.SignerCertificate) {
        throw "Set-AuthenticodeSignature produced no signer for $($f.Name)"
      }
    }
    Assert-Signed $f.FullName
  }
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue $pfx
  if ($cert -and $cert.Thumbprint) {
    $storePath = "Cert:\CurrentUser\My\$($cert.Thumbprint)"
    if (Test-Path $storePath) {
      Remove-Item $storePath -ErrorAction SilentlyContinue
    }
  }
  Get-ChildItem Cert:\CurrentUser\My -ErrorAction SilentlyContinue |
    Where-Object { $_.Subject -like "*Wayshard*" } |
    Remove-Item -ErrorAction SilentlyContinue
}
