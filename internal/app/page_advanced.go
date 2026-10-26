package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Advanced fields edit the same draft as the basic form and TOML editor.
// Collapsing them keeps the common settings short without hiding capabilities.
func (a *App) advancedSettings(c *ui.Context, t *ui.Theme, config *easytier.Config, disabled bool) {
	draft := a.draft()
	ui.Collapsible(c, locale.T("高级网络设置"), &a.advancedOpen, func() {
		card(c, func() {
			ui.Text(c, locale.T("接口与性能")).Bold()
			ui.Form(c, func() {
				ui.Field(c, locale.T("TUN 设备名"), func() { ui.TextInput(c, &config.DevName).Disabled(disabled) })
				ui.Field(c, locale.T("MTU（0 为默认）"), func() { integerInput(c, &config.MTU, 65000, disabled) })
				ui.Field(c, locale.T("接收限速（字节/秒，0 不限）"), func() {
					value := float64(config.RecvBpsLimit)
					ui.NumberInput(c, &value, 0, 1<<50, 1024).Disabled(disabled)
					config.RecvBpsLimit = uint64(value)
				})
			})
			toggleRow(c, t, locale.T("多线程"), locale.T("关闭后使用单线程运行时"), &config.MultiThread, disabled)
			toggleRow(c, t, locale.T("绑定物理网卡"), locale.T("连接套接字绑定物理设备，避免路由冲突"), &config.BindDevice, disabled)
			toggleRow(c, t, "smoltcp", locale.T("使用用户态协议栈处理子网代理"), &config.Smoltcp, disabled)
			// Linux SO_MARK on outbound underlay sockets; 0 is a real mark,
			// so a switch decides set/unset separately from the value.
			markOn := config.SocketMark != nil
			beforeMark := markOn
			toggleRow(c, t, locale.T("出站套接字标记"), locale.T("为所有出站底层套接字设置 SO_MARK（仅 Linux，需配合策略路由）"), &markOn, disabled)
			if markOn != beforeMark {
				if markOn {
					mark := uint32(0)
					config.SocketMark = &mark
				} else {
					config.SocketMark = nil
				}
			}
			if config.SocketMark != nil {
				ui.Form(c, func() {
					ui.Field(c, locale.T("SO_MARK 值"), func() {
						value := float64(*config.SocketMark)
						ui.NumberInput(c, &value, 0, 1<<32-1, 1).Disabled(disabled)
						*config.SocketMark = uint32(value)
					})
				})
			}
		})
		card(c, func() {
			ui.Text(c, locale.T("路由与出口")).Bold()
			ui.Form(c, func() {
				ui.Field(c, locale.T("手动路由"), func() {
					stringListInput(c, &draft.manualRoutesText, locale.T("192.168.0.0/16（每行一条；覆盖自动学习的路由）"), disabled)
				})
				ui.Field(c, locale.T("出口节点"), func() {
					stringListInput(c, &draft.exitNodesText, locale.T("10.144.0.8（按优先级每行一个虚拟 IP）"), disabled)
				})
				ui.Field(c, locale.T("转发网络白名单"), func() {
					ui.TextInput(c, &config.RelayWhitelist).Placeholder(locale.T("* 允许所有；- 不转发；net1 net2 按名称允许")).Disabled(disabled)
				})
				ui.Field(c, locale.T("映射监听器"), func() {
					stringListInput(c, &draft.mappedListenersText, locale.T("tcp://公网地址:端口（每行一个）"), disabled)
				})
			})
			// A non-empty list is manual routing by itself; the switch only
			// decides what an empty list means (routes = [], not absent).
			hasRoutes := len(easytier.SplitLines(draft.manualRoutesText)) > 0
			routesOn := config.ManualRoutesOn || hasRoutes
			beforeRoutes := routesOn
			routesHint := locale.T("列表为空时开启会导出 routes = []，停用自动学习的路由")
			if hasRoutes {
				routesHint = locale.T("列表非空时始终启用手动路由")
			}
			toggleRow(c, t, locale.T("启用手动路由"), routesHint, &routesOn, disabled || hasRoutes)
			if routesOn != beforeRoutes {
				config.ManualRoutesOn = routesOn
			}
			toggleRow(c, t, locale.T("允许作为出口节点"), locale.T("允许其他节点通过本机访问外部网络"), &config.EnableExitNode, disabled)
			toggleRow(c, t, locale.T("不转发数据"), locale.T("不转发其他节点的数据包，本节点不承载自身以外的流量"), &config.DisableRelayData, disabled)
			toggleRow(c, t, locale.T("优先凭证节点转发"), locale.T("优先通过已经转发到目标节点的凭证节点转发，而不是为同一目标再通告一条直连路径"), &config.PreferPeerRelay, disabled)
		})
		card(c, func() {
			ui.Text(c, locale.T("代理")).Bold()
			ui.Form(c, func() {
				ui.Field(c, locale.T("SOCKS5 端口"), func() { integerInput(c, &config.Socks5Port, 65535, disabled) })
				ui.Field(c, locale.T("SOCKS5 绑定地址"), func() {
					ui.TextInput(c, &config.Socks5Host).Placeholder("0.0.0.0").
						Disabled(disabled || config.Socks5Port <= 0)
				})
			})
			ui.Text(c, locale.T("SOCKS5 无认证，默认监听所有接口；仅在可信网络启用。端口 0 为关闭。")).FontSize(12).TextColor(t.Warning)
			toggleRow(c, t, locale.T("系统代理转发"), locale.T("通过系统内核转发子网流量，而不是内置 NAT"), &config.ProxyForwardBySystem, disabled)
		})
		card(c, func() {
			ui.Text(c, locale.T("P2P 与打洞")).Bold()
			for _, item := range []struct {
				label, hint string
				value       *bool
			}{
				{locale.T("禁用 P2P"), locale.T("禁用普通自动 P2P，仍会连接要求 P2P 的节点"), &config.DisableP2P},
				{locale.T("仅使用 P2P"), locale.T("只与已经建立直连的节点通信"), &config.P2POnly},
				{locale.T("延迟建立 P2P"), locale.T("有流量时才建立直连"), &config.LazyP2P},
				{locale.T("要求 P2P"), locale.T("要求其他节点主动向本机建立直连"), &config.NeedP2P},
				{locale.T("转发所有节点 RPC"), locale.T("忽略转发白名单转发管理数据，帮助其他节点打洞"), &config.RelayAllPeerRPC},
				{locale.T("禁用 TCP 打洞"), locale.T("不通过 TCP 尝试穿透 NAT"), &config.DisableTCPHolePunching},
				{locale.T("禁用 UDP 打洞"), locale.T("不通过 UDP 尝试穿透 NAT"), &config.DisableUDPHolePunching},
				{locale.T("禁用对称 NAT 打洞"), locale.T("避免高频探测被运营商拦截"), &config.DisableSymHolePunching},
			} {
				toggleRow(c, t, item.label, item.hint, item.value, disabled)
			}
		})
		card(c, func() {
			ui.Text(c, locale.T("传输协议")).Bold()
			toggleRow(c, t, locale.T("KCP 代理"), locale.T("使用 KCP 代理 TCP 流"), &config.KCPProxy, disabled)
			toggleRow(c, t, locale.T("禁用 KCP 输入"), locale.T("不接受对端的 KCP 代理流"), &config.DisableKCPInput, disabled)
			toggleRow(c, t, locale.T("QUIC 代理"), locale.T("使用 QUIC 代理 TCP 流"), &config.QUICProxy, disabled)
			toggleRow(c, t, locale.T("禁用 QUIC 输入"), locale.T("不接受对端的 QUIC 代理流"), &config.DisableQUICInput, disabled)
			ui.Form(c, func() {
				ui.Field(c, locale.T("默认协议"), func() {
					ui.TextInput(c, &config.DefaultProtocol).Placeholder("tcp / udp / ws / wss / wg / quic").Disabled(disabled)
				})
				ui.Field(c, locale.T("压缩算法"), func() {
					ui.TextInput(c, &config.Compression).Placeholder(locale.T("留空或 none / zstd")).Disabled(disabled)
				})
				ui.Field(c, locale.T("加密算法"), func() {
					ui.TextInput(c, &config.EncryptionAlgorithm).Placeholder(locale.T("默认 aes-gcm，可选 chacha20 / aes-256-gcm / xor")).Disabled(disabled)
				})
			})
		})
		card(c, func() {
			ui.Text(c, locale.T("IPv6 与其他")).Bold()
			ui.Form(c, func() {
				ui.Field(c, locale.T("虚拟 IPv6"), func() { ui.TextInput(c, &config.IPv6Address).Placeholder("fd00::1/64").Disabled(disabled) })
			})
			toggleRow(c, t, locale.T("启用 IPv6 传输"), locale.T("通过 IPv6 连接其他节点"), &config.EnableIPv6, disabled)
			toggleRow(c, t, locale.T("自动配置公网 IPv6"), locale.T("自动分配本机公网 IPv6 地址"), &config.IPv6PublicAuto, disabled)
			toggleRow(c, t, locale.T("提供公网 IPv6 地址"), locale.T("向共享 IPv6 子网的其他节点提供本机的公网 IPv6 地址"), &config.IPv6PublicProvider, disabled)
			toggleRow(c, t, locale.T("禁用 UPnP"), locale.T("不通过路由器自动映射端口"), &config.DisableUPnP, disabled)
			toggleRow(c, t, "Magic DNS", locale.T("修改系统 DNS，以 hostname.et.net 访问节点；请谨慎启用"), &config.MagicDNS, disabled)
			toggleRow(c, t, locale.T("私有模式"), locale.T("拒绝无法验证网络密码的外部网络"), &config.PrivateMode, disabled)
			toggleRow(c, t, locale.T("UDP 广播中继"), locale.T("Windows 下转发 UDP 广播"), &config.UDPBroadcastRelay, disabled)
		})
	})
}

func integerInput(c *ui.Context, value *int, max int, disabled bool) {
	v := float64(*value)
	ui.NumberInput(c, &v, 0, float64(max), 1).Disabled(disabled)
	*value = int(v)
}

func stringListInput(c *ui.Context, text *string, hint string, disabled bool) {
	ui.TextArea(c, text).Height(64).Placeholder(hint).Disabled(disabled)
}
