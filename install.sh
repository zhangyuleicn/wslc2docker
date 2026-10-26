#!/usr/bin/env bash
#
# wslc2docker 自动安装脚本（在 WSL 内的 Linux 侧运行）
#
# 两种模式，自动判定（无需手动选择）：
#   - root 模式   ：以 root/sudo 运行 -> 装系统路径、systemd 系统服务、socket 在 /var/run/docker.sock
#   - 用户模式   ：普通用户直接运行（无需 sudo）-> 装 ~/.local、systemd --user 或后台运行、
#                  socket 在 $XDG_RUNTIME_DIR/wslc2docker.sock
#
# 在 WSL 内一行运行（无需 root）：
#   bash -c "$(curl -sSL https://raw.githubusercontent.com/zhangyuleicn/wslc2docker/main/quick_start.sh)"
#
# 数据来源（三选一，优先级从高到低）：
#   1) WSLC2DOCKER_LOCAL_TARBALL -> 本地 tar.gz 包（含 wslc2docker / endpoints.yaml）
#   2) WSLC2DOCKER_LOCAL_DIR     -> 本地目录（同上文件）
#   3) 默认                      -> 从 GitHub Releases 下载预编译包
#
# 可覆盖的环境变量：
#   WSLC2DOCKER_REPO       GitHub owner/name（默认见下）
#   WSLC2DOCKER_VERSION    版本号
#   WSLC2DOCKER_ROOT=1     强制系统级安装（非 root 时会尝试 sudo）
#
set -euo pipefail

# ---- 可配置项 ----
REPO="${WSLC2DOCKER_REPO:-zhangyuleicn/wslc2docker}"
VERSION="${WSLC2DOCKER_VERSION:-0.1.1}"
LOCAL_TARBALL="${WSLC2DOCKER_LOCAL_TARBALL:-}"
LOCAL_DIR="${WSLC2DOCKER_LOCAL_DIR:-}"
FORCE_ROOT="${WSLC2DOCKER_ROOT:-0}"

SCRIPT_DIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd)"

# 未显式指定本地源时，默认用本脚本所在目录（直接 bash install.sh 即可，无需下载）
if [ -z "$LOCAL_TARBALL" ] && [ -z "$LOCAL_DIR" ]; then
  if [ -f "$SCRIPT_DIR/wslc2docker" ] && [ -f "$SCRIPT_DIR/endpoints.yaml" ]; then
    LOCAL_DIR="$SCRIPT_DIR"
    echo "==> 未指定本地源，自动使用脚本所在目录：$LOCAL_DIR"
  fi
fi

# ---- 判定安装模式 ----
if [ "$(id -u)" -eq 0 ] || [ "$FORCE_ROOT" = "1" ]; then
  MODE="root"
else
  MODE="user"
fi

# ---- 目标路径（按模式确定）----
if [ "$MODE" = "root" ]; then
  BIN="/usr/local/bin/wslc2docker"
  BIN_DIR="/usr/local/bin"
  CFG_DIR="/etc/wslc2docker"
  CFG="$CFG_DIR/endpoints.yaml"
  SOCK="/var/run/docker.sock"
  LOG="/var/log/wslc2docker.log"
  SVC_DIR="/etc/systemd/system"
  SVC="$SVC_DIR/wslc2docker.service"
  HOOK_DIR="/etc/profile.d"
  HOOK="$HOOK_DIR/wslc2docker.sh"
  SVC_USER="root"
  SVC_WANTED="multi-user.target"
else
  BIN_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
  BIN="$BIN_DIR/wslc2docker"
  CFG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/wslc2docker"
  CFG="$CFG_DIR/endpoints.yaml"
  RUNDIR="${XDG_RUNTIME_DIR:-$HOME/.cache}"
  mkdir -p "$RUNDIR"
  SOCK="$RUNDIR/wslc2docker.sock"
  LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/share}/wslc2docker"
  LOG="$LOG_DIR/wslc2docker.log"
  SVC_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  SVC="$SVC_DIR/wslc2docker.service"
  HOOK="$HOME/.profile"
  SVC_USER=""
  SVC_WANTED="default.target"
fi

if [ "$(uname -m)" != "x86_64" ]; then
  echo "错误：仅支持 x86_64 / amd64，当前为 $(uname -m)" >&2
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> wslc2docker 安装开始 (repo=$REPO version=$VERSION 模式=$MODE)"

# ---- 1. 取得安装文件 ----
if [ -n "$LOCAL_TARBALL" ]; then
  echo "==> 使用本地 tarball：$LOCAL_TARBALL"
  tar -xzf "$LOCAL_TARBALL" -C "$TMP"
elif [ -n "$LOCAL_DIR" ]; then
  echo "==> 使用本地目录：$LOCAL_DIR"
  for f in wslc2docker endpoints.yaml; do
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

for f in wslc2docker endpoints.yaml; do
  if [ ! -f "$TMP/$f" ]; then
    echo "错误：安装包内缺少 $f" >&2; exit 1
  fi
done

# ---- 2. 部署文件 ----
echo "==> 部署二进制与配置"
mkdir -p "$BIN_DIR" "$(dirname "$CFG")" "$(dirname "$LOG")" "$(dirname "$SVC")"
install -m 0755 "$TMP/wslc2docker" "$BIN"
install -m 0600 "$TMP/endpoints.yaml" "$CFG"

# 生成 systemd 单元（用实际路径，避免硬编码 /usr/local、/etc）
{
  echo "[Unit]"
  echo "Description=wslc2docker - Docker Engine API shim for WSLC"
  echo "Documentation=https://github.com/$REPO"
  echo "After=network.target"
  echo ""
  echo "[Service]"
  echo "Type=simple"
  echo "ExecStart=$BIN --socket $SOCK --config $CFG"
  echo "ExecReload=/bin/kill -HUP \$MAINPID"
  echo "Restart=on-failure"
  echo "RestartSec=2"
  if [ -n "$SVC_USER" ]; then
    echo "User=$SVC_USER"
    echo "AmbientCapabilities=CAP_NET_BIND_SERVICE"
  fi
  echo ""
  echo "[Install]"
  echo "WantedBy=$SVC_WANTED"
} > "$SVC"
if [ "$MODE" = "root" ]; then
  chmod 0644 "$SVC"
else
  chmod 0644 "$SVC"
fi

echo "    二进制: $BIN"
echo "    配置:   $CFG (0600)"
echo "    服务:   $SVC"

# ---- 3. 启动（按 systemd 是否可用分支）----
SYSTEMD_AVAILABLE=0
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  if [ "$MODE" = "root" ]; then
    SYSTEMD_AVAILABLE=1
  else
    # 用户级 systemd 需要用户会话总线可用
    if systemctl --user status >/dev/null 2>&1; then
      SYSTEMD_AVAILABLE=1
    fi
  fi
fi

if [ "$SYSTEMD_AVAILABLE" = "1" ]; then
  if [ "$MODE" = "root" ]; then
    echo "==> systemd 可用：启用并启动（系统服务）"
    systemctl daemon-reload
    systemctl enable --now wslc2docker
    systemctl status --no-pager wslc2docker || true
  else
    echo "==> systemd --user 可用：启用并启动（用户服务）"
    systemctl --user daemon-reload
    systemctl --user enable --now wslc2docker
    systemctl --user status --no-pager wslc2docker || true
  fi
else
  echo "==> systemd 不可用：降级为后台启动 + 登录钩子"
  pkill -f "$BIN" 2>/dev/null || true
  setsid "$BIN" --socket "$SOCK" --config "$CFG" >"$LOG" 2>&1 &
  # 登录时若未运行则拉起
  if [ "$MODE" = "root" ]; then
    cat > "$HOOK" <<EOF
#!/usr/bin/env bash
pgrep -f "$BIN" >/dev/null || setsid "$BIN" --socket "$SOCK" --config "$CFG" >"$LOG" 2>&1 &
EOF
    chmod 644 "$HOOK"
    echo "    已写入 $HOOK（新登录会话自动拉起）"
  else
    grep -q "wslc2docker" "$HOOK" 2>/dev/null || cat >> "$HOOK" <<EOF

# wslc2docker 自动拉起
pgrep -f "$BIN" >/dev/null || setsid "$BIN" --socket "$SOCK" --config "$CFG" >"$LOG" 2>&1 &
EOF
    echo "    已写入 $HOOK（新登录会话自动拉起）"
    echo "    提示：将 $BIN_DIR 加入 PATH（如写入 ~/.bashrc：export PATH=\"$BIN_DIR:\$PATH\"）"
  fi
fi

# ---- 4. 验证 ----
sleep 1
echo "==> 验证 socket"
if [ -S "$SOCK" ]; then
  echo "    $SOCK 已就绪"
else
  echo "    警告：$SOCK 未生成，请查日志 $LOG" >&2
fi
echo "    （未装 docker CLI，可用 curl --unix-socket $SOCK http://localhost/_ping 验证）"

echo "==> 安装完成。"
if [ "$MODE" = "root" ]; then
  echo "    停止： sudo systemctl stop wslc2docker   (无 systemd: sudo pkill -TERM wslc2docker)"
  echo "    重启： sudo systemctl restart wslc2docker"
  echo "    热加载：sudo systemctl reload wslc2docker   (改 endpoints.yaml 后)"
  echo "    面板接 unix:///var/run/docker.sock"
else
  echo "    停止： systemctl --user stop wslc2docker   (无 systemd: pkill -TERM wslc2docker)"
  echo "    重启： systemctl --user restart wslc2docker"
  echo "    热加载：systemctl --user reload wslc2docker   (改 endpoints.yaml 后)"
  echo "    面板接 unix://$SOCK"
fi
