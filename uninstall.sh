#!/usr/bin/env bash
#
# wslc2docker 卸载脚本（在 WSL 内的 Linux 侧运行，与 install.sh 对应）
#
# 用法：
#   sudo bash uninstall.sh          # 交互确认后卸载（系统级建议加 sudo）
#   sudo bash uninstall.sh -y       # 跳过确认直接卸载
#   wslc2docker-uninstall -y        # 若已通过 install.sh 装上，可直接用此命令
#
# 说明：会同时清理"系统级(root)"与"用户级"两种可能的安装残留，缺哪个都不报错。
#       建议加 sudo 运行，以确保 /usr/local、/etc、/var/run 下的系统级文件被清掉。
#
set -uo pipefail

SKIP_CONFIRM=0
for a in "$@"; do
  case "$a" in
    -y|--yes) SKIP_CONFIRM=1 ;;
  esac
done

# 当前用户与"实际发起卸载的用户"（sudo 场景下为 SUDO_USER）
REAL_HOME="$HOME"
if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ]; then
  REAL_HOME="$(getent passwd "$SUDO_USER" | cut -d: -f6)"
fi

echo "将卸载 wslc2docker，清理以下可能的位置（不存在则跳过）："
echo "  系统级：/usr/local/bin/wslc2docker(+uninstall)、/etc/wslc2docker、"
echo "          /etc/systemd/system/wslc2docker.service(+.d)、/var/run/docker.sock、"
echo "          /var/log/wslc2docker.log、/etc/profile.d/wslc2docker.sh"
echo "  用户级：\$HOME/.local/bin/wslc2docker(+uninstall)、\$HOME/.config/wslc2docker、"
echo "          \$HOME/.config/systemd/user/wslc2docker.service、\$XDG_RUNTIME_DIR/wslc2docker.sock"
echo

if [ "$SKIP_CONFIRM" != "1" ]; then
  read -r -p "确认卸载？(y/N) " ans
  case "$ans" in
    y|Y|yes|YES) ;;
    *) echo "已取消。"; exit 0 ;;
  esac
fi

# ---- 停止并禁用 systemd 系统服务 ----
echo "==> 停止 systemd 系统服务"
systemctl stop wslc2docker 2>/dev/null || true
systemctl disable wslc2docker 2>/dev/null || true
rm -f /etc/systemd/system/wslc2docker.service
rm -rf /etc/systemd/system/wslc2docker.service.d
systemctl daemon-reload 2>/dev/null || true

# ---- 停止并禁用 systemd 用户服务（当前用户与真实用户）----
echo "==> 停止 systemd 用户服务"
for uh in "$HOME" "$REAL_HOME"; do
  [ -n "$uh" ] || continue
  XDG_U="$uh/.config/systemd/user"
  SYSTEMD_HOME="$uh" systemctl --user stop wslc2docker 2>/dev/null || true
  SYSTEMD_HOME="$uh" systemctl --user disable wslc2docker 2>/dev/null || true
  rm -f "$XDG_U/wslc2docker.service"
  SYSTEMD_HOME="$uh" systemctl --user daemon-reload 2>/dev/null || true
done

# ---- 删除文件（系统级）----
echo "==> 删除系统级文件"
rm -f /usr/local/bin/wslc2docker /usr/local/bin/wslc2docker-uninstall
rm -rf /etc/wslc2docker
rm -f /var/run/docker.sock
rm -f /var/log/wslc2docker.log
rm -f /etc/profile.d/wslc2docker.sh

# ---- 删除文件（用户级：当前用户与真实用户）----
echo "==> 删除用户级文件"
for uh in "$HOME" "$REAL_HOME"; do
  [ -n "$uh" ] || continue
  BD="${XDG_BIN_HOME:-$uh/.local/bin}"
  rm -f "$BD/wslc2docker" "$BD/wslc2docker-uninstall"
  rm -rf "$uh/.config/wslc2docker"
  RD="${XDG_RUNTIME_DIR:-$uh/.cache}"
  rm -f "$RD/wslc2docker.sock"
  rm -rf "${XDG_STATE_HOME:-$uh/.local/share}/wslc2docker"
  # 清理 ~/.profile / ~/.bashrc 里的自动拉起块（带标记）
  for f in "$uh/.profile" "$uh/.bashrc"; do
    [ -f "$f" ] || continue
    sed -i '/# >>> wslc2docker autostart >>>/,/# <<< wslc2docker autostart <<</d' "$f"
  done
done

echo "==> 卸载完成。"
echo "    若想重装："
echo "    sudo bash -c \"\$(curl -sSL https://raw.githubusercontent.com/zhangyuleicn/wslc2docker/main/quick_start.sh)\""
