package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) peersPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if snap.Phase != easytier.PhaseRunning {
		a.emptyState(c, t, locale.T("连接后这里会列出网络中的其他设备"))
		return
	}
	peers := snap.Peers
	if query := strings.ToLower(a.peersQuery); query != "" {
		var filtered []easytier.PeerView
		for _, peer := range peers {
			if strings.Contains(strings.ToLower(peer.Name), query) ||
				strings.Contains(peer.IPv4, query) {
				filtered = append(filtered, peer)
			}
		}
		peers = filtered
	}

	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.SearchField(c, &a.peersQuery).Label(locale.T("搜索名称或地址")).Width(240)
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf(locale.T("%d 台设备"), len(peers))).FontSize(12).TextColor(t.TextMuted)
	})

	if len(peers) == 0 {
		a.emptyState(c, t, locale.T("没有匹配的设备"))
		return
	}

	table := ui.Table(c, &a.peersTable, []ui.TableColumn{
		{Title: locale.T("设备"), ID: "name", Sortable: true},
		{Title: locale.T("虚拟 IP"), ID: "ip", Width: 150, Sortable: true},
		{Title: locale.T("延迟"), ID: "latency", Width: 90, Align: ui.End, Sortable: true},
		{Title: locale.T("连接方式"), ID: "path", Width: 120},
		{Title: locale.T("流量（收/发）"), ID: "traffic", Width: 160},
		{Title: locale.T("版本"), ID: "version", Width: 130, Align: ui.End},
		{Title: locale.T("丢包率"), ID: "loss", Width: 90},
		{Title: "NAT", ID: "nat", Width: 120},
	}, len(peers), func(row, col int) {
		peer := peers[row]
		switch col {
		case 0:
			ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
				dot(c, 8, peerColor(t, peer))
				ui.Text(c, peer.Name).SingleLine()
				if peer.PublicServer {
					ui.Badge(c, locale.T("公共服务器"))
				}
				if peer.AvoidRelay {
					ui.Badge(c, locale.T("避免中继"))
				}
			})
		case 1:
			ui.Text(c, peer.IPv4).SingleLine()
		case 2:
			ui.Text(c, latencyText(peer.LatencyMS)).TextColor(latencyColor(t, peer.LatencyMS))
		case 3:
			ui.Badge(c, pathLabel(peer)).Background(pathColor(t, peer).Alpha(0.14)).
				TextColor(pathColor(t, peer))
		case 4:
			ui.Text(c, trafficText(peer)).TextColor(t.TextMuted).SingleLine()
		case 5:
			ui.Text(c, peer.Version).TextColor(t.TextMuted).SingleLine()
		case 6:
			ui.Text(c, peer.LossRate).SingleLine()
		case 7:
			ui.Text(c, peer.NATType).SingleLine()
		}
	})
	table.Grow(1).MinHeight(320)
}
