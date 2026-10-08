# wslc2docker one-click installer (Windows side).
# Enters WSL as root and runs install.sh.
# Usage:
#   .\install.ps1                     # default distro + download release
#   .\install.ps1 -Distro Ubuntu      # specify distro
#   .\install.ps1 -LocalDir .         # install from local dir (no publish needed)

param(
  [string]$Distro = "",
  [string]$Repo = "zhangyuleicn/wslc2docker",
  [string]$Version = "0.1.0",
  [string]$LocalDir = ""
)

$ErrorActionPreference = "Stop"

# Pick a distro
if (-not $Distro) {
  # Primary: read registered distros from registry (no console-encoding issues)
  try {
    $lxss = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Lxss"
    $keys = Get-ChildItem $lxss -ErrorAction SilentlyContinue
    foreach ($k in $keys) {
      $nm = (Get-ItemProperty $k.PSPath -ErrorAction SilentlyContinue).DistributionName
      if ($nm -and $nm.Trim().Length -gt 0) { $Distro = $nm.Trim(); break }
    }
  } catch { }
}
if (-not $Distro) {
  # Fallback: wsl --list --quiet, force UTF-8 output and keep printable ASCII only
  try {
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    $list = wsl --list --quiet
    foreach ($line in $list) {
      $s = $line.Trim() -replace "[^ -~]", ""
      if ($s.Length -gt 0) { $Distro = $s; break }
    }
  } catch { }
}
if (-not $Distro) {
  Write-Error "Cannot determine WSL distro. Use -Distro (e.g. -Distro Ubuntu)"
  exit 1
}

Write-Host "==> Target WSL distro: $Distro"

# Ensure the distro is running, otherwise the \\wsl$ share is unreachable for local copy.
try { wsl.exe -d $Distro -- echo ok | Out-Null } catch { }

if ($LocalDir) {
  $abs = (Resolve-Path $LocalDir).Path
  $remote = "\\wsl$\$Distro\tmp\wslc2docker-install"
  Write-Host "==> Local mode: copy $abs -> $remote"
  if (-not (Test-Path $remote)) {
    New-Item -ItemType Directory -Path $remote -Force | Out-Null
  }
  Copy-Item -Path "$abs\*" -Destination $remote -Recurse -Force
  wsl.exe -d $Distro -u root -- bash -c "cd /tmp/wslc2docker-install; REPO='$Repo' VERSION='$Version' bash install.sh"
} else {
  $url = "https://raw.githubusercontent.com/$Repo/main/install.sh"
  Write-Host "==> Download installer: $url"
  wsl.exe -d $Distro -u root -- bash -c "REPO='$Repo' VERSION='$Version' curl -fsSL '$url' | bash -s --"
}

Write-Host "==> Done. If systemd is not enabled, install.sh configured a login hook; enable WSL systemd per its hint."
