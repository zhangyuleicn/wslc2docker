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
  $running = wsl -l --running
  foreach ($line in $running) {
    $s = $line.Trim()
    if (($s.Length -gt 0) -and ($s -notlike "NAME*")) {
      $Distro = $s.Split(" ")[0]
      break
    }
  }
}
if (-not $Distro) {
  $def = wsl --list --quiet
  if ($def) {
    $Distro = $def[0].Trim()
    $Distro = $Distro -replace "[^A-Za-z0-9-]", ""
  }
}
if (-not $Distro) {
  Write-Error "Cannot determine WSL distro. Use -Distro (e.g. -Distro Ubuntu)"
  exit 1
}

Write-Host "==> Target WSL distro: $Distro"

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
