# 功能补齐实施记录

基准：`official-gui-feature-gap.md` 的 v2.6.4；本地官方源代码位于 `../EasyTier`，main 提交 `54f2850d`。差距文档保留为实施前的历史对比，不将它误当作当前状态。

## 进度

- [x] 1. 多网络配置与多实例（91505a0）
- [x] 2. 后台驻留、服务管理与登录启动（107f269）
- [x] 3. TOML 导入、导出和编辑（32877c8）
- [x] 4. 高级网络设置
- [x] 5. ACL 与访问控制组
- [x] 6. 状态监控和结构化事件（原生事件通过官方同款 CollectNetworkInfo 轮询）
- [x] 7. 远程 RPC 与 Config Server（完整配置编辑、停用保留配置、普通模式/服务模式配置服务；真实下发与恢复已验证）
- [x] 8. WireGuard VPN Portal（v2.6.4 共享配置与 v2.7/main 逐设备模式明确区分，设备管理/组/状态/二维码已实现）
- [ ] 9. Android 与其他辅助功能（日志级别/运行时调整/日志目录操作/中英文已实现；Android 未完成，且不在本次继续范围内）

“已完成”指提交中对应功能的实施，并不表示未经实际系统测试的跨平台行为均验证成功。后续审核中发现的问题继续修复。

## 第二轮：复核差距补齐

复核结论见 `official-gui-feature-gap.md` 更新前的核对记录；按优先级逐项补齐，每项独立提交。

- [x] 1. 保存并恢复启用网络与上次选中网络：app.json 新增 `selected_profile`/`enabled_networks`；连接、断开、删除网络、服务接管时维护；启动时 `RestoreSession` 重连，系统服务运行时跳过；恢复期间生成的逐设备 Portal 私钥随档案落盘。单测覆盖选中恢复、启用集合维护、删除遗忘、服务接管；e2e 用真实 Core 验证 `Manager.Restore`。
- [x] 2. 统一本地/服务/远端实例上下文：`remoteActive` 统一当前实例，侧边栏在本地网络下列出远端分组（状态点/停用标记），点击远端实例或本地网络即切换上下文；view 层在远端上下文激活时于任意页面驱动 3 秒轮询，概览/节点/路由/监控直接复用远端快照。`ReadRemoteStatus` 增加 `-i <id> -v peer` 逐连接原始计数并合并公共服务器/避免中继标记；计数差值逻辑抽为 `DiffTraffic`/`AppendTrafficSample`（120 采样上限），App 侧在轮询结果到达时对同一远端实例差分维护速率历史，切换实例/地址、删除、停用/启用时重置基线；事件时间线沿用已采集的 Core 事件。顶栏在远端上下文显示「远端实例」徽章，连接/断开按钮换成启用/停用远端网络（复用 remoteStateConfirm 确认流程与只读禁用）。监控页提供远端运行时日志级别（LoggerRpc 立即生效，界面标注进程级、重启恢复）。新增 `RemoteConfig.ACL()/WithACL()`：用 dynamicpb + protojson 数字枚举在 NetworkConfig 的 `acl` 字段（56）上读写，未知 wire 字段与其他 section 不动，nil 表示清除，写入前先 `ACL.Validate`；ACL 编辑器复用到远端（读取按钮、保存前确认、保存时重新 GetRemoteConfig 再合并，只读权限不提供入口）。Portal RPC 补 `InstanceIdentifier.id` 实例寻址：`ReadRemoteVPNPortal`（VpnPortalRpc.GetVpnPortalInfo）与 `PatchRemotePortalDevice`（ConfigRpc.PatchConfig 的 instance 字段，请求布局核对自官方 main 的 api_config.proto），远端 Portal 页复用设备列表/增删/配置/二维码，能力探测失败保持原有明确报错，不发明绕过 Core 的路径。设置/日志页在远端上下文显示指引（分别指向远程管理页完整配置编辑与监控页 Core 事件）。远程页「查看系统服务网络」「查看普通模式下发网络」改为直接连接并激活远端上下文、切到概览页。测试：ACL 映射往返/未知字段保留/清除/非法拒绝单测，计数差分与采样上限单测；App 侧远端分组出现与切换、跨轮询速率采样、ACL 读取-编辑-确认-保存（配置读写走 stub 接缝）、Portal 远端渲染、顶栏停用确认与只读禁用、local-only 提示；真实 Core e2e 覆盖远端计数采集、实例寻址 Portal 查询与远端 ACL 保存读回（且不影响无关实例）。
- [x] 3. TOML 往返完整性（安全密钥、DHCP、空手动路由、SOCKS5 绑定）：Config 新增 `secure_mode_private_key`/`secure_mode_public_key`（`EnsureSecureModeKeys` 在保存设置、连接、TOML 应用时生成一次并随档案落盘，导出优先复用存储密钥，节点身份不再每次导出都变）、`dhcp *bool`（nil 保持旧的 IPv4 空 && EnableTUN 推断，非 nil 显式导出 true/false；ipv4 优先于 dhcp）、`manual_routes_on`（表达 `routes = []`，导入用 `*[]string` 区分缺失与空列表）、`socks5_host`（空即 0.0.0.0 旧行为，导入接受任意主机含 IPv6，导出走 net.JoinHostPort）。导入对无效安全模式密钥（非 base64 或非 32 字节）报错而非静默替换；Validate 校验密钥成对且公私匹配、SOCKS5 绑定地址为 IP 或主机名。UI：虚拟 IP 旁 DHCP 开关（IPv4 非空禁用，拨动写显式值）、手动路由开关（列表非空强制开并锁定）、SOCKS5 绑定地址输入（端口为 0 禁用）；toggleRow 整行可点击。测试：四场景往返/旧推断回归/Ensure 幂等/Validate 拒绝单测，DHCP 开关与禁用、路由开关锁定、绑定地址输入 UI 测试，真实 Core e2e 验证 SecureMode 配置到 Running 且两次渲染密钥一致。
- [x] 4. ACL 链管理与组引用联动：`ACL.Validate` 不再拒绝同类型第二条链（保留类型 1-3 与规则校验，错误文案改为「ACL 链类型无效」）；新增 `NewChain`（按类型取默认名、重名自动加序号、Enabled+Allow 默认）、`RenameGroup`（改 Declares 的同时精确替换所有链所有规则的 SourceGroups/DestinationGroups，不误伤子串，空名/重名/组不存在均报错）、`SortChainRules`（优先级降序稳定排序）与 `ACL.Sorted`（返回规则按匹配顺序排好的副本）；`renderConfigTOML` 与 `RemoteConfig.WithACL` 序列化前统一走 `Sorted`，Core 拿到的规则顺序即匹配顺序。UI：链卡片支持删除单条链、三种类型一键添加新链（同类型多链靠可编辑链名区分）；组名改为草稿输入 + 「重命名」按钮一次性提交并联动全部规则引用，失败经 notice 提示；规则保存与上移后即时重排，显示顺序==存储顺序==Core 匹配顺序。测试：同类型多链 Validate/越界拒绝、RenameGroup 跨链替换与失败路径、排序稳定性与 Sorted 不改原对象、NewChain 命名、TOML 导出有序、UI 添加/删除链与重命名联动与手工改优先级重排；真实 Core e2e 验证同类型双链 ACL 起到 PhaseRunning。
- [x] 5. Config Server 状态与服务配置选项
  - 官方通道调查结论（v2.6.4 与 main 一致）：普通模式的连接状态来自 GUI 进程内嵌 WebClient 的 `is_web_client_connected`（进程内 AtomicBool，隧道连上即 true、重试/断开即 false，本质是传输级信号，每秒轮询）；**服务模式官方直接显示 unknown**——proto/rpc_service 没有任何暴露 config-server 状态的 RPC 方法，官方也不读服务日志。服务模式安装参数官方为 `--config-dir/--rpc-portal/--file-log-level/--file-log-dir/--daemon`。
  - `ServiceSettings`（ConfigDir/LogDir/FileLogLevel/RPCAddress，全空=原自动行为）持久化进 app.json；`ServiceInstall`/`ServiceInstallCentral` 使用：ConfigDir 覆盖默认目录（含 central-configs），RPCAddress 校验 host:port（含端口数值范围）后作为 `--rpc-portal`（service-rpc 文件仍写实例根目录，读取不受影响），LogDir/FileLogLevel 非空时传 `--file-log-dir`/`--file-log-level`（级别复用 LogLevels 校验）。平台差异仍由 easytier-cli service 承担，本层无平台分支。
  - Core 版本漂移：安装/更新时把当前内嵌 Core 版本写入服务配置目录的 `core-version` 标记；服务页刷新状态时用 `ServiceCoreDrift`（复用 LocateBinaries 返回的内嵌版本，无需新增 corebin 接口）对比，不一致显示「内嵌 Core 已从 X 变为 Y，点『安装或更新』同步服务配置」横幅，重装后标记刷新、横幅消失；不自动重装（安装需提权）。
  - Config Server 状态：普通模式保留传输推断，补「最近传输状态依据（时间+脱敏日志行）」（CentralSnapshot 新增 TransportAt/TransportLine，状态翻转时记录）；行分类器 `classifyConnectionLine` 同时匹配 v2.6.4（Successfully connected / Failed to connect to the server, retrying / heartbeat failed）与 main（connected to config server）两种措辞。服务模式实现日志推断：`ScanServiceConnectionLog` 扫描服务文件日志（LogDir 或 Core 默认位置=服务工作目录）尾部 256KiB，显示「服务日志最近报告：已连接/未连接（推断）+ 时间+来源行」，与进程状态分区展示；无文件日志时明确显示「服务模式无连接状态通道」。Core 文件日志默认 OFF（v2.6.4 与 main 均如此），故需在服务页设置文件日志级别并重新安装后才有此通道。
  - 测试：ServiceSettings 校验/持久化往返、安装参数开/关矩阵（默认不传日志参数、自定义目录/RPC/日志全传）、版本标记写入与漂移判定、日志尾部解析（含假日志、尾部限量、时间戳解析）；UI 测试覆盖服务页控件渲染/保存/校验拒绝、漂移横幅出现/消失、服务模式状态区（已连接推断与无通道两种）、普通模式依据行；e2e 用真实 v2.6.4 Core 以 Config Server 模式连不存在的服务器，验证从真实 easytier.log 尾部推断出未连接。
- [x] 6. main 高级选项与 WireGuard 操作辅助
  - 官方 main 核实：TOML 键为顶层 `ipv6_public_addr_provider`（bool）与 `[flags]` 的 `disable_relay_data`/`prefer_peer_relay`（bool）/`socket_mark`（optional u32，Linux SO_MARK，0 是合法值故用指针）；CLI 对应 `--ipv6-public-addr-provider`/`--disable-relay-data`/`--prefer-peer-relay`/`--socket-mark`。**官方 main 的 GUI（Tauri 桌面与 web 前端）均未暴露这四项，也没有凭据管理界面**（web 前端只有弃用的语言条目与加入方粘贴凭据的入口；管理仅 easytier-cli `credential generate/revoke/list`）。
  - v2.6.4 对未知 flag 的真实行为（真实内嵌 2.6.4-8428a89d 验证）：带全部四个新键的配置**正常启动、未知键被忽略**，不拒绝启动；故新字段直接进渲染 TOML，不按 Core 版本跳过。e2e 以对照实例 + 新字段实例双双起真实 Core 固化该结论。
  - Config 新增 `disable_relay_data`/`prefer_peer_relay`/`ipv6_public_provider`/`socket_mark *uint32`，TOML 往返（socket_mark 指针区分未设置与 0）；高级设置页：接口与性能加 SO_MARK 开关+数值，路由与出口加「不转发数据」「优先凭证节点转发」（沿用官方 web 中文文案），IPv6 与其他加「提供公网 IPv6 地址」。
  - 临时凭据：官方 GUI 无入口，故只做 RPC 层 + 简单入口——`CredentialManageRpc`（Generate=1/Revoke=2/List=3，按 main 的 api_instance.proto 手编码，含 InstanceIdentifier 实例寻址）本地/远端双通道；Portal 页加「临时凭据」卡片（有效期/ACL 组/允许中继/可复用 → 生成并一次性展示密钥，列表含过期状态，吊销有确认），对 v2.6.4 调用失败如实显示错误。未接 UpsertCredential（本地 schema 与官方 GUI 均无）。
  - WireGuard 辅助（本地与远端实例同效，复用 portalContextKey）：Portal 页打开且设备模式已连接时每 3 秒自动重读设备列表（c.After + 可 stub 的 portalPoll 接缝，客户端有变化才落盘档案）；「使用建议地址」按钮按 ClientCIDR（远端/设备模式从既有设备地址推断网段）给出首个未占用主机地址（排除网段/广播，/31、/32 全可用），只填占位符与按钮、不覆盖手填；已声明 ACL 组时设备组改为勾选 + 手填补充，未声明时维持手填并提示；「使用建议」按钮按当前实例 listeners/mapped_listeners（远端用 Node.Listeners）过滤 0.0.0.0/loopback 后组合 Portal 端口推导候选 Endpoint。
  - 测试：新字段 TOML 往返（含 socket_mark 0/未设置）、SuggestDeviceIP 各网段边界与占用、mergeDeviceGroups 多选合并、SuggestPortalEndpoints 过滤与去重、凭据请求编码/列表解码单测；UI 测试覆盖高级页新控件与持久化、建议 IP/Endpoint 按钮、组勾选、定时刷新接缝触发与档案落盘、凭据生成/列表/吊销/错误展示；e2e 见上。

## 第 9 项：辅助功能与移动端调研

- 日志增加 off/debug/trace，通过官方 `LoggerRpc.SetLoggerConfig` 实时应用，真实 Core GetLoggerConfig 逐级读回验证。关于页可打开/复制日志目录。
- MyGo v0.6.2 README 定位为桌面框架。上游 [PR #157](https://github.com/egoist/mygo/pull/157) 已关闭，维护者明确说不计划维护 iOS/Android；评论提及独立 Android fork，但这不是当前依赖的后端。不能把 GOOS=android 编译或桌面提权逻辑声称为 VpnService 集成。
- 已补齐中英文：internal/locale/en.json 资源化 500 条界面/校验/操作消息（中文原文作为稳定消息键）；全界面和错误格式使用 locale.T，保留 fmt 的格式参数和 %w 语义；应用设置页即时切换简体中文/English，保存 app.json；MyGo App.SetLanguages/SetLocale 同步控件语言，托盘重建标签。用户网络名称、密码、Core 原始事件和第三方错误不翻译。
- 测试逐一渲染 11 个页面的英文内容，包括展开高级设置和 ACL；实际点击切回中文，检查导航即时更新、语言落盘、网络数据不变；源码扫描校验所有中文消息均有英文资源，校验格式占位符不变。atomic 语言状态保证后台日志/控制器并发读取安全。
- 未完成：Android 宿主与 VpnService/生命周期/权限/FD 交接及真机集成测试。当前不更换框架或自动引入评论中的未审核 fork；用户本次指定继续的 1–4 项不含 Android。

## 第 8 项：稳定版 VPN Portal

- 核对 v2.6.4 `VpnPortalConfig`（`client_cidr`、`wireguard_listen`）与 WireGuard 配置生成器；main 已移除共享 client_cidr，改为逐设备 clients，因此不混用接口。
- 新页面启用/关闭 Portal、客户端网段与监听设置、读取 Core 客户端配置及在线客户端、复制/下载配置，支持替换公网 Endpoint（Core 默认模板使用监听地址，不一定可远程访问）。明确提示配置含私钥。
- 单测验证模型往返、地址检查与 Endpoint 替换；UI 保存测试；真实 Core 在无 TUN 模式开启 loopback Portal 并获取 WireGuard 私钥配置集成测试。
- 已补齐 main：明确选择逐设备 schema，启动时核对 Core 版本；专用 X25519 Portal 私钥在本地保存，设备名称/IP/组校验；直接调用 ConfigRpc PatchConfig 的 vpn_portal_clients 字段，先用 listener 能力探测拒绝旧 Core；设备增删、状态、组分配、逐设备配置、复制下载与本地生成二维码（私钥不外传）。运行时设备修改同步本地档案，重启不丢失。
- 实测官方 nightly `2.7.0-54f2850d`：开启现代 Portal、声明 ACL 组、添加设备、读回 IP/组/配置、二维码生成、删除设备。环境变量：`EASYTIER_NEXT_CORE`/`EASYTIER_NEXT_CLI`；v2.6.4 原集成仍同时运行。UI 测试覆盖在线状态、二维码和删除确认。
- 未将配置导出测试说成真实 WireGuard 客户端握手测试；在线状态来自 Core，不以生成配置冒充在线。

## 第 7 项：调研与首个独立子项

- 核对 `WebClientService`：先 method 5 列出 ID，再 method 8 取得名称/权限（空 meta 请求不是列出全部）。CLI 的 `-i` 选择实例，不能把多实例响应当单实例解析。
- 已实现独立远程查看页：TCP RPC 地址检查、网络枚举/选择、节点和路由/主机状态、错误后下次刷新重试、断开仅取消查看，不停止远端；明确提示 RPC 无认证，应走可信隧道。
- 真实 Core 集成验证网络枚举、UUID 转换、实例选择及无副作用；UI 验证权限/安全说明。
- 后续子项：Config Server 系统服务安装（地址校验、持久化稳定 UUID Machine ID、单独 central-configs 目录）；服务 RPC 地址落盘，可由远程查看页接入服务网络；Core 拥有心跳/重连而非 GUI 模拟。
- 校验：服务命令参数桩测试、真实 Core 的参数/配置校验；未搭建 Config Server 进行下发/断网恢复端到端测试，界面不把服务运行误称为已连接服务器。
- 修复既有测试工具的隔离问题：辅助函数中的 defer 过早恢复配置目录，改为 t.Cleanup，防止测试向用户目录写入。
- 再后续子项：确认后创建基础远端网络、按 ID 删除网络（Core 校验 no-delete 权限）；基础投影拒绝任何未支持的高级字段，防止静默丢失。RPC 请求支持分片，真实 Core 验证大请求创建/枚举/删除及不影响原实例。
- 已补齐远端完整编辑与停用/启用：内嵌官方 v2.6.4 FileDescriptorSet，以 dynamicpb/protojson 编辑完整 NetworkConfig，保留未知 wire 字段和 source；应用前调用远端 ValidateConfig。停用按官方 GUI 语义，先本机 0600 保存完整配置、再删除运行实例；地址哈希绑定备份，启用成功后移除备份。远端只读/no-delete 权限在 UI、客户端及 Core 三层检查。
- 单测覆盖未知字段保留、不可修改 ID、未知 JSON 拒绝；真实 Core 集成覆盖高级修改→读回→停用→地址隔离→启用→删除，UI 覆盖取消编辑与本地草稿隔离。
- 已补齐普通模式 Config Server：独立受监督 Core，独立 central/central-configs 目录与 PID，普通模式关闭随 GUI 退出，系统服务不受 GUI 退出影响；稳定 Machine ID 持久化，界面可查看下发网络。Core 管理重连，不在 GUI 重写协议。生命周期日志只保留有界内存、令牌脱敏，传输状态不冒充认证心跳。
- 新增真实官方 easytier-web v2.6.4 集成：使用本地 SQLite 与 TCP 配置服务，认证心跳证据来自服务器 sessions API；通过 REST 下发网络，断开/重启服务器，验证原数据面存活、同 Machine ID 重连；重启客户端 Core 后恢复同一网络 ID。运行：`EASYTIER_WEB=/path/to/easytier-web go test ./...`。无二进制时显式跳过该集成测试，不用 mock 冒充。
- 第 7 项主要差距已补齐，仍需目标系统的真实服务安装/开机测试（不在本机容器中冒充验证）。

## 第 6 项：已交付子项与尚存差距

- 核对官方 v2.6.4 CLI `handle_peer_list`：`-o json peer` 是格式化表格，`-v peer` 才有逐连接原始 uint64 字节计数；按连接 ID 求差计算速率，连接重建/计数回退/读取失败均重置基线，保留 120 次样本。
- 监控页绘制上传/下载曲线，标明物理对端流量（含开销）而非应用吞吐。节点页加入丢包、NAT、公共服务器和避免中继标签；设备详情解析原始 IPv4/IPv6 接口地址。
- 后续补齐原生事件：最小 EasyTier TCP RPC 适配调用 `WebClientService.CollectNetworkInfo`（method 4），读取 Core 的序列化事件，不解析普通日志。官方本身也是轮询最近 20 条事件；界面说明潜在遗漏，保留来源与事件时间。旧 Core 不支持时才退回轮询差分。
- RPC 核对官方 `common.proto`、`rpc_impl/packet.rs`、`tunnel/packet_def.rs`；每次连接一个调用，明确协商无压缩，分片数量/帧长度上限、超时与取消。真实 Core 验证 ListenerAdded 原生事件，TCP 测试验证分片合并；此适配不是重写数据面。
- 验证：计数回退/新连接/缺样本/缓冲上限单测、真实 Core verbose CLI 查询、接口录制样本解析、监控页 UI 测试，以及全套测试和 race。

## 第 5 项：调研与验证

- 核对官方 v2.6.4 `acl.proto`、`config.rs` 的 TOML 用例及 `acl_processor.rs` 的匹配排序：三种链，Allow/Drop，TCP/UDP/ICMP/ICMPv6/Any，数字枚举；优先级降序。
- 模型覆盖规则元数据、IP/CIDR、双向端口/范围、Stateful、PPS/Burst、源/目的组、组声明密钥及本机成员。
- 独立访问控制页：链启用/默认动作、规则增删编辑/上移、组管理、显式保存；移除全部 ACL 需要确认。运行中禁止编辑。
- 校验与 TOML 往返单测、UI 新建规则/保存/无效端口测试；真实 Core 启动后通过 RPC 读回完整 ACL，与输入逐字段比较。草稿深拷贝防止未保存修改影响服务安装。
- 尚未在真实多设备网络上验证每种 ACL 规则的包过滤结果；当前集成测试证明 Core 加载模型，不将其夸大为全协议流量测试。

## 第 4 项：调研与验证

- 核对官方 v2.6.4 `easytier/src/common/config.rs` 的 Config、默认 Flags 和 `core.rs` 的命令行映射，而不只依赖当前 main（main 有 schema 重构）。
- 顶层字段：`routes`、`exit_nodes`、`mapped_listeners`、`socks5_proxy`、`ipv6`、`ipv6_public_addr_auto`。
- 代理网段是 `[[proxy_network]]` 的 `cidr`/`mapped_cidr`；此前客户端使用的 `proxy_networks` 会被 Core 静默忽略，已修正，导入仍兼容旧客户端文件。
- v2.6.4 压缩枚举为整数 `data_compress_algo = 2`（zstd）；字符串会使旧 Core panic。
- 安全模式为 `[secure_mode]`，不是 `[flags].secure_mode`；使用 Go 标准库生成配套的 X25519 密钥。
- 新增设置覆盖接口、MTU、接收限速、多线程、物理接口绑定、smoltcp、出口、白名单、映射监听器、SOCKS5、系统转发、P2P/打洞、KCP/QUIC、压缩、协议、加密算法、IPv6、UPnP、Magic DNS、私有模式、Windows UDP 广播中继。
- 校验/序列化往返单测；UI 编辑到持久化测试；真实 Core 的 `--check-config` 和 RPC 读回归一化配置集成测试。测试不修改宿主 DNS，不启用系统出口。
- 验证：`go test ./...`、`go vet ./...`、`go test -race ./internal/easytier ./internal/app -short`。
