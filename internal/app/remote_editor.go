package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func remoteNetworkLabel(network easytier.RemoteNetwork) string {
	label := network.Name + " · " + network.ID
	if network.Disabled {
		label += locale.T("（已停用）")
	}
	return label
}

const remoteOperationTask = "remote-operation"

func (a *App) remoteBusy() bool { return a.asyncBusy(remoteOperationTask) }

func (a *App) openRemoteConfig(address string, network easytier.RemoteNetwork) {
	get := a.getRemoteConfig
	if get == nil {
		get = easytier.GetRemoteConfig
	}
	a.runAsync(remoteOperationTask, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		config, err := get(ctx, address, network)
		cancel()
		text := ""
		if err == nil {
			text, err = config.JSON()
		}
		return func() {
			if a.settings.RemoteRPC != address || a.remoteSelected != network.ID {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			a.remoteEditConfig = config
			a.remoteEditText = text
			a.remoteEditAddress = address
			a.remoteEditOpen = true
			a.remoteEditErr = ""
		}
	})
}

func (a *App) saveRemoteEditor() {
	config, err := a.remoteEditConfig.WithJSON(a.remoteEditText)
	if err != nil {
		a.remoteEditErr = err.Error()
		return
	}
	address, id := a.remoteEditAddress, a.remoteEditConfig.Network.ID
	save := a.saveRemoteConfig
	if save == nil {
		save = easytier.SaveRemoteConfig
	}
	a.runAsync(remoteOperationTask, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := save(ctx, address, config)
		cancel()
		return func() {
			if a.settings.RemoteRPC != address || a.remoteEditConfig.Network.ID != id {
				return
			}
			if err != nil {
				a.remoteEditErr = err.Error()
				return
			}
			a.remoteEditOpen = false
			a.remoteEditText = ""
			a.remoteChecked = time.Time{}
		}
	})
}

func (a *App) changeRemoteState() {
	address, target := a.remoteEditAddress, a.remoteStateTarget
	disabled := !target.Disabled
	a.remoteStateConfirm = false
	a.runAsync(remoteOperationTask, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := easytier.SetRemoteDisabled(ctx, address, target, disabled)
		cancel()
		return func() {
			if a.settings.RemoteRPC != address {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			if a.remoteSelected == target.ID {
				a.remoteChecked = time.Time{}
				a.remoteSnapshot = easytier.Snapshot{}
				a.resetRemoteTraffic()
			}
		}
	})
}

func (a *App) createRemoteNetwork() {
	address := a.settings.RemoteRPC
	config := newDraft(easytier.Profile{Config: a.connectionConfig()}).config
	a.remoteCreateConfirm = false
	a.runAsync(remoteOperationTask, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		id, err := easytier.CreateRemoteNetwork(ctx, address, config)
		cancel()
		return func() {
			if a.settings.RemoteRPC != address {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			a.remoteSelected = id
			a.remoteChecked = time.Time{}
		}
	})
}

func (a *App) deleteRemoteNetwork(network easytier.RemoteNetwork) {
	address := a.settings.RemoteRPC
	a.remoteDeleteConfirm = false
	a.runAsync(remoteOperationTask, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := easytier.DeleteRemoteNetwork(ctx, address, network)
		cancel()
		return func() {
			if a.settings.RemoteRPC != address {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			if a.remoteSelected == network.ID {
				a.remoteSelected = ""
				a.remoteSnapshot = easytier.Snapshot{}
				a.remoteChecked = time.Time{}
				a.resetRemoteTraffic()
			}
		}
	})
}

func (a *App) remoteConfigModals(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &a.remoteEditOpen, func() {
		ui.Column(c).Padding(20).Gap(12).Width(680).Children(func() {
			ui.Text(c, locale.T("完整远端 NetworkConfig（JSON）")).Bold()
			ui.Text(c, locale.T("编辑官方配置模型，包含 ACL、安全模式、路由和高级选项。保存会替换运行实例；停用配置仅保存到本机备份。实例 ID 不可更改。"))
			ui.Text(c, a.remoteEditAddress+" · "+a.remoteEditConfig.Network.ID).FontSize(11)
			ui.TextArea(c, &a.remoteEditText).Height(350).Label(locale.T("远端 NetworkConfig JSON"))
			if a.remoteEditErr != "" {
				ui.Text(c, a.remoteEditErr).TextColor(t.Danger)
			}
			ui.Row(c).Gap(10).Children(func() {
				if ui.Button(c, locale.T("取消远端编辑")).Clicked() {
					a.remoteEditOpen = false
					a.remoteEditText = ""
				}
				if ui.PrimaryButton(c, locale.T("保存并应用远端配置")).Disabled(a.remoteBusy()).Clicked() {
					a.saveRemoteEditor()
				}
			})
		})
	})
	ui.Modal(c, &a.remoteStateConfirm, func() {
		ui.Column(c).Padding(20).Gap(12).Width(500).Children(func() {
			ui.Text(c, locale.T("确认更改远端网络运行状态？")).Bold()
			ui.Text(c, a.remoteEditAddress+" · "+a.remoteStateTarget.ID)
			ui.Text(c, locale.T("停用将先在本机保存完整配置再停止远端实例；重新启用需要此备份。本机备份权限 0600，包含凭据。"))
			if ui.Button(c, locale.T("取消状态更改")).Clicked() {
				a.remoteStateConfirm = false
			}
			if ui.PrimaryButton(c, locale.T("确认状态更改")).Disabled(a.remoteBusy()).Clicked() {
				a.changeRemoteState()
			}
		})
	})
}
