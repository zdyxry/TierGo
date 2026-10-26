package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Remote RPC is an explicit, non-owning connection. Mutating operations
// require confirmation; disconnecting only leaves the inspector. RPC has no
// authentication: use a trusted tunnel, not the Internet.
func (a *App) remotePage(c *ui.Context, t *ui.Theme) {
	card(c, func() {
		ui.Text(c, locale.T("远程 RPC")).Bold()
		ui.Text(c, locale.T("连接/断开本身不启动或停止远端进程；创建/删除需确认。RPC 不提供身份认证，请通过可信 VPN/SSH 隧道使用。")).FontSize(12).TextColor(t.Warning)
		ui.TextInput(c, &a.settings.RemoteRPC).Placeholder("tcp://127.0.0.1:15888").Label(locale.T("远程 RPC 地址"))
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, locale.T("连接远程 RPC")).Clicked() {
				a.remoteConnected = true
				a.remoteChecked = time.Time{}
				a.resetRemoteTraffic()
				if err := a.settings.Save(); err != nil {
					a.notice = err.Error()
				}
			}
			if ui.Button(c, locale.T("断开远程查看")).Clicked() {
				a.remoteConnected = false
				a.remoteActive = false
				a.remoteNetworks = nil
				a.remoteSnapshot = easytier.Snapshot{}
				a.resetRemoteTraffic()
			}
		})
	})
	card(c, func() {
		ui.Text(c, locale.T("Config Server（普通 / 系统服务模式）")).Bold()
		ui.Text(c, locale.T("由 Core 处理心跳、重连和服务器下发配置。此处不将本地网络配置推送至服务器；安装后需启动服务。")).FontSize(12).TextColor(t.TextMuted)
		ui.TextInput(c, &a.settings.Central.Endpoint).Label(locale.T("Config Server 地址")).Placeholder(locale.T("udp://服务器:22020/用户令牌"))
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.settings.Central.MachineID).Label("Machine ID").Grow(1)
			if ui.Button(c, locale.T("生成稳定 Machine ID")).Clicked() {
				a.settings.Central.MachineID = easytier.NewMachineID()
			}
		})
		central := a.central.Snapshot()
		c.After(500 * time.Millisecond)
		ui.Text(c, locale.T("普通模式：")+statusLabel(central.Phase))
		if central.TransportConnected {
			ui.Text(c, locale.T("Core 报告传输已连接（不等于已验证服务端心跳）"))
		}
		// The transport claim rests on individual Core stdout lines; show the
		// last one it was inferred from instead of a bare boolean.
		if !central.TransportAt.IsZero() && central.TransportLine != "" {
			ui.Text(c, fmt.Sprintf(locale.T("最近传输状态依据（%s）：%s"),
				central.TransportAt.Format("15:04:05"), central.TransportLine)).
				FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
		if central.Error != "" {
			ui.Text(c, central.Error).TextColor(t.Danger)
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, locale.T("普通模式连接 Config Server")).Disabled(central.Phase == easytier.PhaseRunning || central.Phase == easytier.PhaseStarting).Clicked() {
				if a.settings.Central.MachineID == "" {
					a.settings.Central.MachineID = easytier.NewMachineID()
				}
				if err := a.settings.Save(); err != nil {
					a.notice = err.Error()
				} else if err := a.central.Start(a.settings.Central); err != nil {
					a.notice = err.Error()
				}
			}
			if ui.Button(c, locale.T("停止普通模式集中管理")).Disabled(a.asyncBusy("central-stop")).Clicked() {
				central := a.central
				a.runAsync("central-stop", func() func() {
					err := central.StopAndWait(5 * time.Second)
					return func() {
						if err != nil {
							a.notice = err.Error()
						}
					}
				})
			}
			if ui.Button(c, locale.T("查看普通模式下发网络")).Disabled(central.Portal == "").Clicked() {
				a.viewRemote(central.Portal)
			}
		})
		for _, line := range central.Logs {
			ui.Text(c, line).FontSize(10).SingleLine()
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, locale.T("安装集中管理服务")).Clicked() {
				if err := a.settings.Central.Validate(); err != nil {
					a.notice = err.Error()
				} else if err := a.settings.Save(); err != nil {
					a.notice = err.Error()
				} else {
					centralConfig, serviceSettings := a.settings.Central, a.settings.Service
					install := a.serviceInstallCentralFn
					a.runServiceOp(locale.T("集中管理服务已安装"), func(ctx context.Context) error {
						return install(ctx, centralConfig, serviceSettings)
					})
				}
			}
			if ui.Button(c, locale.T("查看系统服务网络")).Disabled(a.asyncBusy("service-rpc-address")).Clicked() {
				a.runAsync("service-rpc-address", func() func() {
					address, err := easytier.ServiceRPCAddress()
					return func() {
						if err != nil {
							a.notice = err.Error()
						} else {
							a.viewRemote(address)
						}
					}
				})
			}
		})
		a.serviceModeStatus(c, t)
	})
	if !a.remoteConnected {
		return
	}
	// The view layer drives the poll on every page while a remote instance
	// is active; this page only draws what the poll brought in.
	card(c, func() {
		if a.remoteErr != "" {
			ui.Text(c, locale.T("连接失败，将在下次刷新时重试：")+a.remoteErr).TextColor(t.Danger)
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, locale.T("刷新远程状态")).Clicked() {
				a.remoteChecked = time.Time{}
			}
			ui.Text(c, fmt.Sprintf(locale.T("%d 个远端网络"), len(a.remoteNetworks))).FontSize(12).TextColor(t.TextMuted)
		})
		for _, network := range a.remoteNetworks {
			if ui.Button(c, remoteNetworkLabel(network)).Clicked() {
				if a.remoteSelected != network.ID {
					a.remoteSelected = network.ID
					a.resetRemoteTraffic()
				}
				a.remoteChecked = time.Time{}
			}
		}
		ui.Text(c, locale.T("远端新建仅支持基础字段；操作将真实更改远端。高级草稿会被拒绝而不是丢弃。请确认当前本地选中的草稿。")).FontSize(12).TextColor(t.TextMuted)
		if a.remoteBusy() {
			ui.Text(c, locale.T("正在执行远端操作…")).FontSize(12).TextColor(t.TextMuted)
		}
		if ui.Button(c, locale.T("将当前草稿创建为远端新网络")).Disabled(a.remoteBusy()).Clicked() {
			a.remoteCreateConfirm = true
		}
		for _, network := range a.remoteNetworks {
			if network.ID == a.remoteSelected {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					if ui.Button(c, locale.T("编辑完整远端配置")).Disabled(a.remoteBusy() || network.Permissions&1 != 0).Clicked() {
						a.openRemoteConfig(a.settings.RemoteRPC, network)
					}
					label := locale.T("停用远端网络（保留配置）")
					if network.Disabled {
						label = locale.T("启用远端网络")
					}
					if ui.Button(c, label).Disabled(a.remoteBusy() || network.Permissions&3 != 0).Clicked() {
						a.remoteStateTarget = network
						a.remoteStateConfirm = true
						a.remoteEditAddress = a.settings.RemoteRPC
					}
					if ui.Button(c, locale.T("删除选中远端网络")).Disabled(a.remoteBusy() || network.Permissions&2 != 0).Clicked() {
						a.remoteDeleteConfirm = true
					}
				})
			}
		}
	})
	ui.Modal(c, &a.remoteCreateConfirm, func() {
		ui.Column(c).Padding(20).Gap(14).Width(380).Children(func() {
			ui.Text(c, locale.T("在远端创建网络")).FontSize(15).Bold()
			ui.Text(c, locale.T("确定在远端创建当前草稿网络？可能创建虚拟网卡或暴露监听端口。")).FontSize(12.5)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locale.T("取消远端创建")).Clicked() {
					a.remoteCreateConfirm = false
				}
				if ui.PrimaryButton(c, locale.T("确认远端创建")).Disabled(a.remoteBusy()).Clicked() {
					a.createRemoteNetwork()
				}
			})
		})
	})
	ui.Modal(c, &a.remoteDeleteConfirm, func() {
		ui.Column(c).Padding(20).Gap(14).Width(380).Children(func() {
			ui.Text(c, locale.T("删除远端网络")).FontSize(15).Bold()
			ui.Text(c, locale.T("删除会停止选中远端网络并移除其配置，无法撤销。")).FontSize(12.5)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locale.T("取消远端删除")).Clicked() {
					a.remoteDeleteConfirm = false
				}
				if ui.PrimaryButton(c, locale.T("确认远端删除")).Disabled(a.remoteBusy()).Clicked() {
					for _, network := range a.remoteNetworks {
						if network.ID == a.remoteSelected {
							a.deleteRemoteNetwork(network)
						}
					}
				}
			})
		})
	})
	if a.remoteSnapshot.Phase == easytier.PhaseRunning {
		card(c, func() {
			keyValue(c, locale.T("远端主机"), a.remoteSnapshot.Node.Hostname, false)
			keyValue(c, locale.T("虚拟 IP"), a.remoteSnapshot.Node.IPv4, true)
			keyValue(c, locale.T("版本"), a.remoteSnapshot.Node.Version, false)
		})
		a.peersPage(c, t, a.remoteSnapshot)
		a.routesPage(c, t, a.remoteSnapshot)
	}
}

// serviceModeStatus shows what the service mode knows about Config Server
// connectivity. The official GUI shows "unknown" here — no RPC exposes the
// state and the service is a separate process — so the only signal is the
// service's file log, which exists only when the service options enable it.
// The answer is inference from log lines, shown apart from the process
// state and never presented as a verified server heartbeat.
func (a *App) serviceModeStatus(c *ui.Context, t *ui.Theme) {
	if time.Since(a.serviceLogAt) > 5*time.Second && !a.asyncBusy("service-log-scan") {
		scan := a.serviceLogScan
		if scan == nil {
			settings := a.settings.Service
			scan = func() (easytier.ServiceLogStatus, error) {
				return easytier.ScanServiceConnectionLog(settings)
			}
		}
		a.serviceLogAt = time.Now()
		a.runAsync("service-log-scan", func() func() {
			status, err := scan()
			return func() {
				a.serviceLog = status
				a.serviceLogErr = ""
				if err != nil {
					a.serviceLogErr = err.Error()
				}
			}
		})
	}
	ui.Text(c, locale.T("服务模式连接状态")).FontSize(13).Bold()
	switch {
	case a.serviceLogErr != "":
		ui.Text(c, locale.T("服务日志读取失败：")+a.serviceLogErr).FontSize(12).TextColor(t.Danger)
	case !a.serviceLog.Found:
		ui.Text(c, locale.T("服务模式无连接状态通道（官方 GUI 同样没有）；在服务页设置文件日志级别并重新安装后，此处可从服务日志推断。")).
			FontSize(12).TextColor(t.TextMuted)
	case !a.serviceLog.Known:
		ui.Text(c, locale.T("服务日志中暂无 Config Server 连接记录。")).FontSize(12).TextColor(t.TextMuted)
	default:
		state := locale.T("未连接")
		color := t.Warning
		if a.serviceLog.Connected {
			state = locale.T("已连接")
			color = t.Success
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			dot(c, 7, color)
			ui.Text(c, fmt.Sprintf(locale.T("服务日志最近报告：%s（推断） · %s"),
				state, a.serviceLog.At.Format("01-02 15:04:05"))).FontSize(12.5)
		})
		ui.Text(c, fmt.Sprintf(locale.T("来自 %s 尾部的日志行，非实时心跳；服务进程运行也不等于已连通。"), a.serviceLog.Path)).
			FontSize(11).TextColor(t.TextMuted)
		if a.serviceLog.Line != "" {
			ui.Text(c, a.serviceLog.Line).FontSize(10).TextColor(t.TextMuted).SingleLine()
		}
	}
}
