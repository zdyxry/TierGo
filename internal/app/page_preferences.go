package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) preferencesPage(c *ui.Context, t *ui.Theme) {
	card(c, func() {
		ui.Text(c, "Language / 语言").FontSize(14).Bold()
		chosen := 0
		if locale.Language() == "en" {
			chosen = 1
		}
		before := chosen
		ui.Segmented(c, &chosen, "简体中文", "English").Label("Language")
		if chosen != before {
			tag := "zh-Hans"
			if chosen == 1 {
				tag = "en"
			}
			a.settings.Language = tag
			if err := a.settings.Save(); err != nil {
				c.Toast(err.Error())
			} else {
				locale.Set(tag)
				if mygo.App.IsReady() {
					mygo.App.SetLocale(tag)
				}
				if a.win != nil {
					a.win.Invalidate()
				}
			}
		}
	})

	card(c, func() {
		ui.Text(c, locale.T("登录时启动客户端")).FontSize(14).Bold()
		before := a.settings.OpenAtLogin
		toggleRow(c, t, locale.T("开机自动启动"),
			locale.T("登录系统时自动打开客户端；窗口关闭后驻留托盘，网络继续运行"),
			&a.settings.OpenAtLogin, false)
		if a.settings.OpenAtLogin != before {
			a.saveAppSettings(c)
		}
	})
}

func (a *App) saveAppSettings(c *ui.Context) {
	if err := a.settings.Save(); err != nil {
		c.Toast(err.Error())
		return
	}
	if mygo.App.IsReady() {
		if err := mygo.App.SetOpenAtLogin(a.settings.OpenAtLogin); err != nil {
			c.Toast(locale.T("设置开机启动失败：") + err.Error())
		}
	}
}
