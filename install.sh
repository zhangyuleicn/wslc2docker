#!/usr/bin/env bash
#
# wslc2docker 自动安装脚本（在 WSL 内的 Linux 侧以 root 运行）
#
# 通常由 Windows 侧的 install.ps1 调用；也可手动在 WSL 里执行：
#   sudo bash install.sh
#
# 数据来源（三选一，优先级从高到低）：
#   1) WSLC2DOCKER_LOCAL_TARBALL  -> 本地 tar.gz 包（含 wslc2docker / endpoints.yaml / wslc2docker.service）
#   2) WSLC2DOCKER_LOCAL_DIR      -> 本地仓库目录（同上三个文件）
#   3) 默认                       -> 从 GitHub Releases 下载预编译包
#
# 可覆盖的环境变量：
#   WSLC2DOCKER_REPO     GitHub 仓库 owner/name（默认见下）
#   WSLC2DOCKER_VERSION  版本号（默认见下）
#
set -euo pipefail

# ---- 可配置项 ----
REPO="${WSLC2DOCKER_REPO:-wslc2docker/wslc2docker}"   # TODO: 改成你的 GitHub 仓库
VERSION="${WSLC2DOCKER_VERSION:-0.1.0}"
LOCAL_TARBALL="${WSLC2DOCKER_LOCAL_TARBALL:-}"
LOCAL_DIR="${WSLC2DOCKER_LOCAL_DIR:-}"

# ---- 目标路径（与 design.md / README 一致）----
BIN="/usr/local/bin/wslc2docker"
CFG_DIR="/etc/wslc2docker"
CFG="$CFG_DIR/endpoints.yaml"
SVC="/etc/systemd/system/wslc2docker.service"
SOCK="/var/run/docker.sock"
LOG="/var/log/wslc2docker.log"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> wslc2docker 安装开始 (repo=$REPO version=$VERSION)"

# ---- 0. 前置检查 ----
if [ "$(id -u)" -ne 0 ]; then
  echo "错误：需以 root 运行（sudo bash install.sh）" >&2
  exit 1
fi
if [ "$(uname -m)" != "x86_64" ]; then
  echo "错误：仅支持 x86_64 / amd64，当前为 $(uname -m)" >&2
  exit 1
fi

# ---- 1. 取得安装文件 ----
if [ -n "$LOCAL_TARBALL" ]; then
  echo "==> 使用本地 tarball：$LOCAL_TARBALL"
  tar -xzf "$LOCAL_TARBALL" -C "$TMP"
elif [ -n "$LOCAL_DIR" ]; then
  echo "==> 使用本地目录：$LOCAL_DIR"
  for f in wslc2docker endpoints.yaml wslc2docker.service; do
    if [ ! -f "$LOCAL_DIR/$f" ]; then
      echo "错误：本地目录缺少 $f" >&2; exit 1
    fi
    cp "$LOCAL_DIR/$f" "$TMP/$f"
  done
else
  URL="https://github.com/$REPO/releases/download/v$VERSION/wslc2docker-v$VERSION-linux-amd64.tar.gz"
  echo "==> 从 GitHub Releases 下载：$URL"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP/wslc2docker.tar.gz"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP/wslc2docker.tar.gz" "$URL"
  else
    echo "错误：需要 curl 或 wget 来下载预编译包" >&2; exit 1
  fi
  tar -xzf "$TMP/wslc2docker.tar.gz" -C "$TMP"
fi

for f in wslc2docker endpoints.yaml wslc2docker.service; do
  if [ ! -f "$TMP/$f" ]; then
    echo "错误：安装包内缺少 $f" >&2; exit 1
  fi
done

# ---- 2. 部署文件 ----
echo "==> 部署二进制与配置"
install -m 0755 "$TMP/wslc2docker" "$BIN"
mkdir -p "$CFG_DIR"
install -m 0600 "$TMP/endpoints.yaml" "$CFG"
install -m 0644 "$TMP/wslc2docker.service" "$SVC"
echo "    二进制: $BIN"
echo "    配置:   $CFG (0600)"
echo "    服务:   $SVC"

# ---- 3. 启动（按 systemd 是否可用分支）----
if [ -d /run/systemd/system ]; then
  echo "==> systemd 可用：启用并启动"
  systemctl daemon-reload
  systemctl enable --now wslc2docker
  systemctl status --no-pager wslc2docker || true
else
  echo "==> systemd 不可用：降级为 nohup 后台启动 + 登录钩子"
  pkill -f "$BIN" 2>/dev/null || true
  setsid "$BIN" --socket "$SOCK" --config "$CFG" >"$LOG" 2>&1 &
  # 最佳努力：每个 login shell 起来时若未运行则拉起
  cat > /etc/profile.d/wslc2docker.sh <<EOF
#!/usr/bin/env bash
pgrep -f "$BIN" >/dev/null || setsid "$BIN" --socket "$SOCK" --config "$CFG" >"$LOG" 2>&1 &
EOF
  chmod 644 /etc/profile.d/wslc2docker.sh
  echo "    已写入 /etc/profile.d/wslc2docker.sh（新登录会话自动拉起）"
  echo "    建议开启 WSL systemd（更稳）："
  echo "     在 WSL 内： echo -e '[boot]\nsystemd=true' | sudo tee -a /etc/wsl.conf"
  echo "     在 Windows 侧： wsl --shutdown   然后重新进入 WSL"
fi

# ---- 4. 验证 ----
sleep 1
echo "==> 验证 socket"
if [ -S "$SOCK" ]; then
  echo "    $SOCK 已就绪"
else
  echo "    警告：$SOCK 未生成，请查日志 $LOG" >&2
fi
if command -v docker >/dev/null 2>&1; then
  echo "==> docker ps"
  docker ps 2>&1 | head -5 || true
else
  echo "    （未装 docker CLI，可用 curl http://localhost/_ping 或 1Panel/Portainer 验证）"
fi

echo "==> 安装完成。"
echo "    停止： sudo systemctl stop wslc2docker   (无 systemd: sudo pkill -TERM wslc2docker)"
echo "    重启： sudo systemctl restart wslc2docker"
echo "    热加载：sudo systemctl reload wslc2docker   (改 endpoints.yaml 后)"
echo "    面板接 unix:///var/run/docker.sock"
