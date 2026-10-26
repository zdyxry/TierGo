package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) logsPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if a.remoteActive {
		a.emptyState(c, t, locale.T("远端实例的进程日志不在本机；「监控」页可查看远端 Core 事件"))
		return
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf(locale.T("共 %d 条"), len(snap.Logs))).FontSize(12).TextColor(t.TextMuted).Grow(1)
		if ui.Button(c, locale.T("清空")).Disabled(len(snap.Logs) == 0).Clicked() {
			a.ctl().ClearLogs()
		}
	})
	if len(snap.Logs) == 0 {
		a.emptyState(c, t, locale.T("还没有日志"))
		return
	}
	a.logsList.FollowEnd = true
	ui.List(c, &a.logsList, len(snap.Logs), func(i int) {
		logRow(c, t, snap.Logs[i])
	}).Gap(4).Grow(1).MinHeight(320)
}
