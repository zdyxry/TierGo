package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) routesPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if snap.Phase != easytier.PhaseRunning || len(snap.Routes) == 0 {
		a.emptyState(c, t, locale.T("还没有学到任何路由"))
		return
	}
	routes := snap.Routes
	table := ui.Table(c, &a.routesTable, []ui.TableColumn{
		{Title: locale.T("目标网段"), ID: "dest", Width: 190, Sortable: true},
		{Title: locale.T("下一跳"), ID: "next", Width: 150},
		{Title: locale.T("跳数"), ID: "hops", Width: 80, Align: ui.End},
		{Title: locale.T("延迟"), ID: "latency", Width: 100, Align: ui.End, Sortable: true},
		{Title: locale.T("主机名"), ID: "host", Sortable: true},
		{Title: locale.T("代理网段"), ID: "proxy"},
	}, len(routes), func(row, col int) {
		route := routes[row]
		switch col {
		case 0:
			ui.Text(c, route.Dest).SingleLine()
		case 1:
			ui.Text(c, route.NextHop).TextColor(t.TextMuted).SingleLine()
		case 2:
			ui.Text(c, fmt.Sprint(route.Hops)).TextColor(t.TextMuted)
		case 3:
			ui.Text(c, latencyText(route.LatencyMS)).TextColor(latencyColor(t, route.LatencyMS))
		case 4:
			ui.Text(c, route.Name).SingleLine()
		case 5:
			ui.Text(c, route.ProxyCIDRs).TextColor(t.TextMuted).SingleLine()
		}
	})
	table.Grow(1).MinHeight(320)
}
