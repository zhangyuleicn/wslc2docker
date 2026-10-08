# wslc2docker — 技术方案设计文档

> 一句话定位：**wslc2docker 是一个 Docker Engine API 兼容层（shim），把 Microsoft 原生容器引擎 WSLC（`wslc.exe`）伪装成标准 Docker 守护进程，让既有的 Docker 生态工具（docker CLI、Portainer、1Panel、CI/CD、.NET Aspire、Compose 文件）无需修改即可驱动 WSLC。**
>
> **部署形态（主推）：wslc2docker 作为 Linux 进程直接运行在 WSL 发行版内**，原生创建 `/var/run/docker.sock`。

---

## 0. 术语表

| 术语 | 含义 |
|---|---|
| **WSLC** | WSL Containers，微软自研原生容器引擎（CLI `wslc.exe` / 别名 `container.exe`），随 WSL 3.0.1 于 2026-09-29 GA，支持 Windows 11 22H2+ 与 Windows Server 2025 |
| **wslcsdk / Microsoft.WSL.Containers** | 微软官方给 **Windows 应用**调用的原生 API（NuGet，C/C#/C++，底层 `wslcsdk.dll`）。**仅 Windows 进程内可用**，本方案不对接此面（见 §1.4） |
| **WSL interop** | WSL 的双向互通能力：在 WSL Linux shell 中可直接以全名（含 `.exe`）调用任意 Windows 可执行文件（微软官方支持） |
| **Docker Engine API** | Docker 守护进程暴露的 HTTP REST API（默认端点 `/var/run/docker.sock`），是整个 Docker 生态的通用语 |
| **shim / 兼容层** | 本文档的核心组件，监听 Docker Engine API，内部翻译为 WSLC 调用 |
| **docker.sock** | Linux 侧 Docker 套接字路径；本方案由 WSL 内守护进程**原生创建** |

---

## 1. 背景与问题

### 1.1 事实基线（已核实）
- WSLC 是**微软第一方**容器引擎，定位为 Docker Desktop 的替代，**免费、随 Windows 提供**。
- WSLC 的 CLI 刻意模仿 Docker 语法（`wslc run/build/pull/exec/logs`、`wslc container list`…），且能 `pull`/`run` 标准 OCI 镜像。
- 但 WSLC **刻意不暴露 Docker 生态所需的接口**：
  - ❌ 没有 `dockerd` 守护进程
  - ❌ 不创建 `/var/run/docker.sock`
  - ❌ 不提供 Docker Engine API
  - ❌ 不支持 Docker Compose（`wsl compose up` 官方已确认在开发、无交付日期）
  - ❌ 不支持 Kubernetes（roadmap 未宣布）
- 微软官方给的"管理"手段只有：Windows CLI、Windows 原生 NuGet API、VS Code 集成，以及社区第三方文本/图形面板（lazywslc / WSL Container Desktop）。

### 1.2 目标市场的现实
现代 .NET（Core 3.1+ / 5–10）生态**已经是 Docker-native**：
- 微软官方 .NET 镜像发布在 MCR + Docker Hub；.NET 8+ 支持 `dotnet publish -t:PublishContainer` 直接产出 Docker 镜像。
- Visual Studio / VS Code Dev Containers / .NET Aspire 全部围绕 Docker/OCI 产物。
- 主流实践将 Docker Compose 作为本地开发标准起手式。

### 1.3 问题
WSLC 想"吃掉"这块市场，却**不说 Docker 的话**。.NET 团队已有的 Docker 工具链（面板、compose、CI、Aspire）在 WSLC 上全部失联。这正是 wslc2docker 要补的缺口。

---

### 1.4 WSLC 开放接口定位（本方案对接面）— 回答"接口到底在哪"

WSLC 对外部暴露**两个独立接口面**，二者位置不同、适用场景不同：

| 接口面 | 形态 | 所在侧 | 能否从 WSL Linux 内调用 | 本方案是否使用 |
|---|---|---|---|---|
| **A. `wslc.exe` CLI** | Windows PE 可执行文件（含别名 `container.exe`） | Windows 侧二进制，但经 **WSL interop** 可从 Linux shell 调用 | ✅ `exec.Command("wslc.exe", ...)` | ✅ **主对接面** |
| **B. `Microsoft.WSL.Containers` NuGet / `wslcsdk.dll`** | Windows 原生 C/C#/C++ API（C++/WinRT 投影） | 仅 Windows 进程内（Win32 DLL） | ❌ 不能从 Linux 直接调用 | ❌ 不使用 |

**关键结论（回应架构疑问）**：
- 您担心的"如果开放 API 在 Windows 进程，那只能在 Windows 端开发、监听与跑进程"——**仅对接口面 B 成立**。B 是给"写 Windows 应用"的开发者用的（C#/C++ 调 Win32 DLL），本方案不碰。
- 本方案对接的是**接口面 A（`wslc.exe` CLI）**。WSL interop 让 WSL Linux 内进程直接 exec 这个 Windows 二进制，stdout/stderr 正常回传。因此：
  - **wslc2docker 守护进程是一个纯 WSL Linux 进程**（Go 编译 linux/amd64）；
  - 它监听 `/var/run/docker.sock`（Linux unix socket），服务 1Panel / Portainer / Linux 侧 docker CLI；
  - 仅在"翻译某个 Docker API 请求"时，才经 interop 调一次 `wslc.exe`。
- **WSLC 没有第三个"Linux 原生库/守护进程"接口**：容器以 daemonless 方式跑在独立 utility VM 中，仅由 `wslc.exe` 控制。所以 CLI 是 WSL Linux 侧唯一可行的门——这印证了我们的架构既是唯一、也是正确形态。

> **代价说明**：走 CLI 面（而非 NuGet SDK 面）意味着我们解析 `wslc` 的 stdout（多为 `--format json` 输出），依赖 CLI 输出稳定性，而非类型化 SDK。MVP 足够；若微软未来稳定 SDK 且我们想在 Windows 侧做增强，可再加一个 Windows 模式后端（见 §7 备选 / §10 风险）。

---

### 1.5 双向翻译模型（核心机制）

wslc2docker **不是"单向代理"**，而是把 **Docker 语言** 与 **WSLC 语言** 互相桥接的协议层。面板 / docker CLI / CI / Aspire 用 **docker 语句**（docker 命令或 Docker Engine API 请求）发出的每一个动作，都必须被**翻译**成 WSLC 能执行的动作；反过来，WSLC 的执行结果也必须**翻译回** Docker 形状回传给面板。两个方向缺一不可——这正是"docker 面板 ↔ wslc 应用"双向打通的含义。

```
┌──────────────┐      docker 语句(命令/API)       ┌──────────────────┐     wslc 调用      ┌──────────────┐
│  Docker 面板  │ ───────────────────────────────▶ │   wslc2docker    │ ────────────────▶ │   WSLC 容器  │
│ (1Panel/      │                                   │   翻译层(双向)    │                   │  (utility VM) │
│  Portainer/   │ ◀─────────────────────────────── │                  │ ◀──────────────── │              │
│  docker CLI)  │      Docker 形状响应(Moby schema)  │ ① 入向: Docker→ │     OCI/JSON/stdout│              │
└──────────────┘                                   │       wslc        │                   └──────────────┘
                                                    │ ② 出向: wslc→  │
                                                    │       Docker      │
                                                    └──────────────────┘
```

**① 入向翻译（Docker 语句 → WSLC 应用）— 动作落地**
- 方向：面板 / docker CLI / CI / Aspire 发出的 docker 语句（如 `docker run -d -p 8080:80 --name web nginx`，或 `POST /containers/create` 的 JSON body） → wslc2docker 翻译层 → 调用 `wslc.exe` 在 WSLC utility VM 中实际执行。
- 这是"动作被 WSLC 应用"的方向：**docker 面板上的每一个按钮、每一条命令，最终都变成一条 `wslc` 命令在跑**。
- 端点级映射见 §4「Docker Engine API 端点 → wslc 对接接口」两列；参数级细节见 §5.1（create body → wslc run flags）。

**② 出向翻译（WSLC 状态 → Docker 语句）— 状态同步**
- 方向：`wslc` 的返回（stdout `--format json` / OCI 形状的 inspect / 事件流 / 日志流） → wslc2docker 重塑为 **Moby schema 的标准 Docker Engine API 响应** → 回传面板。
- 这是"面板读得到结果"的方向：没有它，面板即使发出了动作也看不到容器、列表空、详情错。
- 关键重塑见 §4 标注「翻译+重塑」的端点（`inspect` 三兄弟的 OCI→Docker 重塑）、标注「合成」的 `/system/df`（聚合计数）、标注「模拟」的 `restart`（stop+start）。

> **为什么必须双向**：Docker 生态工具只"说 Docker"——它们发出 Docker 语句、也只"读懂" Docker 形状的回答；WSLC 只"说 WSLC"——它接收 wslc 命令、只返回 wslc/OCI 形状。wslc2docker 的价值，正在于在两者之间做**双向实时翻译**，使任意一侧都"以为对方说的是自己的话"。单做入向（只把动作派给 wslc）而忽略出向，面板会"发了指令但看不到容器"；单做出向而无入向，则根本无法驱动 WSLC。

---

## 2. 目标与非目标

### 目标（Goals）
1. 暴露**标准 Docker Engine API**（HTTP over unix socket + 可选 TCP），后端翻译到 WSLC。
2. 让 `docker` CLI、`docker compose`、Portainer、1Panel 等**零修改**连接 WSLC。
3. **守护进程作为 Linux 进程运行在 WSL 内**，原生提供 `/var/run/docker.sock`，1Panel 等工具开箱即用。
4. MVP 阶段即可驱动：容器/镜像的列举、创建、启停、删除、日志、exec、events。
5. 与 WSLC 原生能力演进对齐：未来 `wsl compose up` 落地后，Compose 路径直接转发。

### 非目标（Non-Goals）
- **不**实现容器运行时本身（复用 WSLC）。
- **不**伪造 Kubernetes API（WSLC 无 k8s，超出本方案能力）。
- **不**重新实现镜像构建后端（调用 `wslc build`）。
- **不**处理 Windows 容器（.NET Framework / Windows-only），仅面向 WSLC 的 Linux 容器。

---

## 3. 整体架构（主推：WSL Linux 侧守护进程）

```
┌─────────────────────────────────────────────────────────────────┐
│  WSL2 Linux (Ubuntu)  ←── wslc2docker 主运行环境                  │
│                                                                   │
│   1Panel / Portainer / docker CLI (Linux 侧)                      │
│        │  期望 /var/run/docker.sock                               │
│        ▼                                                          │
│   wslc2docker daemon (Linux 进程, Go 编译的 linux 二进制)          │
│        │  监听 /var/run/docker.sock (原生 unix socket)            │
│        │  可选同时监听 TCP :2375 (供同机 Windows 侧工具)            │
│        ▼                                                          │
│   翻译层：把 Docker Engine API 请求 → 调用 wslc.exe                │
│        │  （经 WSL interop，直接执行 Windows 二进制）              │
│        ▼                                                          │
│   wslc.exe  (Windows 侧控制器，管理 WSLC utility VM 中的容器)       │
└─────────────────────────────────────────────────────────────────┘
        ▲
        │ (Windows 侧 docker CLI / Portainer 也可经 TCP :2375 连接)
┌─────────────────────────────────────────────────────────────────┐
│  Windows Host                                                     │
│   docker CLI / Portainer for Windows  →  tcp://127.0.0.1:2375     │
└─────────────────────────────────────────────────────────────────┘
```

### 关键决策
- **守护进程跑在 WSL Linux 内（主推）**：本方案核心服务对象（1Panel、Portainer、Linux 侧 docker CLI）都期望 `/var/run/docker.sock` 这个 **Linux unix socket**。由一个 Linux 进程直接在该路径创建 socket，工具零配置即可连上，**无需任何 socat / npipe 桥接**。
- **Linux 进程如何驱动 WSLC**：WSL 提供双向互通（interop），在 WSL shell 中可直接以全名（含 `.exe`）调用任意 Windows 可执行文件——微软官方文档明确支持（`wslc.exe`、`powershell.exe` 等）。因此 wslc2docker 用 `exec.Command("wslc.exe", ...)` 即可驱动 WSLC，stdout/stderr 正常回传。
- **翻译层是双向的（见 §1.5）**：面板发出的 docker 语句（命令/API 请求）经「入向翻译」变 `wslc` 调用落到 WSLC；WSLC 的返回再经「出向翻译」重塑为 Moby schema 的 Docker 响应回传面板。两侧都"以为对方说自己的话"，这正是 wslc2docker 能零修改驱动既有 Docker 工具的本质。
- **容器实际在哪里跑**：WSLC 把容器跑在独立的 utility VM 中（与用户 Ubuntu 发行版隔离）。wslc2docker 在 Ubuntu 内建 socket、代理到 wslc、再到 utility VM——这正是 WSLC 的设计意图，1Panel 等工具无需感知这层隔离。
- **Windows 侧工具怎么办（可选）**：daemon 同时监听 TCP `:2375`，Windows 上的 docker CLI / Portainer 设 `DOCKER_HOST=tcp://127.0.0.1:2375` 即可连。一份守护进程同时服务 Linux 侧与 Windows 侧。

### 部署模式对比

| 模式 | 守护进程位置 | /var/run/docker.sock | Windows 侧工具 | 结论 |
|---|---|---|---|---|
| **A. WSL Linux 侧（主推）** | WSL Ubuntu 内（Linux 进程） | 原生创建，直接可用 | 经 TCP :2375 同机连 | ✅ 推荐，零桥接 |
| B. Windows 侧（弃用） | Windows 进程 | 需 socat/npipe 桥接伪 socket | 原生 TCP | ❌ 更复杂，已弃用 |

> 模式 B（Windows 进程 + socat 把 `docker.sock` 桥回 WSL）是 Docker Desktop 的历史做法，但本方案目标工具本就在 WSL 内，模式 A 直接消除桥接层，更简单、更稳。

---

## 4. Docker Engine API 端点映射表 和开发计划表

> **本表是本文档唯一、权威的端点对照 + 开发计划表**，所有信息整合在一张表里，便于综合阅读：
> - 左：`Docker Engine API 端点`（对照官方 v1.56，约 70 个；行轴 = Docker 一侧的全部接口，不只 wslc 开放的）
> - 中：`wslc 对接接口`（WSLC 当前实际可调用 / 对应的命令或能力，或 ❌ 无）
> - 右：`面板调用` / `对接方式` / `微软计划` / `开发计划` / `缺口备注与对策`
>
> **图例**
> - **面板调用**：`核心`(连上即调/列表) · `详情`(点开资源才调) · `常用`(功能按钮) · `否`(面板不调或按钮禁用)
> - **对接方式**：`直接翻译` / `翻译+重塑`(OCI→Docker schema) / `合成`(wslc2docker 自研) / `模拟`(wslc2docker 自研) / `不伪造`
> - **微软计划**（逐行标注，无单独速查块）：`wslc 已可对接` / `Microsoft 开发中` / `Microsoft 未计划`
> - **开发计划**：本项目排期 `P0`(脚手架) → `P1`(MVP) → `P1.5`(面板功能完整) → `P2`(交互) → `P3`(镜像网络卷) → `P4`(Compose) / `不实现`

| 序号 | 资源 | Docker Engine API 端点 | wslc 对接接口（当前能力） | 面板调用 | 对接方式 | 微软计划 | 开发计划 | 缺口备注与对策 |
|---|---|---|---|---|---|---|---|---|
| 1 | 系统 | `GET /_ping` | 无需（恒 OK） | 核心 | 直接翻译 | wslc 已可对接 | P0 | — |
| 2 | 系统 | `HEAD /_ping` | 无需（恒 OK） | 核心 | 直接翻译 | wslc 已可对接 | P0 | — |
| 3 | 系统 | `GET /version` | `wslc version` | 核心 | 直接翻译(合成 docker 形状) | wslc 已可对接 | P0 | 取 wslc 版本号填入 Moby Version 结构 |
| 4 | 系统 | `GET /info` | `wslc system info` | 核心 | 直接翻译(合成) | wslc 已可对接 | P1 | 聚合各 noun 计数合成 Info |
| 5 | 系统 | `GET /events` | `wslc events` | 常用 | 直接翻译(流) | wslc 已可对接 | P2 | — |
| 6 | 系统 | `GET /system/df` | ❌ 无（wslc 不支持） | 核心(首页 dashboard) | 合成(聚合 list 计数+体积估算) | wslc 已可对接 | P1.5 | wslc2docker 自研聚合各 noun list 计算 |
| 7 | 系统 | `POST /system/prune` | ❌ 无整体（仅 per-noun） | 常用(一键清理) | 模拟(聚合各 noun prune) | wslc 已可对接 | P1.5 | 聚合 containers/images/networks/volumes prune |
| 8 | 系统 | `POST /auth` | `wslc login` | 常用(仓库设置) | 直接翻译 | wslc 已可对接 | P2 | — |
| 9 | 容器 | `GET /containers/json` | `wslc container list --format json` | 核心 | 直接翻译 | wslc 已可对接 | P1 | 解析 json 输出 |
| 10 | 容器 | `POST /containers/create` | `wslc container create` / `wslc run` | 核心 | 直接翻译(参数映射) | wslc 已可对接 | P1 | create body→wslc run flags 逐项映射(见 §5.1) |
| 11 | 容器 | `GET /containers/{id}/json` | `wslc inspect -t container`（OCI 形状） | 详情 | 翻译+重塑 | wslc 已可对接 | P1 | inspect 仅 `-t/--type`，无 Go 模板；按 Moby 结构体重塑 |
| 12 | 容器 | `GET /containers/{id}/top` | ❌ 无（wslc 无 top） | 否(禁用) | 不伪造 | Microsoft 未计划 | 不实现 | 面板"进程"按钮禁用 |
| 13 | 容器 | `GET /containers/{id}/logs` | `wslc logs`（`-n/--since/--until/-f`） | 核心 | 直接翻译(流) | wslc 已可对接 | P1 | — |
| 14 | 容器 | `GET /containers/{id}/changes` | ❌ 无（wslc 无 diff） | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 diff |
| 15 | 容器 | `GET /containers/{id}/export` | `wslc container export` | 否 | 直接翻译(tar) | wslc 已可对接 | P3 | — |
| 16 | 容器 | `GET /containers/{id}/stats` | `wslc stats`（一次性快照） | 常用(资源图表) | 直接翻译(快照) | wslc 已可对接 | P2 | 不流式；面板实时图降级为轮询 |
| 17 | 容器 | `POST /containers/{id}/resize` | `wslc attach`/`exec` tty | 否(tty 内部) | 直接翻译 | wslc 已可对接 | P1 | attach/exec 内部用 |
| 18 | 容器 | `POST /containers/{id}/start` | `wslc container start` | 核心 | 直接翻译 | wslc 已可对接 | P1 | — |
| 19 | 容器 | `POST /containers/{id}/stop` | `wslc container stop`（超时默认 5s） | 核心 | 直接翻译 | wslc 已可对接 | P1 | — |
| 20 | 容器 | `POST /containers/{id}/restart` | ❌ 无（wslc 无 restart） | 核心(按钮) | 模拟(stop+start) | wslc 已可对接 | P1 | wslc2docker 用 stop+start 模拟 |
| 21 | 容器 | `POST /containers/{id}/kill` | `wslc kill` | 常用(按钮) | 直接翻译 | wslc 已可对接 | P1.5 | — |
| 22 | 容器 | `POST /containers/{id}/update` | ❌ 无（wslc 无 update） | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 update(资源限制) |
| 23 | 容器 | `POST /containers/{id}/rename` | ❌ 无（wslc 无 rename） | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 rename |
| 24 | 容器 | `POST /containers/{id}/pause` | ❌ 无（wslc 无 pause） | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 pause |
| 25 | 容器 | `POST /containers/{id}/unpause` | ❌ 无（wslc 无 unpause） | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 unpause |
| 26 | 容器 | `POST /containers/{id}/wait` | ❌ 无（wslc 无 wait） | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 wait |
| 27 | 容器 | `DELETE /containers/{id}` | `wslc container rm`（`-f`） | 核心 | 直接翻译 | wslc 已可对接 | P1 | — |
| 28 | 容器 | `POST /containers/{id}/attach` | `wslc attach` | 否(用 ws) | 直接翻译(流) | wslc 已可对接 | P2 | 面板走 websocket |
| 29 | 容器 | `GET /containers/{id}/attach/ws` | `wslc attach` | 常用(控制台) | 直接翻译 | wslc 已可对接 | P2 | — |
| 30 | 容器 | `POST /containers/{id}/exec` | `wslc exec` | 常用(终端入口) | 直接翻译 | wslc 已可对接 | P2 | — |
| 31 | 容器 | `GET /containers/{id}/archive` | `wslc container cp` | 详情(文件浏览) | 直接翻译(tar) | wslc 已可对接 | P3 | — |
| 32 | 容器 | `HEAD /containers/{id}/archive` | `wslc container cp` | 否 | 直接翻译(返回 Path-Stat 头) | wslc 已可对接 | P3 | — |
| 33 | 容器 | `PUT /containers/{id}/archive` | `wslc container cp`（上传） | 详情(上传文件) | 直接翻译(tar 提取) | wslc 已可对接 | P3 | — |
| 34 | 容器 | `POST /containers/prune` | `wslc container prune` | 常用 | 直接翻译 | wslc 已可对接 | P3 | — |
| 35 | Exec | `POST /exec/{id}/start` | `wslc exec` | 常用(终端) | 直接翻译(流) | wslc 已可对接 | P1.5 | — |
| 36 | Exec | `POST /exec/{id}/resize` | `wslc exec` resize | 常用(终端) | 直接翻译 | wslc 已可对接 | P1.5 | — |
| 37 | Exec | `GET /exec/{id}/json` | `wslc exec` | 详情 | 直接翻译 | wslc 已可对接 | P2 | — |
| 38 | 镜像 | `GET /images/json` | `wslc image list --format json` | 核心 | 直接翻译 | wslc 已可对接 | P1 | — |
| 39 | 镜像 | `POST /images/create` | `wslc pull` | 核心(pull) | 直接翻译(流) | wslc 已可对接 | P1 | — |
| 40 | 镜像 | `GET /images/{id}/json` | `wslc inspect -t image`（OCI 形状） | 详情 | 翻译+重塑 | wslc 已可对接 | P1.5 | 按 Moby 结构体重塑 |
| 41 | 镜像 | `GET /images/{id}/history` | ❌ 无（wslc 无 history） | 否 | 不伪造(或合成占位) | Microsoft 未计划 | 不实现 | wslc 无 history |
| 42 | 镜像 | `POST /images/{id}/tag` | `wslc tag` | 常用 | 直接翻译 | wslc 已可对接 | P3 | — |
| 43 | 镜像 | `DELETE /images/{id}` | `wslc image remove` | 常用(rmi 按钮) | 直接翻译 | wslc 已可对接 | P1.5 | — |
| 44 | 镜像 | `GET /images/{id}/get` | `wslc image export` | 否 | 直接翻译(tar) | wslc 已可对接 | P3 | — |
| 45 | 镜像 | `POST /images/{id}/push` | `wslc push` | 常用 | 直接翻译(流) | wslc 已可对接 | P3 | — |
| 46 | 镜像 | `POST /images/load` | `wslc image load` | 否 | 直接翻译 | wslc 已可对接 | P3 | — |
| 47 | 镜像 | `GET /images/get` | `wslc image export` | 否 | 直接翻译(tar) | wslc 已可对接 | P3 | — |
| 48 | 镜像 | `GET /images/search` | ❌ 无（wslc 无 search） | 否(禁用搜索) | 不伪造(可转 Docker Hub 或留空) | Microsoft 未计划 | 不实现 | 面板搜索降级/禁用 |
| 49 | 镜像 | `GET /images/{id}/attestation` | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | 1.56 新增，wslc 无 |
| 50 | 镜像 | `DELETE /images/prune` | `wslc image prune` | 常用 | 直接翻译 | wslc 已可对接 | P3 | — |
| 51 | 镜像 | `POST /build` | `wslc build -t`（Containerfile） | 常用(构建按钮) | 直接翻译(上下文) | wslc 已可对接 | P3 | 上下文流落盘后 wslc build |
| 52 | 镜像 | `POST /build/prune` | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 buildkit 缓存清理 |
| 53 | 镜像 | `GET /build/cache` | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 buildkit |
| 54 | 网络 | `GET /networks` | `wslc network list` | 常用 | 直接翻译 | wslc 已可对接 | P3 | — |
| 55 | 网络 | `POST /networks/create` | `wslc network create`（`--network host` 被拒） | 常用 | 直接翻译(host 降级/报错) | wslc 已可对接 | P2 | wslc 拒绝 --network host；降级 bridge |
| 56 | 网络 | `GET /networks/{id}` | `wslc inspect -t network`（OCI 形状） | 详情 | 翻译+重塑 | wslc 已可对接 | P1.5 | 按 Moby 结构体重塑 |
| 57 | 网络 | `DELETE /networks/{id}` | `wslc network rm` | 常用 | 直接翻译 | wslc 已可对接 | P2 | — |
| 58 | 网络 | `POST /networks/{id}/connect` | `wslc network connect` | 常用 | 直接翻译 | wslc 已可对接 | P2 | — |
| 59 | 网络 | `POST /networks/{id}/disconnect` | `wslc network disconnect` | 常用 | 直接翻译 | wslc 已可对接 | P2 | — |
| 60 | 网络 | `DELETE /networks/prune` | `wslc network prune` | 常用 | 直接翻译 | wslc 已可对接 | P2 | — |
| 61 | 卷 | `GET /volumes` | `wslc volume list` | 常用 | 直接翻译 | wslc 已可对接 | P3 | — |
| 62 | 卷 | `POST /volumes/create` | `wslc volume create`（bind 走 VirtioFS） | 常用 | 直接翻译 | wslc 已可对接 | P2 | bind 卷经 VirtioFS，语义略有差异 |
| 63 | 卷 | `GET /volumes/{name}` | `wslc inspect -t volume`（OCI 形状） | 详情 | 翻译+重塑 | wslc 已可对接 | P1.5 | 按 Moby 结构体重塑 |
| 64 | 卷 | `PUT /volumes/{name}` | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | Swarm-only 卷更新 |
| 65 | 卷 | `DELETE /volumes/{name}` | `wslc volume rm` | 常用 | 直接翻译 | wslc 已可对接 | P2 | — |
| 66 | 卷 | `DELETE /volumes/prune` | `wslc volume prune` | 常用 | 直接翻译 | wslc 已可对接 | P2 | — |
| 67 | 分发/会话 | `GET /distribution/{name}/json` | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无 registry 信息接口 |
| 68 | 分发/会话 | `POST /session` | ❌ 无 | 否 | 不伪造(普通流代替) | Microsoft 未计划 | 不实现 | exec/stdin 多路复用握手；普通流功能等价 |
| 69 | 编排 | Compose API（`/v1/.../compose/*`、`docker compose`） | ❌ 无运行时（`wsl compose up` 待 MS） | 常用(Stack) | 转发 `wsl compose up`(待 MS)或本地 compose→wslc 翻译 | Microsoft 开发中 | P4 | 官方头号需求 #40948，确认在开发、无日期；落地前 MVP 可线性翻译 compose.yaml |
| 70 | 编排 | `GET /swarm` 等 Swarm 全套（init/join/leave/update/unlockkey/unlock、nodes/services/tasks/secrets/configs 的 CRUD） | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | 官方明确超出范围，建议用 k8s |
| 71 | 编排 | Kubernetes API | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现(Roadmap 未宣布) | 引擎缺口，无法伪造 |
| 72 | 插件 | `GET /plugins` 等插件全套（privileges/install/enable/disable/upgrade/set/create/push） | ❌ 无 | 否 | 不伪造 | Microsoft 未计划 | 不实现 | wslc 无插件机制 |

> **一句话**：本表把「Docker 公开接口」与「wslc 当前可对接接口」左右并列，并标注面板调用频度、对接方式、微软计划（wslc 已可对接 / Microsoft 开发中 / Microsoft 未计划）与开发计划（P0–P4 / 不实现）。`wslc 已可对接` 今天即用 wslc 跑（其中 `system/df`、`system/prune`、`restart` 由 wslc2docker 自研合成/模拟兜底）；`Microsoft 开发中` 微软承诺做（仅 Compose）；`Microsoft 未计划` 微软明确不做/未宣布（top/diff/rename/pause/wait/update/history/search/attestation/build-cache/distribution/session/Swarm 全套/Plugin 全套/k8s）。`否` 类端点会在面板上表现为按钮禁用或留空——这是明确的兼容边界，不构成技术阻断。

> **Schema 保真度是最大工作量**：每个响应 JSON 必须严格匹配 Moby 的类型定义（`Container`、`Image`、`Network`、`Volume` 等）。建议**直接复用 Moby 的 `api/types` Go 结构体**来构造响应，避免手写 schema 漂移。

---

## 5. 转译核心难点

### 5.1 create body → wslc run 参数
Docker `POST /containers/create` 的 JSON body 字段极多（HostConfig、ExposedPorts、Mounts、Env、Cmd…）。需逐项映射：
- `ExposedPorts` + `HostConfig.PortBindings` → `-p host:container`
- `HostConfig.Binds` / `Mounts` → `-v / --mount`
- `Env` → `-e`
- `Cmd` / `Entrypoint` → 容器命令
- `HostConfig.RestartPolicy` → wslc 暂无原生 restart，用 Windows 计划任务或容器标签近似（标注为"尽力而为"）

### 5.2 ID ↔ Name 映射层（必须）
Docker 世界用 64 位十六进制 ID；WSLC 用 name。shim 需维护一张 **内存 + 持久化（bolt/db 或 JSON 文件）的 ID↔Name 映射表**，并对所有 `/containers/{id}` 请求做解析。新建容器时为 WSLC 生成确定性 name（如 `w2d-<shortid>`），并记录。

### 5.3 流式语义
`logs` / `exec` / `attach` / `events` / `stats` 都是长连接流。Docker 用 chunked/分帧传输；需把 `wslc` 的子进程 stdout/stderr 正确 multiplex 成 Docker 的流格式（尤其 `exec` 的 stdin/stdout/stderr 多路复用）。这是 P2 的主要复杂度。

### 5.4 构建上下文
`POST /build` 上传的是 tar 上下文流。MVP 可把流落盘为构建目录后调用 `wslc build -t {tag} .`；更优是 `wslc build` 支持 stdin 上下文（待验证）。

---

## 6. Compose 与 k8s 边界

| 能力 | 状态 | wslc2docker 策略 |
|---|---|---|
| 单容器生命周期 | WSLC 原生支持 | 直接翻译 ✅ |
| Docker Compose | WSLC 官方 `wsl compose up` **在开发** | 等原生落地后**直接转发**；落地前 MVP 可用本地 compose.yaml→多 `wslc run` 翻译（参考社区 `docker2wslc` 思路） |
| Kubernetes | WSLC **不支持**（roadmap 未宣布） | **超出本方案能力**，明确列为硬限制；不伪造 |

> 结论：Compose 是"时间问题"，k8s 是"引擎缺口"。wslc2docker 对 Compose 是桥接，对 k8s 是弃权。

---

## 7. 技术栈与选型（已确认）

### 7.1 语言：**Go**（已确认，非仅供选择）
- 编译目标 **linux/amd64**，单一静态二进制，作为普通 Linux 守护进程安装于 WSL 发行版内（配合 systemd 或后台进程常驻）。
- 标准库 `net/http` 实现 Docker API 服务端极顺手；`net` 直接监听 unix socket `/var/run/docker.sock`。
- 调用 WSLC 用 `os/exec` 执行 `wslc.exe`（经 WSL interop，从 Linux 侧直接调 Windows 二进制），简单可靠。
- 可**直接 import Moby 的 `github.com/moby/moby/api/types`** 保证响应 schema 与 Docker 一致（这是选 Go 的决定性优势，见 §12 选型论证）。
- **不再需要 Windows 命名管道 / socat 桥接**（socket 原生创建于 WSL 内）。
- 备选 Rust（性能更优、内存更安全，但开发成本高、Moby 类型复用不如 Go 直接）——仅作长期选项，本期不采用。

### 7.2 配置与状态：**单个 `endpoints.yaml`，不用数据库、核心不含 Web UI**（已确认）
- **唯一机器真相源 = `endpoints.yaml`**：72 个端点的「开/关」、wslc 映射、备注都在这一份结构化文件里；守护进程启动时加载，直接按开关决定端点行为（关了的返回 `501 Not Implemented` / 禁用）。
- **为何不用数据库**：记录量小（~72 条基本静态）、无并发多写、无关系查询需求；YAML 即版本可控、可 review、无外部依赖。SQLite 属过度设计。
- **为何核心不含 Web UI**：原想做"接口开关页面化"，但 YAML 本身就是结构化清单、`design.md` 的表即其人读版，文本编辑 ≈ 页面编辑且**攻击面归零**（无 HTTP 认证面、无前端代码）。Web UI 降级为 **Phase 5 可选**（只读/编辑前端），不进核心。
- `design.md` 的端点表与 `endpoints.yaml` 保持对应：`design.md` 给人看与评审，`endpoints.yaml` 给程序用。

### 7.3 运维模型：**root 受控修改 + SIGHUP 热加载**（已确认）
- 配置文件权限 **`root:root` / `0600`**，仅经 `sudo` 进入 WSL 才能改（与改 `/etc/docker/daemon.json` 同一套心智模型），天然安全。
- 不强制重启进程：守护进程监听 **`SIGHUP`** 重新读取 `endpoints.yaml` 即时生效；`systemctl restart wslc2docker` 亦可。`SIGHUP` 实现成本极低，优先支持。
- Docker Engine API 套接字（`/var/run/docker.sock`）保持 unix socket + root 仅权限、**不加登录**，与真实 dockerd 一致（客户端不认自定义登录）。**绝不**在 `0.0.0.0` 暴露 TCP `:2375` 而不加 TLS+鉴权。

**License**：**Apache-2.0**（与 Moby 一致，利于生态协作）。仅调用 WSLC 的 CLI/API，不 redistributable 微软专有代码。

---

## 8. 实施阶段

- **P0 — 脚手架**：Go module（linux/amd64）、HTTP + unix-socket 服务、配置（socket 路径、wslc 路径、TCP 开关）、`/_ping` + `/version` 打通。
- **P1 — MVP（驱动 docker CLI 基本命令 + 面板连通）**：`containers/json`、`images/json`、`containers/create`、`start/stop/restart`、`rm`、`inspect`、`logs`、`images/create(pull)`。
- **P1.5 — 面板功能完整（必需，前移自 P2/P3）**：`/system/df` 合成、单资源 inspect（`/images/{id}/json`、`/networks/{id}`、`/volumes/{name}`）、`/images/{id}` 删除（rmi）、`/containers/{id}/kill`、exec 生命周期（`/exec/{id}/start`、`/exec/{id}/resize`、websocket exec）。**无此阶段面板首页/详情/终端会残缺**。
- **P2 — 交互能力**：`attach`、`events`、`stats`、`top`，以及 `/volumes`、`/networks` 的 create/connect/disconnect/rm、`/auth` 登录。
- **P3 — 镜像与网络**：`build`、`push`、`tag`、`networks`、`volumes`、archive 双向。
- **P4 — Compose 桥接**：WSLC 原生 compose 落地后转发；否则本地 compose.yaml→wslc 翻译器。
- **P5 —（可选）Web 管理界面**：仅当需图形化开关/编辑 `endpoints.yaml` 时再加；非核心，核心用 YAML + root 受控修改（见 §7.2）。

---

## 9. 验证方案

```bash
# 在 WSL Ubuntu 内启动 wslc2docker（默认监听 /var/run/docker.sock + TCP :2375）

# 1. Linux 侧原生 socket（1Panel / Portainer / docker CLI 直接可用）
docker ps
docker images
docker run -d -p 8080:80 --name web nginx
docker logs web
docker exec web cat /etc/os-release
docker stop web && docker rm web

# 2. 面板验证
#    1Panel：安装时指向 /var/run/docker.sock，应正常调出 Docker 管理
#    Portainer：连接 unix:///var/run/docker.sock，应能看到容器列表

# 3. Windows 侧工具（可选，经 TCP）
#    PowerShell: $env:DOCKER_HOST="tcp://127.0.0.1:2375"; docker ps
```

---

## 10. 风险与对策

| 风险 | 对策 |
|---|---|
| **微软未来自建 Docker 兼容层** → 本方案冗余 | 以开源占位、做事实标准；若微软补上则上游化或转型做"增强层" |
| **WSLC API 仍处演进期**（preview→GA、C++/WinRT 投影变动） | 抽象出"WSLC 后端接口"，CLI 与 SDK 两种实现可切换；跟进官方 changelog |
| **WSL interop 行为变动** | interop 为微软长期支持能力；P0 验证 `wslc.exe` 从 WSL 内调用正常；保留 `command_not_found_handle`/alias 兜底 |
| **Schema 保真度**是持续工作量 | 复用 Moby `api/types`，编写契约测试对照真实 dockerd 输出 |
| **k8s 永远无法伪造** | 文档明确硬限制，避免用户误期 |
| **Compose 语义复杂**（依赖图/健康检查/网络） | 等 WSLC 原生 compose；过渡期仅做"尽力而为"的线性翻译 |

---

## 11. 项目元信息（建议）

```
wslc2docker/
├── cmd/wslc2docker/main.go      # 入口：加载配置、启动 HTTP + unix socket 服务
├── internal/
│   ├── api/                    # Docker Engine API 路由与处理器
│   ├── translate/              # create body → wslc flags 等转译逻辑
│   ├── backend/                # WSLC 后端（cli.go：调用 wslc.exe；sdk.go：Windows 模式可选）
│   ├── idmap/                  # ID ↔ Name 映射（持久化）
│   └── socket/                 # /var/run/docker.sock 创建与权限
├── config.yaml                 # 监听地址、wslc 路径、日志级别
├── go.mod
└── README.md
```
> 编译目标：linux/amd64；安装方式：复制到 WSL 发行版内，作为 systemd 服务或后台常驻进程。

---

## 12. 附录：WSLC 命令参考（来自官方文档）

| 用途 | 命令 |
|---|---|
| 运行容器 | `wslc run -d --rm -p 8080:80 --name web nginx` |
| 列容器 | `wslc container list [--all]` |
| 启/停 | `wslc container start/stop <name>` |
| 删容器 | `wslc container rm <name>` / `wslc container prune` |
| 拷贝 | `wslc container cp <name>:/path ./local` |
| 日志 | `wslc container logs <name>` |
| 执行 | `wslc exec <name> <cmd>` |
| 检查 | `wslc container inspect <name>` |
| 镜像列表 | `wslc image list` |
| 拉/推 | `wslc pull/push <ref>` |
| 构建 | `wslc build -t <tag> .`（使用 Containerfile） |
| 网络 | `wslc network create/connect/disconnect` |
| 系统信息 | `wslc system info` |
| 事件流 | `wslc events` |
| 资源统计 | `wslc stats` |

---

*文档版本：v2.2（设计稿）· 2026-10-05 · 变更：§7 技术选型**三项确认落地**——① 语言定 **Go**（linux/amd64，决定性优势为可 import Moby 类型保证 schema 一致）；② 配置用**单个 `endpoints.yaml`、不用数据库、核心不含 Web UI**（Web 降级为 Phase 5 可选，YAML 即结构化清单、文本编辑≈页面编辑且攻击面归零）；③ 运维模型为 **root 受控修改（`0600`）+ `SIGHUP` 热加载/可重启**，docker.sock 保持 root 仅权限不加登录、禁 `0.0.0.0` 裸暴露 `:2375`。*
