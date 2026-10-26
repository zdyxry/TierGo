# TierGo

[![CI](https://github.com/zdyxry/TierGo/actions/workflows/ci.yml/badge.svg)](https://github.com/zdyxry/TierGo/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

TierGo 是一个**非官方** [EasyTier](https://github.com/EasyTier/EasyTier) 桌面客户端，使用 Go 和 [MyGo](https://github.com/egoist/mygo) 编写，支持 Linux、macOS 和 Windows。

应用内嵌官方 `easytier-core` 和 `easytier-cli`。每个本地网络使用独立进程、配置、日志和端口，不影响已有的 EasyTier 服务。

## 功能

- 多网络同时连接，支持托盘驻留和快速启停
- 查看节点、路由、流量、网络事件和连接状态
- 编辑、导入和导出 TOML，支持高级网络选项与 ACL
- 管理远端实例、Config Server 和 VPN Portal
- 支持系统服务、登录启动及中英文界面

## 安装

从 [Releases](https://github.com/zdyxry/TierGo/releases) 下载对应平台的安装包。发布版已内嵌 Core，无需单独安装 EasyTier。

创建虚拟网卡需要管理员权限。Windows 的 Npcap 不随应用分发，需要相关功能时请从 <https://npcap.com> 自行安装。

## 从源码构建

需要 Go 1.27.2。以下命令下载锁定的 Core、校验 SHA-256，并构建当前平台安装包：

```sh
go run ./cmd/fetch-core
go tool mygo build
```

产物位于 `build/`。macOS 通用包需要先下载两种架构的 Core：

```sh
go run ./cmd/fetch-core -platform darwin/amd64
go run ./cmd/fetch-core -platform darwin/arm64
go tool mygo build -platform darwin/universal
```

也可以不内嵌 Core，通过 `EASYTIER_CORE` 和 `EASYTIER_CLI` 指定已有二进制。

## 测试

```sh
go test -short ./...       # 单元测试
go test ./...              # 完整测试；有官方二进制时运行集成测试
go vet ./...
go tool mygo vet .
```

CI 会下载锁定的测试二进制，运行完整集成测试和 Race 检查，并构建各平台安装包。

## 使用边界

- Linux 配置目录为 `~/.config/easytier-mygo/`，其他系统使用系统配置目录。网络密码和私钥文件应妥善保护。
- 远端 RPC 没有认证，只能通过可信网络或隧道使用。
- Android 尚未支持。系统服务、提权、真实 TUN 和 WireGuard 客户端连通性仍需在目标设备上验证。

详细实现进度和验证范围见 [implementation-progress.md](docs/implementation-progress.md)。

## 许可与安全

项目代码采用 [MIT License](LICENSE)。第三方组件的来源和许可证见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

安全问题及数据存储说明见 [SECURITY.md](SECURITY.md)。
