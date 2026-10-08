#!/usr/bin/env bash
#
# wslc2docker 一键安装脚本（类 1Panel quick_start 模式）
#
# 在 WSL / Linux 终端里直接运行下面这行即可（无需手动下载、无需参数）：
#   bash -c "$(curl -sSL https://raw.githubusercontent.com/zhangyuleicn/wslc2docker/main/quick_start.sh)"
#
# 可覆盖的环境变量：
#   WSLC2DOCKER_VERSION  指定版本，例如 v0.1.1（默认取 GitHub 最新 Release）
#   WSLC2DOCKER_REPO     GitHub owner/name（默认 zhangyuleicn/wslc2docker）
#
set -euo pipefail

REPO="${WSLC2DOCKER_REPO:-zhangyuleicn/wslc2docker}"
# API 限流 / 无 Release 时的兜底版本（随新版本发布时同步更新）
FALLBACK_VERSION="v0.1.1"

# ---- 下载工具 ----
have_curl=0; have_wget=0
command -v curl >/dev/null 2>&1 && have_curl=1
command -v wget >/dev/null 2>&1 && have_wget=1
if [ "$have_curl" = 0 ] && [ "$have_wget" = 0 ]; then
  echo "错误：需要 curl 或 wget 才能下载安装包" >&2
  exit 1
fi
http_get()  { if [ "$have_curl" = 1 ]; then curl -fsSL "$1"; else wget -qO- "$1"; fi; }
http_down() { if [ "$have_curl" = 1 ]; then curl -fsSL "$1" -o "$2"; else wget -qO "$2" "$1"; fi; }

# ---- 架构 ----
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "错误：不支持的架构 $arch（当前仅发布 amd64 预编译包）" >&2; exit 1 ;;
esac

# ---- 版本 ----
if [ -n "${WSLC2DOCKER_VERSION:-}" ]; then
  VERSION="$WSLC2DOCKER_VERSION"
else
  echo "==> 查询 GitHub 最新 Release ..."
  body="$(http_get "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null || true)"
  VERSION="$(printf '%s' "$body" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*:[[:space:]]*//; s/[" ]//g')"
  if [ -z "$VERSION" ]; then
    echo "    未能通过 API 获取最新版本（限流或暂无 Release），使用兜底版本 $FALLBACK_VERSION"
    VERSION="$FALLBACK_VERSION"
  fi
fi
echo "==> 版本：$VERSION   架构：$ARCH"

# ---- 下载 tarball ----
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

TARBALL="wslc2docker-${VERSION}-linux-${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$TARBALL"
echo "==> 下载安装包：$URL"
if ! http_down "$URL" "$TMP/$TARBALL"; then
  echo "错误：下载失败，请确认版本 $VERSION 的 Release 已发布，或手动指定 WSLC2DOCKER_VERSION" >&2
  exit 1
fi
tar -xzf "$TMP/$TARBALL" -C "$TMP"

# ---- 用仓库最新的 install.sh（保证与 main 同步，不依赖包内旧版）----
echo "==> 拉取最新 install.sh"
if ! http_down "https://raw.githubusercontent.com/$REPO/main/install.sh" "$TMP/install.sh"; then
  echo "    警告：无法拉取最新 install.sh，退回使用包内版本"
fi
if [ ! -f "$TMP/install.sh" ]; then
  echo "错误：安装包内缺少 install.sh" >&2
  exit 1
fi

# ---- 提权运行安装（install.sh 会自动用同目录的二进制/配置，无需再次联网）----
if [ "$(id -u)" -eq 0 ]; then
  bash "$TMP/install.sh"
else
  echo "==> 需要 root 权限完成安装，正在提权（输入 sudo 密码）"
  sudo bash "$TMP/install.sh"
fi
