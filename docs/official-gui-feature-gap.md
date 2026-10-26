# 与 EasyTier 官方 GUI 的功能差距

## 对比口径

本文以 EasyTier 官方最新发布版 **v2.6.4** 为主要基准，并查看官方 `main` 分支补充开发版功能。

- 官方 v2.6.4：提交 `8428a89d2dabc94c97d370ec607c6ca142473626`
- 官方 main：提交 `54f2850d9d3cc42eb93e0d44d19bed5d9fbf2d0a`，仅用于核对开发版 WireGuard 设备管理等新功能
- 当前项目：以本仓库实际代码为准，不根据 EasyTier Core 的能力推断 GUI 已支持相应功能

官方 GUI 的网络配置和状态页面复用了 `easytier-web/frontend-lib`，因此对比范围同时包含 `easytier-gui` 和它实际挂载的共享组件。

## 总体结论

当前实现已经覆盖**单网络的基础组网、节点和路由查看、端口转发、日志及提权**。与官方 GUI 的主要差距是：

1. 多网络配置与多实例。
2. 后台驻留、系统服务和开机运行。
3. 高级网络配置。
4. ACL 与访问控制组。
5. 配置导入、导出和 TOML 编辑。
6. 远程 RPC 与 Config Server 集中管理。
7. WireGuard VPN Portal。
8. 状态监控与结构化事件。
9. 移动端和部分辅助功能。

底层都使用官方 Core，并不意味着 GUI 暴露的功能一致。当前配置模型只有 13 项，见 `internal/easytier/config.go:14-30`；实际生成 TOML 时写入的网络开关只有 `no_tun`、`enable_encryption`、`secure_mode` 和 `latency_first`，见 `internal/easytier/toml.go:112-116`。

## 当前已具备的功能

- 单网络连接、断开、失败状态显示和 Core 进程监督。
- 网络名称、网络密码、本机名称、固定虚拟 IP 或 DHCP。
- 初始对端、监听地址、代理网段、TCP/UDP 端口转发。
- 虚拟网卡、数据加密、安全模式、延迟优先。
- 概览、节点搜索和排序、完整路由表、日志查看。
- Linux、macOS、Windows 提权路径，以及内嵌 Core/CLI 的发布方式。

主要实现位置：

- 设置 UI：`internal/app/page_settings.go:8-95`
- 配置模型：`internal/easytier/config.go:13-44`
- TOML 生成：`internal/easytier/toml.go:67-117`
- Core 生命周期：`internal/easytier/controller.go`
- 节点和路由：`internal/app/page_peers.go`、`internal/app/page_routes.go`

## 详细功能差距

### 1. 多网络配置与多实例同时运行

官方 GUI 可以创建、切换、编辑、停用、删除多个网络，并同时运行多个网络实例。配置档案与实例使用 UUID 标识，停用实例会保留配置。

当前只有一个 `Config`、一个 `Controller` 和一个 Core 进程，没有配置档案切换或并行实例管理。

官方实现：[`RemoteManagement.vue`](https://github.com/EasyTier/EasyTier/blob/v2.6.4/easytier-web/frontend-lib/src/components/RemoteManagement.vue#L450-L602)

### 2. 后台驻留、系统服务与开机运行

官方 GUI 支持：

- 系统托盘；关闭窗口时隐藏窗口，普通模式下网络继续运行。
- 安装、启动、停止、卸载系统服务并查看服务状态。
- 服务模式开机启动和失败后重启。
- GUI 到管理 RPC 的连接断开后自动重连。

当前退出应用会主动停止 Core，见 `main.go:35-39`。没有托盘、服务模式、开机启动或 Core 崩溃后的自动重启。

官方实现：

- [运行模式与服务管理](https://github.com/EasyTier/EasyTier/blob/v2.6.4/easytier-gui/src/components/ModeSwitcher.vue#L165-L234)
- [关闭窗口后隐藏](https://github.com/EasyTier/EasyTier/blob/v2.6.4/easytier-gui/src-tauri/src/lib.rs#L1392-L1398)

这里的“自动重连”特指 GUI 与管理 RPC 的连接恢复，不应写成独立的底层 Peer 自动重连设置。

### 3. 高级网络配置

当前缺少官方设置页中的以下能力：

- **接口与性能**：TUN 设备名、MTU、实例接收限速、多线程、绑定物理网卡、`smoltcp`。
- **路由与出口**：手动路由、出口节点、转发网络白名单、映射监听器。
- **代理**：SOCKS5、系统代理转发。
- **P2P 与打洞**：禁用 P2P、仅使用 P2P、延迟建立 P2P、要求 P2P，以及 TCP、UDP、对称 NAT 打洞控制。
- **传输协议**：KCP、QUIC 代理及输入开关。
- **其他**：IPv6 控制、公网 IPv6 自动配置、UPnP 开关、Magic DNS、私有模式、Windows UDP 广播中继等。

官方实现：[`Config.vue`](https://github.com/EasyTier/EasyTier/blob/v2.6.4/easytier-web/frontend-lib/src/components/Config.vue#L80-L107)

### 4. ACL 与访问控制组

官方 GUI 已有完整的 ACL 编辑器，包括：

- Inbound、Outbound、Forward 三类链。
- 链和规则的启用状态、默认 Allow/Drop 动作。
- TCP、UDP、ICMP、ICMPv6、IP、端口及端口范围。
- 规则名称、描述、优先级和排序。
- Stateful、PPS 限速和 Burst。
- ACL 组声明、组成员以及源组和目的组规则。

当前没有 ACL 配置模型或 UI，`README.md:248` 已将其列为后续功能。

### 5. 配置导入、导出和 TOML 编辑

官方 GUI 支持单个网络的 TOML 导入、导出和文本编辑。当前只能读取和保存自己的 `config.json`，没有配置迁移、备份、分享或原始配置入口。

官方 TOML 编辑器经过其网络配置模型投影，并不等于任意 Core TOML 的无损编辑器；后续实现时也不应笼统承诺支持所有 Core 字段。

### 6. 远程 RPC 与 Config Server 集中管理

官方 GUI 提供三种运行模式：

- 普通本地模式。
- 系统服务模式。
- 连接另一台机器的远程 RPC 模式。

普通模式还可开启 RPC 监听；普通模式和服务模式能够接入 Config Server，由 Web 控制台下发配置并显示连接状态。

当前只能管理本应用启动的独立 Core，通过 `easytier-cli -o json` 轮询本机状态，见 `internal/easytier/engine.go:105-130`。

### 7. WireGuard VPN Portal

官方 v2.6.4 已支持：

- 启用 VPN Portal。
- 配置客户端网段和监听端口。
- 查看 WireGuard 客户端配置。

官方开发版 `main` 进一步支持：

- 添加、删除 WireGuard 设备。
- 查看设备在线状态。
- 生成二维码。
- 复制、下载客户端配置。
- 给设备分配 ACL 组。

当前没有 VPN Portal 或 WireGuard 设备管理功能。

开发版实现：[`VpnPortalDialog.vue`](https://github.com/EasyTier/EasyTier/blob/54f2850d9d3cc42eb93e0d44d19bed5d9fbf2d0a/easytier-web/frontend-lib/src/components/VpnPortalDialog.vue)

### 8. 状态监控与结构化事件

官方状态页额外提供：

- 实时上传、下载速率曲线。
- 对端丢包率和 NAT 类型。
- 公共服务器、避免中继等标签。
- 本机接口 IPv4/IPv6、公共 IPv6、TUN 名称等详情。
- TUN、节点、监听器、DHCP、端口转发等结构化事件时间线。

当前已有延迟、累计流量和基础设备信息，但没有速率图表和结构化网络事件。对端丢包率和 NAT 已经解析，却未进入节点页，见 `internal/easytier/views.go:14-28`、`internal/app/page_peers.go:39-65`。

官方实现：[`Status.vue`](https://github.com/EasyTier/EasyTier/blob/v2.6.4/easytier-web/frontend-lib/src/components/Status.vue#L382-L464)

### 9. 平台与辅助功能

- 官方提供 Android 客户端及系统 VPN 服务集成；当前仅覆盖桌面端。
- 官方支持中英文切换；当前界面只有中文。
- 官方文件日志支持 `off`、`warn`、`info`、`debug`、`trace`，并可运行时调整；当前只有 `error`、`warn`、`info`，且 Core 日志级别在下次启动时生效。
- 官方可打开或复制日志目录；当前关于页只提供日志目录信息，但拥有官方没有的内置日志内容查看器。

## 不应误算成差距的项目

以下功能官方 GUI 同样没有，或者当前实现反而更完整：

- **在线应用或内核更新**：官方没有在线更新器。服务模式更新是使用当前 GUI 内置版本重新安装服务，不是在线下载 Core。
- **每连接质量曲线**：官方没有对应页面。
- **托盘中的网络快速启停**：官方托盘也只有显示/隐藏和退出。
- **完整路由表**：当前有独立路由页，官方 GUI 没有同等完整的 CIDR、下一跳路由视图。
- **日志内容查看器**：当前可以直接查看 Core 日志；官方主要提供结构化事件查看和日志目录操作。

## 建议补齐顺序

1. 多网络配置与多实例。
2. 后台驻留、托盘和系统服务。
3. 配置导入、导出与 TOML 编辑。
4. 高级网络设置。
5. ACL 与访问控制组。
6. 状态图表和结构化事件。
7. 远程 RPC 与 Config Server。
8. WireGuard VPN Portal。
9. Android 和其他辅助功能。

这个顺序优先考虑日常使用范围和部署能力，不代表各项实现难度。
