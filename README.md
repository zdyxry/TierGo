# TierGo

TierGo 是用 [MyGo](https://github.com/egoist/mygo) 编写的**非官方**
[EasyTier](https://github.com/EasyTier/EasyTier) 桌面客户端。名称中的
`Tier` 来自 EasyTier，`Go` 来自 MyGo 和 Go。

界面是 MyGo 原生 UI（无 HTML / JS / WebView），网络部分用**官方的原生 `easytier-core`**，每个本地网络各启动一个独立进程。数据面由官方 Core 处理，没有 wasm 或自制网卡；这不表示 GUI 已暴露 Core 的所有功能。

界面参考 Tailscale 的信息结构：左侧导航 + 状态卡片 + 设备表。

## 架构

```
┌────────────────────────────────────┐
│  Go + MyGo 原生 UI（无窗口依赖）      │
│  概览 / 节点 / 路由 / 网络配置 / 日志  │
│  Controller：状态机 + 故障处理        │
└──────────┬─────────────────────────┘
           │ coreEngine 边界（可替换、可测试）
           │ 生成 config.toml · 探测 · 监督 · 停止
┌──────────▼─────────────────────────┐
│  easytier-core（独立进程）            │
│  内核自己建 TUN、配路由、             │
│  子网代理、打洞、加密                 │
└────────────────────────────────────┘
```

桌面包的不只是「生成 TOML」：它还要挑不冲突的端口、写配置文件、提权、
**探测内核是否真的工作**、盯着进程别偷偷死掉、抓日志并脱敏、轮询状态、
停止并升级到强杀、以及清理上次崩溃留下的孤儿进程。

| 部分 | 做法 |
|---|---|
| 界面 | MyGo 原生 UI（`ui` 包），无 HTML / JS / WebView |
| 内核 | 官方 `easytier-core` 二进制，**内嵌在应用里**，独立进程运行 |
| 状态 | 官方 `easytier-cli -o json`（轮询 node / peer / route） |
| 依赖 | 应用本身纯 Go、无 cgo；Go 侧只依赖 MyGo 和 `golang.org/x/sys` |

**独立实例**：客户端用**自己的配置目录、监听端口和管理端口**（自动挑选空闲端口），不会影响机器上已有的 EasyTier 服务，也不会占用默认的 11010 / 15888。

## 功能

- **概览**：连接状态、虚拟 IP、在线节点数、路由数、连接时长、本设备属性（含代理网段、公网 IP、NAT 类型）、最近事件
- **节点**：设备名 / 虚拟 IP / 延迟 / 直连或中继 / 收发流量 / 版本，可搜索、可排序
- **路由**：目标网段 / 下一跳 / 跳数 / 延迟 / 主机名 / 代理网段
- **网络配置**：网络名称、密码、本机名称、虚拟 IP、对端节点、监听地址、代理网段、端口转发、虚拟网卡 / 加密 / 安全模式 / 延迟优先
- **日志**：内核事件 + 本机操作；默认只记错误，级别可在设置里调（越高越费磁盘）
- **关于**：客户端版本、commit、内核版本、配置文件与日志路径、管理端口

### 新增功能与当前进度

- **多网络**：侧栏创建/切换网络档案，可同时连接；每个网络独立 Core、端口、配置、日志。关闭窗口隐藏到托盘，从托盘退出才停止本地网络。
- **状态栏/托盘**：显示连接状态、当前网络、虚拟 IP、在线节点数和各网络状态，可快速启停、打开主窗口或日志；图标随连接状态变化。
- **配置**：TOML 导入/导出/编辑、高级接口/性能/路由/出口/代理/P2P/传输/IPv6 设置；导入只保留配置模型支持的字段，不承诺任意 Core TOML 无损。
- **访问控制**：三类 ACL 链、规则、IP/端口范围、Stateful/PPS/Burst、组声明与组成员。
- **监控**：原始逐连接字节计数速率图、丢包/NAT、接口详情、Core 原生事件时间线。Core 事件环仅保留最近 20 条，轮询间可能遗漏。
- **系统服务**：安装/启动/停止/卸载、Config Server 服务安装（稳定 Machine ID）。系统服务的真实开机/提权行为仍需目标平台验证。
- **应用设置**：语言、登录启动客户端；与单个网络的配置和系统服务分开。
- **远端**：显式连接 RPC 查看实例、基础网络创建、完整官方 NetworkConfig JSON 编辑、停用/启用与确认删除。停用先将完整配置备份到本机，按远端地址隔离，保留凭据和配置来源；启用需要该备份。RPC 无认证，必须使用可信隧道。
- **VPN Portal**：v2.6.4 共享配置和 v2.7/main 逐设备模式明确区分；设备增删、ACL 组分配、在线状态、逐设备配置复制/下载与本地二维码。新模式需要支持的 Core，启动前检查版本。
- **日志**：off/error/warn/info/debug/trace，运行时调整，打开/复制日志目录。

- **集中管理**：普通模式可启动受监督的 Config Server Core，关闭 GUI 时停止；服务模式独立运行。官方 Config Server 下发、断线重连和客户端重启恢复均有真实集成测试。
- **语言**：应用设置页即时切换简体中文/English，保存选择，导航、设置、弹窗、错误与托盘同步切换。用户网络名称和 Core 原始日志不翻译。

完整实施与测试边界见 [`docs/implementation-progress.md`](docs/implementation-progress.md)。Android 不在本轮实现范围，仍未交付。

配置目录（Linux）：`~/.config/easytier-mygo/`，其他系统使用 `os.UserConfigDir()`。

| 路径 | 用途 |
|---|---|
| `networks/<id>.json` | 网络档案，含网络密码，权限 0600 |
| `instances/<id>/config.toml`、`logs/` | 每个本地实例的 Core 配置与日志 |
| `app.json` | 客户端设置和集中管理参数 |
| `service-configs/` | 系统服务的本地网络配置 |
| `central-configs/` | 系统服务 Config Server 下发配置，由 Core 管理 |
| `central/central-configs/` | 普通模式 Config Server 配置（与服务隔离） |
| `remote-disabled/<endpoint-hash>/<id>.json` | 停用远端网络的完整配置备份，含凭据，权限 0600 |

旧 `config.json` 会迁移为默认网络档案并保留，便于回退。

## 安装与运行

**发布版内嵌官方 `easytier-core` 和 `easytier-cli`**（压缩后约 10 MB），
首次运行释放到 `~/.config/easytier-mygo/bin/`，无需单独安装 EasyTier。
Windows 的 Npcap 不由本客户端分发；需要 Npcap 的功能请先自行从
<https://npcap.com> 安装。各平台仍可能需要自己的桌面运行库。

从源码构建时，先把内核抓进仓库（该目录不进版本控制）。默认从
`core-lock.json` 读取版本、当前平台的 asset 名和 SHA-256，并强制校验：

```sh
go run ./cmd/fetch-core                             # 当前平台的锁定版本
go run ./cmd/fetch-core -platform darwin/arm64      # 指定平台，可累积
go tool mygo build -platform darwin/universal
go tool mygo build                             # 打包，内核已在里面
```

显式指定 `-version`（包括 `latest`）时，若锁文件中没有该版本与平台的记录，
必须同时提供 `-sha256`。`-lock ""` 可显式禁用锁文件，此时也必须显式提供
`-version` 和 `-sha256`；工具不再接受无摘要的下载。例如：

```sh
go run ./cmd/fetch-core -version v2.7.0 -sha256 <release-zip-sha256>
```

不带内嵌内核也能构建：程序会退回到「找机器上已有的 EasyTier」，
顺序是环境变量 → 应用同目录 → `~/.config/easytier-mygo/bin/` → `PATH`。

```sh
export EASYTIER_CORE=/path/to/easytier-core    # 手动指定
export EASYTIER_CLI=/path/to/easytier-cli
```

### macOS 构建

```sh
go run ./cmd/fetch-core -platform darwin/arm64
go run ./cmd/fetch-core -platform darwin/amd64
go tool mygo build -platform darwin/universal      # TierGo.app，含两种架构
```

在 Linux 上交叉编译 `darwin/universal` 会得到 `.app`，但**不会得到 `.dmg`**（打镜像需要 macOS）。
macOS runner 会同时生成 `.app` 和 `.dmg`，发布流程再把 `.app` 打成 zip。

本地开发构建使用 ad-hoc 签名，不等同于 Developer ID 或苹果公证。
正式发布工作流要求 `MACOS_CERTIFICATE`、`MACOS_SIGNING_IDENTITY` 和
notarytool 凭据，缺任何一个都会停止发布，不上传未签名的正式资产。

关于页和 `TierGo --version` 会显示客户端版本、commit 和 Core 版本。

### 内核内嵌

| | |
|---|---|
| 放在哪 | `internal/corebin/assets/<goos>-<goarch>/`，**不进 Git**（已 gitignore） |
| 怎么来 | `go run ./cmd/fetch-core`：按 `core-lock.json` 下载官方 release zip 并强制校验 SHA-256，验证必需文件后在输出目录内 staging，全部成功后原子替换对应平台目录；失败不留下半成品 |
| 嵌什么 | `easytier-core`、`easytier-cli`（Windows 上游 `.exe` 也规范化为同名 `.gz`）；Windows 额外强制要求 `wintun.dll` / `WinDivert64.sys`；不分发 Npcap `Packet.dll`，不带 `easytier-web*` |
| 多大 | Linux 约 +10.6 MB，macOS 约 +30.5 MB；具体以各版本构建产物为准 |
| 按平台内嵌 | `assets_<goos>_<goarch>.go` 决定嵌入哪个平台：`darwin/universal` 两个架构各带各的内核，Linux 构建不会带上 macOS 的资源（`fetch-core -clean` 可清掉其他平台） |
| 溯源 | 每次抓取会写入 `provenance.json`（版本、asset 名、zip 的 SHA-256，不含抓取时间），随构建一起内嵌，界面的「关于」页会显示 |

## 虚拟网卡与权限

EasyTier 的**内核自己创建和管理 TUN**（Linux 的 `/dev/net/tun`、macOS 的 utun、Windows 的 wintun/WinDivert），
这也是为什么客户端里没有任何平台相关的网络代码。

创建网卡需要管理员权限：

- 打开虚拟网卡时，客户端会在概览页提示「需要管理员权限」，点「以管理员身份重启」即可
  （Linux 走 pkexec，macOS 走系统授权弹窗，Windows 走 UAC）。
- 不想给权限也可以：**关掉虚拟网卡**，内核照样加入网络、看得到节点和路由、端口转发也能用。

### 有 TUN 和没有 TUN 差在哪

| 能力 | 有 TUN | 无 TUN |
|---|---|---|
| 入网、发现节点、学到路由、看状态 | ✅ | ✅ |
| 作为中继帮别的节点转发 | ✅ | ✅ |
| **本机任意程序访问虚拟网络里任意主机的任意端口** | ✅ | ❌ |
| 本机端口 → 对端（出向端口转发） | ✅ | ✅ |
| 把本机局域网共享给其他节点（子网代理） | ✅ | ✅ |
| 别人访问本机某个端口（入向） | ✅ | ❌（需 TUN 或内核的 DataPlane API） |
| 需要管理员权限 | 需要 | 不需要 |

一句话：**要「整机接入虚拟网络」就必须有 TUN**；无 TUN 只剩点对点的出向转发和子网代理。

`bind_addr` 永远是**本机**地址，所以端口转发只能写成本机 → 对端：

```
tcp://127.0.0.1:5202/10.144.0.20:5201
```

## 代码结构

分两半，和架构图对应：**窗口**那半只管画，**内核**那半只管跑。

```
main.go                    入口：读配置、建控制器、开窗口、退出时停内核
internal/
  app/                     窗口这一半（只依赖 easytier 包，不碰进程）
    app.go                 App 状态、根视图、侧边栏、顶栏、提权提示、通用部件
    page_overview.go       概览页        page_routes.go    路由页
    page_peers.go          节点页        page_settings.go  网络配置页
    page_preferences.go    应用设置页    page_service.go   系统服务页
    page_logs.go           日志页        page_about.go     关于页
    style.go               主题（配色、圆角）与内联 SVG 图标
    view_test.go           无窗口的界面测试（ui.NewTester）
    render_test.go         把每页渲染成 PNG，改样式时肉眼对比
  easytier/                内核这一半（不依赖 app，也不依赖任何界面代码）
    controller.go          Controller 状态机：启动 → 探测 → 轮询 → 故障处理
    engine.go              coreEngine 边界：真实实现跑 easytier-core，可替换
    cli.go                 二进制查找 + easytier-cli 的 JSON
    process.go             进程原语（合并 stdout/stderr、优雅停止、强杀）
    config.go              Config 读写、实例目录
    toml.go                生成内核的 config.toml、端口挑选、端口转发解析
    views.go               CLI 的 JSON → 界面用的视图模型
    orphan.go              清理上次崩溃留下的内核进程（按平台核对进程名）
    elevate_*.go           提权（Linux pkexec / macOS osascript / Windows UAC）
    durable.go             持久化写入 + 日志脱敏
    controller_test.go     用假内核测整个状态机
    fake_test.go           假内核（脚本化：什么时候失败、什么时候崩）
    e2e_test.go            真跑官方 easytier-core 的端到端测试
  corebin/                 内嵌内核：释放、版本标记、可执行位
cmd/fetch-core/            抓取官方 release 并准备内嵌资源
```

`internal/easytier` 不 import `internal/app`：界面依赖内核，反过来不行。

## 测试

```sh
go test -short ./...    # 视图单测 + 纯逻辑，0.15 秒
go test ./...           # 全部，约 5 秒（需要官方二进制，否则自动跳过 e2e）
```

| 层 | 内容 |
|---|---|
| 视图单测 | `ui.NewTester` 无窗口驱动界面：点击、输入、断言文本 |
| 配置生成 | `buildInstanceTOML` 的 TOML 内容、端口转发解析、端口挑选 |
| CLI 解析 | 用**真实 `easytier-cli` 输出**当夹具（`testdata/cli-*.json`），保证解析跟着工具走 |
| **e2e** | 真启动 `easytier-core`：控制器状态机、节点信息、独立目录/端口；再起第二个 core 连上来，验证客户端发现它并画在界面上 |
| **内核内嵌** | 释放、可执行位、重复释放不重写、版本不匹配时替换 |
| **抓取工具** | 平台名映射、只取需要的文件、解压与压缩的正确性 |
| 静态检查 | `go tool mygo vet .`（抓「把 `ui.Element` 存进 struct/全局」这类生命周期错误） |

```sh
go test -run TestE2E -v        # 有内嵌内核时直接跑；否则跳过
EASYTIER_CORE=/tmp/et/easytier-core EASYTIER_CLI=/tmp/et/easytier-cli go test -run TestE2E -v
#   TestE2EControllerWithRealCore   真跑内核，验证状态机 / 节点信息 / 独立配置
#   TestE2EPeerDiscovery            第二个内核连上来，验证发现与界面
```

渲染截图（改样式时肉眼对比）：

```sh
RENDER=1 go test -run TestRenderScreens    # 输出到 /tmp/easytier-shots
```

## 已验证 / 未验证

已验证：

- `linux/amd64`、`linux/arm64`、`windows/amd64`、`darwin/amd64`、`darwin/arm64` 都能编译
- e2e 真跑官方 `easytier-core` 2.6.4：两个节点经控制器连上、延迟 0.11 ms、界面正确显示（`cost: p2p`）
- 配置生成、端口挑选（不与系统已有的 EasyTier 冲突）、CLI JSON 解析
- 每个页面用 `ui.Render` 无头渲染成 PNG，界面即截图所见
- `go vet`、`go tool mygo vet`、`gofmt` 干净

未验证：

- **真正创建 TUN**：当前沙箱连 `sudo ip tuntap add` 都被内核拒绝（EPERM），所以只验证到管理员权限缺失时的提示路径。
- **macOS / Windows 的实际运行**：提权与打包只做了编译验证（这两个平台我这边跑不了）。
- 提权后的整机连通性（需要一台有权限的机器）。

## 进程与状态管理

参考 [k-carrier](https://github.com/zdyxry/k-carrier) 的做法，把「谁在跑进程」和「怎么判断它好了」分开：

| 机制 | 做法 |
|---|---|
| **引擎边界** | `coreEngine` 接口负责启动/就绪/探测/停止；真实实现跑官方二进制，测试用脚本化的假内核 |
| **先探测再宣布** | 管理端口能连上 ≠ 内核工作正常。只有**完整读回一次状态**之后才进入「已连接」 |
| **崩溃会被发现** | 内核自己退出时，界面从「已连接」转为「失败」并写明原因，不会停在假状态 |
| **不隐瞒读不到的部分** | 节点列表 / 路由表读失败会显示「无法读取…」，而不是继续显示上一次的旧数据 |
| **连续失败即不健康** | 连续 3 次读不回状态，判定内核无响应并给出原因 |
| **失败会留痕** | 失败原因保留在界面上直到用户重新连接，不会被重置成「未连接」 |
| **持久化写入** | 配置用「临时文件 + fsync + rename」写入，崩溃不会留下半截文件 |
| **退出不留残骸** | 退出应用时停止内核并等待；进程号记在 `core.pid`，下次启动会清理上次崩溃留下的孤儿（先核对进程名，不会误杀） |
| **日志脱敏** | 对已登记的网络密码和私钥按原文脱敏；为避免破坏普通日志，短于 4 字符的密钥不替换 |
| **默认只记错误** | 内核在 `info` 下非常啰嗦（空闲时约 226 行/20 秒，全部写盘 ≈35 MB/天）。默认级别 `error` 实测 **0 字节**；需要排查时可在设置里临时调高 |
| **重复行折叠** | 同一行反复出现只记一条并计数（`×12`），不再刷屏 |

## 开源与第三方许可

本项目自身代码采用 [MIT License](LICENSE)。EasyTier、Go 依赖、图标和
Windows 组件的许可证、来源与对应源码链接见
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)，完整文本位于
`third_party/`，并会随应用包分发。Npcap 不允许未经许可再分发，因此不在
本项目中打包。

漏洞报告、密钥存储、日志脱敏、无遥测声明和 RPC 安全边界见
[SECURITY.md](SECURITY.md)。

## 已知边界

- 提权是「重启整个应用为管理员」，和官方 GUI 的做法一致（官方 GUI 也是整个界面跑在 root 下）。
- 状态表通过 `easytier-cli -o json` 查询；原始计数使用 `-v peer`；原生事件/日志重载/远程生命周期使用有上限和超时的 EasyTier TCP RPC（不是 gRPC）。接口按 v2.6.4 验证，未来 Core schema 变动需要适配。
- 虚拟 IP 留空时使用 EasyTier 的 DHCP 模式，需要网络里已有节点能分配地址；固定 IP 更可靠。
- 自签名的 macOS/Windows 应用在提权时会有额外的系统提示。

## 待完成

以 [`docs/implementation-progress.md`](docs/implementation-progress.md) 为准。Android 原生 VPN 集成仍未实现；真实系统服务开机行为和 WireGuard 客户端握手仍需目标设备环境验证，不以配置/API 测试冒充。
