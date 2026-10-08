<#
.SYNOPSIS
  wslc2docker 一键安装（Windows 侧）。自动进入 WSL 以 root 运行 install.sh。

.DESCRIPTION
  默认从 GitHub Releases 下载预编译包并安装到 WSL 默认发行版。
  提供 -LocalDir 可指向本地仓库目录，免发布即可测试。

.PARAMETER Distro
  目标 WSL 发行版名（如 Ubuntu）。默认取当前正在运行的发行版；若无则取默认发行版。

.PARAMETER Repo
  GitHub 仓库 owner/name。默认 wslc2docker/wslc2docker（按需修改）。

.PARAMETER Version
  版本号，对应 Release tag v<Version>。默认 0.1.0。

.PARAMETER LocalDir
  本地仓库目录（含 wslc2docker / endpoints.yaml / wslc2docker.service / install.sh）。
  指定后不上网，直接把目录拷进 WSL 运行 install.sh（开发/测试用）。

.EXAMPLE
  .\install.ps1                                  # 默认发行版 + 下载 release
  .\install.ps1 -Distro Ubuntu                   # 指定发行版
  .\install.ps1 -LocalDir .                      # 用当前目录本地安装（免发布测试）
#>
param(
  [string]$Distro = "",
  [string]$Repo = "wslc2docker/wslc2docker",
  [string]$Version = "0.1.0",
  [string]$LocalDir = ""
)

$ErrorActionPreference = "Stop"

# ---- 选定发行版 ----
if (-not $Distro) {
  $running = (wsl -l --running 2>$null | Where-Object { $_ -match '\S' -and $_ -notmatch 'NAME' } | Select-Object -First 1)
  if ($running) {
    $Distro = ($running -replace '\s.*$', '').Trim()
  } else {
    $def = (wsl -l --running 2>$null) # fallback
    $Distro = (& wsl.exe --list --quiet 2>$null | Select-Object -First 1)
    $Distro = ($Distro -replace '[^A-Za-z0-9-]', '').Trim()
  }
}
if (-not $Distro) {
  Write-Error "无法确定 WSL 发行版，请用 -Distro 指定（如 -Distro Ubuntu）"
}

Write-Host "==> 目标 WSL 发行版: $Distro"

if ($LocalDir) {
  # ---- 本地模式：把目录拷进 WSL 后运行 ----
  $abs = (Resolve-Path $LocalDir).Path
  $remote = "\\wsl$\$Distro\tmp\wslc2docker-install"
  Write-Host "==> 本地模式：复制 $abs -> $remote"
  if (-not (Test-Path $remote)) { New-Item -ItemType Directory -Path $remote -Force | Out-Null }
  Copy-Item -Path "$abs\*" -Destination $remote -Recurse -Force
  wsl.exe -d $Distro -u root -- bash -c "cd /tmp/wslc2docker-install && REPO='$Repo' VERSION='$Version' bash install.sh"
} else {
  # ---- 下载模式：拉取 install.sh 并管道执行 ----
  $url = "https://raw.githubusercontent.com/$Repo/main/install.sh"
  Write-Host "==> 下载安装脚本: $url"
  wsl.exe -d $Distro -u root -- bash -c "REPO='$Repo' VERSION='$Version' curl -fsSL '$url' | bash -s --"
}

Write-Host "==> 完成。如 systemd 未启用，脚本已配置登录钩子；建议按提示开启 WSL systemd。"
