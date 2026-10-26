package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) overviewPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	card(c, func() {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			dot(c, 10, statusColor(t, snap.Phase))
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				title := statusLabel(snap.Phase)
				if snap.Phase == easytier.PhaseRunning {
					networkName := a.draft().config.NetworkName
					if a.remoteActive {
						networkName = a.currentInstanceName()
					}
					title = fmt.Sprintf(locale.T("已连接到 %s"), networkName)
				}
				ui.Text(c, title).FontSize(17).Bold()
				subtitle := locale.T("点击右上角「连接」加入网络")
				switch {
				case snap.Phase == easytier.PhaseRunning:
					subtitle = fmt.Sprintf(locale.T("本机 %s"), snap.Node.IPv4)
					if !snap.TUN {
						subtitle += locale.T("，仅端口转发（未启用虚拟网卡）")
					}
				case snap.Phase == easytier.PhaseFailed && snap.Err != "":
					subtitle = snap.Err
				}
				ui.Text(c, subtitle).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
			})
		})
		// What the core did not answer for is said, not papered over with
		// the values from the last successful read.
		if len(snap.Unreadable) > 0 {
			ui.Badge(c, locale.T("无法读取 ")+strings.Join(snap.Unreadable, "、")).
				Background(t.Warning.Alpha(0.16)).TextColor(t.Warning)
		}
	})

	ui.Row(c).Gap(16).Children(func() {
		ui.Column(c).Grow(1).Children(func() {
			a.statCard(c, t, iconPeers, locale.T("在线节点"), fmt.Sprint(peerCount(snap)))
		})
		ui.Column(c).Grow(1).Children(func() {
			a.statCard(c, t, iconRoutes, locale.T("路由条目"), fmt.Sprint(len(snap.Routes)))
		})
		ui.Column(c).Grow(1).Children(func() {
			uptime := "—"
			if snap.Phase == easytier.PhaseRunning && !snap.Since.IsZero() {
				uptime = shortDuration(time.Since(snap.Since))
			}
			a.statCard(c, t, iconRefresh, locale.T("已连接时长"), uptime)
		})
	})

	card(c, func() {
		ui.Text(c, locale.T("本设备")).FontSize(14).Bold()
		if snap.Phase != easytier.PhaseRunning {
			ui.Text(c, locale.T("连接后显示")).FontSize(12.5).TextColor(t.TextMuted)
			return
		}
		keyValue(c, locale.T("主机名"), snap.Node.Hostname, false)
		keyValue(c, locale.T("虚拟 IP"), snap.Node.IPv4, true)
		keyValue(c, locale.T("节点 ID"), snap.Node.PeerID, false)
		keyValue(c, locale.T("实例 ID"), snap.Node.InstID, true)
		keyValue(c, locale.T("内核版本"), snap.Node.Version, false)
		keyValue(c, locale.T("监听地址"), strings.Join(snap.Node.Listeners, ", "), false)
		keyValue(c, locale.T("代理网段"), strings.Join(snap.Node.ProxyCIDRs, ", "), false)
		keyValue(c, locale.T("公网 IP"), strings.Join(snap.Node.PublicIPs, ", "), false)
		keyValue(c, locale.T("接口 IPv4"), strings.Join(snap.Node.InterfaceIPv4s, ", "), false)
		keyValue(c, locale.T("接口 IPv6"), strings.Join(snap.Node.InterfaceIPv6s, ", "), false)
		keyValue(c, locale.T("公网 IPv6"), snap.Node.PublicIPv6, false)
		keyValue(c, locale.T("TUN 配置名"), snap.Node.TUNName, false)
		if snap.Node.UDPNATType != 0 {
			keyValue(c, locale.T("NAT 类型"), fmt.Sprintf("UDP %s / TCP %s",
				natName(snap.Node.UDPNATType), natName(snap.Node.TCPNATType)), false)
		}
	})

	if len(snap.Logs) > 0 {
		card(c, func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locale.T("最近事件")).FontSize(14).Bold().Grow(1)
				if ui.Button(c, locale.T("查看全部")).Clicked() {
					a.page = "logs"
				}
			})
			recent := snap.Logs
			if len(recent) > 6 {
				recent = recent[len(recent)-6:]
			}
			for _, entry := range recent {
				logRow(c, t, entry)
			}
		})
	}
}
