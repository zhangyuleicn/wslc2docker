# wslc2docker

把微软原生容器引擎 **WSLC**（`wslc.exe`）伪装成一个标准 **Docker Engine** 的兼容层（shim）。
让 docker CLI / Portainer / 1Panel / CI / .NET Aspire 等既有 Docker 工具，零修改地驱动 WSLC 跑容器。

- **入向翻译**：面板、docker CLI 发出的 docker 语句（`docker run ...` 或 `POST /containers/create`）→ 翻译成 `wslc` 调用 → 在 WSLC 的 utility VM 中真正执行。
- **出向翻译**：wslc 返回的 OCI/JSON/stdout → 重塑成 Moby schema 的标准 Docker 响应回传面板。
- 守护进程是**纯 WSL Linux 进程**（Go，linux/amd64），原生创建 `/var/run/docker.sock` 服务 1Panel / Portainer。

> 详细设计（端点对照、双向翻译、能力边界）见 [`design.md`](./design.md)。

---

## 0. 部署教程（5 分钟上手）

下面是一份**完整的端到端部署流程**：从准备 WSLC 环境、编译二进制、部署到 systemd、再到把 1Panel / Portainer 接上来。每一步都可照抄。

### 0.0 一键自动安装（推荐）

不想手动敲命令？用仓库自带安装器，一条命令搞定：

**方式 A — Windows 侧一键（推荐）**：在 PowerShell 里运行 `install.ps1`，它会自动选默认 WSL 发行版、以 root 进 WSL 跑 `install.sh` 完成编译/部署/systemd 启动。

```powershell
# 已发布到 GitHub Releases 时（默认从 release 下载预编译包）
.\install.ps1

# 指定发行版
.\install.ps1 -Distro Ubuntu

# 尚未发布、本地直接测：用当前目录里的二进制/配置/脚本（免联网）
.\install.ps1 -LocalDir .
```

**方式 B — WSL 内手动跑**：把仓库拷进 WSL 后，

```bash
sudo bash install.sh
```

安装器会自动处理：二进制下载/部署、`/etc/wslc2docker`（0600）、systemd 单元启用启动；若 WSL 未开 systemd 则降级为 nohup 后台 + 登录钩子（并提示如何开启 systemd）。

> **发布预编译包**（让方式 A 默认路径可用）：给仓库打 `v*` tag 推送即触发 `.github/workflows/release.yml`，自动构建 linux/amd64 并上传 `wslc2docker-v<version>-linux-amd64.tar.gz`（含二进制 + endpoints.yaml + service + install.sh）。把 `install.sh` / `install.ps1` 顶部的 `REPO` 改成你的 `owner/name` 即可。

### 0.1 前提条件

| 项 | 说明 |
|---|---|
| Windows | Windows 11 / Windows Server 2025，已启用 **WSL2**（`wsl --install` 或已就绪） |
| WSLC | 微软原生容器引擎 `wslc.exe` 可用（GA 版 3.0.1）。在 **PowerShell** 验证：`wslc --version`；在 **WSL Linux shell** 验证：`wslc --version`（经 WSL interop 调用 `.exe`） |
| 构建机 | 仅**编译**需要 Go 1.22+；目标运行机是 WSL 内的 Linux（amd64） |
| 权限 | 守护进程监听 `/var/run/docker.sock` 需 **root**（与真实 dockerd 一致） |

> 若 `wslc --version` 在 WSL 内返回「command not found」：WSLC 的 `wslc.exe` 通常已在 Windows PATH，但 WSL interop 需确保 `/mnt/c/...` 在 PATH 且未禁用。可在 WSL 内试 `wslc.exe --version`（带 `.exe` 后缀）确认 interop 正常。

### 0.2 编译二进制（在构建机 / WSL 内均可）

```bash
cd wslc2docker
GOOS=linux GOARCH=amd64 go build -o wslc2docker ./cmd/wslc2docker
# 产物：wslc2docker（Linux amd64 静态二进制）
```

> 没有 Go 也可：在任意已装 Go 的机器交叉编译后把二进制拷进 WSL 即可。

### 0.3 部署文件到 WSL（在 WSL Linux shell 中执行）

```bash
# 1) 二进制 -> PATH
sudo cp wslc2docker /usr/local/bin/wslc2docker
sudo chmod 755 /usr/local/bin/wslc2docker

# 2) 配置 -> /etc/wslc2docker（root:root，0600）
sudo mkdir -p /etc/wslc2docker
sudo cp endpoints.yaml /etc/wslc2docker/endpoints.yaml
sudo chown root:root /etc/wslc2docker/endpoints.yaml
sudo chmod 600 /etc/wslc2docker/endpoints.yaml

# 3) systemd 单元
sudo cp wslc2docker.service /etc/systemd/system/wslc2docker.service
sudo systemctl daemon-reload
```

### 0.4 选择后端并启用端点

打开配置，确认两处关键项（其余端点按 `design.md` 的 P0/P1/P1.5 默认已 `enabled: true`）：

```bash
sudo nano /etc/wslc2docker/endpoints.yaml
```

- `backend: wslc` —— 接真实 `wslc.exe`（生产）。
  （无 WSLC 想先验证 HTTP 层时，临时改成 `backend: fake` 跑通后再切回 `wslc`。）
- 想临时关掉某个端点：把对应项的 `enabled: false`。

> 真实 `wslc` 后端的 JSON 字段解析是「尽力而为」映射，首次上真机请用 `wslc` 实际 `--format json` 输出校准（见 §11 说明）。

### 0.5 启动并设为开机自启

```bash
sudo systemctl enable --now wslc2docker
systemctl status wslc2docker        # 应显示 active (running)
```

确认 socket 已生成：

```bash
ls -l /var/run/docker.sock          # srw-rw---- root root
```

### 0.6 验证 Docker 兼容面

```bash
docker ps
docker images
docker run -d -p 8080:80 --name web nginx
docker logs web
docker exec web cat /etc/os-release
docker stop web && docker rm web
```

### 0.7 接入管理面板

- **1Panel**：进入「容器」设置，把 Docker 连接地址填为 `unix:///var/run/docker.sock`（与本机 dockerd 填法完全一致）。1Panel 不再需要单独装 docker-ce。
- **Portainer**：新建 Environment 选 Docker，Endpoint URL 填 `unix:///var/run/docker.sock`。
- 仅本机调试可用 TCP：先改 YAML 加 `--tcp 127.0.0.1:2375` 重启，面板填 `tcp://127.0.0.1:2375`（**切勿**绑 `0.0.0.0`）。

### 0.8 改配置后的热加载（不重启）

```bash
sudo nano /etc/wslc2docker/endpoints.yaml
sudo systemctl reload wslc2docker     # 发 SIGHUP，即时生效，不中断面板连接
```

### 0.9 常见排错

| 现象 | 排查 |
|---|---|
| `docker ps` 报 `permission denied` | 当前用户不在 docker 组，或 socket 权限不对；用 `sudo docker ps` 验证，或把用户加入 `docker` 组 |
| `Is the docker daemon running?` | 进程没起：`systemctl status wslc2docker`；看日志 `journalctl -u wslc2docker -n 50` |
| `wslc: command not found`（后端为 wslc） | WSL interop 未打通；在 WSL 内试 `wslc.exe --version`，确认 PATH 含 Windows 目录 |
| 面板连上但按钮灰（501） | 该端点属 P2/P3/P4 或「不实现」阶段，`endpoints.yaml` 中 `enabled: false`，属设计内兼容边界（见 §11） |
| TCP 连不上 | 确认 `--tcp 127.0.0.1:2375` 已加且 `reload/restart`；防火墙仅放行本机回环 |

### 0.10 卸载

```bash
sudo systemctl disable --now wslc2docker
sudo rm -f /usr/local/bin/wslc2docker /etc/systemd/system/wslc2docker.service
sudo rm -rf /etc/wslc2docker /var/run/docker.sock
sudo systemctl daemon-reload
```

---

## 1. 构建

需要本机或 WSL 内安装 Go 1.22+。

```bash
cd wslc2docker
GOOS=linux GOARCH=amd64 go build -o wslc2docker .
```

产物：`wslc2docker`（Linux 静态二进制）。

---

## 2. 配置

唯一机器真相源是 **`endpoints.yaml`**：72 个 Docker Engine API 端点的「开/关」、wslc 映射、备注都在这里。
关掉的端点返回 `501 Not Implemented`（面板表现为按钮禁用 / 留空）。

部署位置与权限（与改 `/etc/docker/daemon.json` 同一模型）：

```bash
sudo mkdir -p /etc/wslc2docker
sudo cp endpoints.yaml /etc/wslc2docker/endpoints.yaml
sudo chown root:root /etc/wslc2docker/endpoints.yaml
sudo chmod 600 /etc/wslc2docker/endpoints.yaml
```

> 仅经 `sudo` 进入 WSL 才能改此文件 —— 天然安全，无需 Web / 数据库。

---

## 3. 运行（前台）

```bash
sudo ./wslc2docker --socket /var/run/docker.sock --config /etc/wslc2docker/endpoints.yaml
```

- 默认监听 unix socket `/var/run/docker.sock`（服务 1Panel / Portainer / Linux 侧 docker CLI）。
- 可选 TCP（仅本机调试用）：`--tcp 127.0.0.1:2375`。
  ⚠️ **绝不在 `0.0.0.0` 裸暴露 `:2375`**（无 TLS + 无鉴权 = 裸奔）。

---

## 4. 运行（systemd 守护进程，推荐）

仓库内含 `wslc2docker.service` 示例单元，安装即用：

```bash
sudo cp wslc2docker /usr/local/bin/
sudo cp wslc2docker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now wslc2docker
```

查看状态：

```bash
systemctl status wslc2docker
```

---

## 5. 停止

```bash
# 方式一：systemd（推荐）
sudo systemctl stop wslc2docker

# 方式二：直接发信号（前台或非 systemd 运行时）
sudo pkill -TERM wslc2docker
```

进程收到 `SIGTERM` 后优雅退出并删除 `/var/run/docker.sock`。

---

## 6. 重启

```bash
# 方式一：systemd（推荐）
sudo systemctl restart wslc2docker

# 方式二：手动
sudo pkill -TERM wslc2docker
sudo ./wslc2docker --socket /var/run/docker.sock --config /etc/wslc2docker/endpoints.yaml
```

---

## 7. 热加载配置（改了 endpoints.yaml 不必重启）

守护进程监听 **`SIGHUP`**，收到后重新读取 `endpoints.yaml` 即时生效：

```bash
# 改配置
sudo nano /etc/wslc2docker/endpoints.yaml

# 热加载（推荐，不中断已连面板）
sudo systemctl reload wslc2docker
# 等价手动写法：
# sudo pkill -HUP wslc2docker
```

> `systemctl reload` 依赖单元里的 `ExecReload=/bin/kill -HUP $MAINPID`（见 `wslc2docker.service`）。

---

## 8. 修改端点的标准流程

1. `sudo nano /etc/wslc2docker/endpoints.yaml` —— 改某端点的 `enabled: true/false` 或映射。
2. `sudo systemctl reload wslc2docker` —— SIGHUP 热加载生效。
3. 验证（见下）。

---

## 9. 验证

```bash
docker ps
docker images
docker run -d -p 8080:80 --name web nginx
docker logs web
docker exec web cat /etc/os-release
docker stop web && docker rm web
```

若以上命令正常返回，说明 wslc2docker 已把 WSLC 伪装成标准 Docker Engine。
Portainer / 1Panel 连 `unix:///var/run/docker.sock` 即可管理 WSLC 容器。

---

## 10. 安全注意事项

- `endpoints.yaml` 权限 `0600`，仅 root 可改。
- docker Engine API 套接字保持 **root 仅权限、不加登录**（与真实 dockerd 一致；客户端不认自定义登录）。
- 启用 TCP 时务必绑定 `127.0.0.1`；跨网络暴露必须叠 TLS + 鉴权。
- 本工具用于**开发 / 自托管**场景，替代 Docker Desktop / docker-ce 在 WSL 内提供 Docker 兼容面；生产环境请评估 Compose / k8s 边界（见 `design.md`）。

---

## 11. 当前实现状态（v0.1.0）

按 `design.md` 的阶段推进，已实现 **P0 + P1 + P1.5**（面板连通与核心功能所需的端点）：

| 阶段 | 状态 | 端点（id） |
|---|---|---|
| P0 脚手架 | ✅ | 1,2 `/_ping`、3 `/version` |
| P1 MVP | ✅ | 9 容器列表、10 创建、11 inspect、13 日志、18 start、19 stop、20 restart(模拟)、27 rm、38 镜像列表、39 pull |
| P1.5 面板完整 | ✅ | 4 `/info`、6 `/system/df`(合成)、17 resize、21 kill、30/35/36/37 exec、34 prune、40 镜像 inspect、43 rmi、56 网络 inspect、63 卷 inspect |
| P2 交互 | ⏳ 待开发 | 5 events、8 auth、16 stats、28/29 attach/ws、54/55/57-60 网络、61-62/65-66 卷 |
| P3 镜像网络卷 | ⏳ 待开发 | 15/31-33 archive、42 tag、44-47/50-51 镜像、54 等 |
| P4 Compose | ⏳ 待开发 | 69 Compose（`wsl compose up` 待微软） |
| 不实现 | — | 12/14/22-26/41/48/49/52/53/64/67/68/70/71/72（微软未计划，按设计返回 501） |

> 已实现的端点由 `endpoints.yaml` 的 `enabled: true` 控制；未实现的端点（含 `不实现`）返回 `501 Not Implemented`，面板表现为按钮禁用 / 留空——这是明确的兼容边界。
>
> **wslc 后端解析说明**：真实后端 `internal/backend/wslc.go` 通过 `wslc.exe --format json` 调用并解析输出。其 JSON schema 以 WSLC GA 版为准，当前为「尽力而为」映射（见各 `reshape*` 函数）。部署到真实 WSLC 时，应以实际 `wslc` 输出校准字段名（单点修正即可，不影响路由与响应塑形）。

## 12. 本地自测（fake 后端，无需 WSLC）

在无 WSLC 的机器上可用内置 `fake` 后端验证 HTTP 层、路由与响应塑形：

```bash
# 仅 TCP（跳过 unix socket 便于本机 curl）
./wslc2docker --backend fake --socket none --tcp 127.0.0.1:23750 --config endpoints.yaml

# 另一个终端
curl http://127.0.0.1:23750/_ping
curl http://127.0.0.1:23750/version
curl http://127.0.0.1:23750/containers/json
curl -X POST "http://127.0.0.1:23750/containers/create?name=web" -d '{"Image":"nginx"}'
```

也可用 Go 测试覆盖（已随仓库提供）：

```bash
go test ./...
```

## 13. 构建（从源码）

```bash
# 目标平台产物（WSL 内运行）
GOOS=linux GOARCH=amd64 go build -o wslc2docker ./cmd/wslc2docker

# 本机（Windows/macOS）调试
go build -o wslc2docker ./cmd/wslc2docker
```

## 许可证

Apache-2.0（与 Moby 一致）。仅调用 WSLC 的 CLI/API，不 redistributable 微软专有代码。
