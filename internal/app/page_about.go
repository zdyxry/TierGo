package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/buildinfo"
	"github.com/zdyxry/easytier-mygo/internal/corebin"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) aboutPage(c *ui.Context, t *ui.Theme) {
	snap := a.ctl().Snapshot()
	card(c, func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(40, 40).Radius(12).Background(t.Accent).Center().Children(func() {
				ui.Icon(c, iconNetwork).FontSize(22).TextColor(t.AccentText)
			})
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "TierGo").FontSize(15).Bold()
				ui.Text(c, locale.T("非官方 EasyTier 客户端 · MyGo 原生 UI")).FontSize(12).TextColor(t.TextMuted)
			})
		})
		keyValue(c, locale.T("客户端版本"), buildinfo.Version, false)
		keyValue(c, locale.T("提交"), buildinfo.Commit(), true)
		keyValue(c, locale.T("界面"), locale.T("MyGo 原生 UI（无 WebView）"), false)
		// The embedded version is known without connecting.
		embedded := snap.EmbeddedCore
		if embedded == "" {
			if provenance, ok := corebin.EmbeddedProvenance(); ok {
				embedded = provenance.Version + locale.T("（内嵌）")
			}
		}
		keyValue(c, locale.T("内嵌内核"), embedded, false)
		keyValue(c, locale.T("运行中的内核"), snap.Node.Version, false)
		keyValue(c, locale.T("配置文件"), snap.ConfigPath, true)
		keyValue(c, locale.T("日志目录"), snap.LogPath, true)
		if ui.Button(c, locale.T("打开日志目录")).Disabled(snap.LogPath == "").Clicked() {
			if err := mygo.Shell.OpenPath(snap.LogPath); err != nil {
				c.Toast(err.Error())
			}
		}
		if ui.Button(c, locale.T("复制日志目录")).Disabled(snap.LogPath == "").Clicked() {
			c.WriteClipboard(snap.LogPath)
			c.Toast(locale.T("已复制日志目录"))
		}
		keyValue(c, locale.T("管理端口"), snap.Portal, true)
	})
	card(c, func() {
		ui.Text(c, locale.T("说明")).FontSize(14).Bold()
		for _, line := range []string{
			locale.T("客户端自己拉起一个独立的 easytier-core 进程，使用独立的配置目录、端口和管理端口，") +
				locale.T("不会影响机器上已有的 EasyTier 服务。"),
			locale.T("虚拟网卡、路由、子网代理都由内核自己管理，所以性能和官方命令行完全一致。"),
			locale.T("创建虚拟网卡需要管理员权限；没有权限时内核仍会加入网络并转发端口，只是本机流量进不了虚拟网络。"),
		} {
			ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Box(c).Size(5, 5).Radius(3).Background(t.TextMuted).Margin(7, 0, 0, 0)
				ui.Text(c, line).FontSize(12.5).TextColor(t.TextMuted).Grow(1)
			})
		}
	})
}
