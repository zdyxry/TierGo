package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// servicePage manages the system service that runs networks without the window.
func (a *App) servicePage(c *ui.Context, t *ui.Theme) {
	a.refreshServiceStatus(false)

	card(c, func() {
		busy := a.asyncBusy("service-operation")
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locale.T("系统服务")).FontSize(14).Bold().Grow(1)
			ui.Badge(c, a.serviceState.String()).
				Background(serviceStateColor(t, a.serviceState).Alpha(0.14)).
				TextColor(serviceStateColor(t, a.serviceState))
			if ui.Button(c, locale.T("刷新状态")).Disabled(busy || a.asyncBusy("service-status")).Clicked() {
				a.refreshServiceStatus(true)
			}
		})
		if busy {
			ui.Text(c, locale.T("正在执行服务操作…")).FontSize(12).TextColor(t.TextMuted)
		}
		if a.serviceErr != "" {
			ui.Text(c, a.serviceErr).FontSize(12).TextColor(t.Danger)
		}
		// An upgraded client replaced the embedded core in place; the service
		// picks it up on its next restart, but its config should be re-synced.
		// Reinstalling needs elevation, so this never happens on its own.
		if a.serviceDrift {
			ui.Row(c).Gap(10).Padding(10).Radius(8).
				Background(t.Warning.Alpha(0.12)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconShield).FontSize(15).TextColor(t.Warning)
				ui.Text(c, fmt.Sprintf(locale.T("内嵌 Core 已从 %s 变为 %s，点「安装或更新」同步服务配置"),
					a.serviceDriftFrom, a.serviceDriftTo)).FontSize(12).TextColor(t.Warning).Grow(1)
			})
		}
		ui.Text(c, locale.T("服务以系统方式运行当前所有网络：开机自动启动，崩溃后由系统自动重启，")+
			locale.T("无需打开客户端。修改网络配置后，点「安装或更新」同步给服务。")).
			FontSize(12).TextColor(t.TextMuted)

		installed := a.serviceState == easytier.ServiceStopped || a.serviceState == easytier.ServiceRunning
		running := a.serviceState == easytier.ServiceRunning
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, locale.T("安装或更新")).Disabled(busy).Clicked() {
				profiles := append([]easytier.Profile(nil), a.profiles...)
				settings := a.settings.Service
				install := a.serviceInstallFn
				a.runServiceOp(locale.T("服务已安装"), func(ctx context.Context) error {
					return install(ctx, profiles, settings)
				})
			}
			if ui.Button(c, locale.T("启动")).Disabled(busy || !installed || running).Clicked() {
				a.startSystemService()
			}
			if ui.Button(c, locale.T("停止")).Disabled(busy || !running).Clicked() {
				a.runServiceOp(locale.T("服务已停止"), easytier.ServiceStop)
			}
			if ui.Button(c, locale.T("卸载")).Disabled(busy || !installed).Clicked() {
				a.runServiceOp(locale.T("服务已卸载"), easytier.ServiceUninstall)
			}
		})
	})

	a.serviceOptionsCard(c, t)
}

// serviceOptionsCard edits the install-time service options. They persist
// with the other app settings but only take effect on the next install.
func (a *App) serviceOptionsCard(c *ui.Context, t *ui.Theme) {
	if !a.serviceOptionsInit {
		a.serviceOptions = a.settings.Service
		a.serviceOptionsInit = true
	}
	card(c, func() {
		ui.Text(c, locale.T("服务选项")).FontSize(14).Bold()
		ui.Text(c, locale.T("安装或更新时生效；留空保持自动行为。")).FontSize(12).TextColor(t.TextMuted)
		ui.TextInput(c, &a.serviceOptions.ConfigDir).Label(locale.T("服务配置目录")).
			Placeholder(locale.T("默认：配置目录下的 service-configs"))
		ui.TextInput(c, &a.serviceOptions.RPCAddress).Label(locale.T("服务 RPC 地址")).
			Placeholder(locale.T("默认：自动挑选空闲端口，如 127.0.0.1:15999"))
		ui.TextInput(c, &a.serviceOptions.LogDir).Label(locale.T("服务日志目录")).
			Placeholder(locale.T("默认：服务工作目录"))
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, locale.T("服务文件日志级别")).FontSize(13)
				ui.Text(c, locale.T("默认关闭；开启后远程管理页可从服务日志推断 Config Server 连接状态")).
					FontSize(11.5).TextColor(t.TextMuted)
			})
			levels := append([]string{""}, easytier.LogLevels...)
			labels := make([]string, len(levels))
			chosen := 0
			for index, level := range levels {
				if level == "" {
					labels[index] = locale.T("默认")
				} else {
					labels[index] = easytier.LogLevelName(level)
				}
				if level == a.serviceOptions.FileLogLevel {
					chosen = index
				}
			}
			ui.Segmented(c, &chosen, labels...).Width(360)
			if levels[chosen] != a.serviceOptions.FileLogLevel {
				a.serviceOptions.FileLogLevel = levels[chosen]
			}
		})
		if ui.Button(c, locale.T("保存服务选项")).Clicked() {
			if err := a.serviceOptions.Validate(); err != nil {
				a.notice = err.Error()
			} else {
				a.settings.Service = a.serviceOptions
				a.saveAppSettings(c)
			}
		}
	})
}

func serviceStateColor(t *ui.Theme, state easytier.ServiceState) ui.Color {
	switch state {
	case easytier.ServiceRunning:
		return t.Success
	case easytier.ServiceStopped:
		return t.Warning
	default:
		return t.TextMuted
	}
}

type serviceStatusResult struct {
	state     easytier.ServiceState
	err       error
	drift     bool
	driftFrom string
	driftTo   string
}

func queryServiceStatus(ctx context.Context, status func(context.Context) (easytier.ServiceState, error), check func(easytier.ServiceSettings) (string, string, bool), settings easytier.ServiceSettings) serviceStatusResult {
	result := serviceStatusResult{}
	result.state, result.err = status(ctx)
	if result.err == nil {
		result.driftFrom, result.driftTo, result.drift = check(settings)
	}
	return result
}

// refreshServiceStatus asks the operating system about the service, at most
// every few seconds unless forced: each answer costs a process spawn.
func (a *App) refreshServiceStatus(force bool) {
	if a.asyncBusy("service-operation") || (!force && time.Since(a.serviceCheckedAt) < 10*time.Second) {
		return
	}
	generation := a.serviceGeneration
	status := a.serviceStatus
	check := a.serviceDriftCheck
	if check == nil {
		check = easytier.ServiceCoreDrift
	}
	settings := a.settings.Service
	a.runAsync("service-status", func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		result := queryServiceStatus(ctx, status, check, settings)
		cancel()
		return func() {
			if generation != a.serviceGeneration {
				return
			}
			a.applyServiceStatus(result)
		}
	})
}

func (a *App) applyServiceStatus(result serviceStatusResult) {
	a.serviceCheckedAt = time.Now()
	if result.err != nil {
		a.serviceState = easytier.ServiceUnknown
		a.serviceErr = locale.T("读取服务状态失败：") + result.err.Error()
		return
	}
	a.serviceState = result.state
	a.serviceErr = ""
	a.serviceDriftFrom, a.serviceDriftTo, a.serviceDrift = result.driftFrom, result.driftTo, result.drift
}

func (a *App) startSystemService() {
	stopAll := a.stopAllAndWait
	if stopAll == nil {
		stopAll = a.mgr.StopAllAndWait
	}
	start := a.serviceStartFn
	if start == nil {
		start = easytier.ServiceStart
	}
	a.runServiceOpAndThen(locale.T("服务已启动"), func(ctx context.Context) error {
		if err := stopAll(5 * time.Second); err != nil {
			return fmt.Errorf(locale.T("停止本地网络失败：%w"), err)
		}
		return start(ctx)
	}, a.clearEnabledNetworks)
}

// runServiceOp performs one service operation and reports the outcome.
func (a *App) runServiceOp(done string, op func(context.Context) error) {
	a.runServiceOpAndThen(done, op, nil)
}

func (a *App) runServiceOpAndThen(done string, op func(context.Context) error, onSuccess func()) {
	if a.asyncBusy("service-operation") {
		return
	}
	a.serviceGeneration++
	generation := a.serviceGeneration
	status := a.serviceStatus
	check := a.serviceDriftCheck
	if check == nil {
		check = easytier.ServiceCoreDrift
	}
	settings := a.settings.Service
	a.runAsync("service-operation", func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		opErr := op(ctx)
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		result := queryServiceStatus(ctx, status, check, settings)
		cancel()
		return func() {
			if generation != a.serviceGeneration {
				return
			}
			a.applyServiceStatus(result)
			if opErr != nil {
				a.notice = opErr.Error()
				return
			}
			a.notice = done
			if onSuccess != nil {
				onSuccess()
			}
		}
	})
}
