package app

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
	"time"
)

func (a *App) settingsPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if a.remoteActive {
		a.emptyState(c, t, locale.T("远端实例的配置请在「远程管理」页用完整 NetworkConfig 编辑"))
		return
	}
	running := a.mgr.Running(a.selected)
	draft := a.draft()
	profile := a.selectedProfile()

	card(c, func() {
		ui.Text(c, locale.T("网络")).FontSize(14).Bold()
		ui.Form(c, func() {
			ui.Field(c, locale.T("显示名称"), func() {
				ui.TextInput(c, &draft.name).Placeholder(locale.T("在左侧列表中显示的名字")).Disabled(running)
			})
			ui.Field(c, locale.T("网络名称"), func() {
				ui.TextInput(c, &draft.config.NetworkName).Placeholder("easytier").Disabled(running)
			})
			ui.Field(c, locale.T("网络密码"), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					secret := ui.TextInput(c, &draft.config.NetworkSecret).
						Placeholder(locale.T("留空表示不校验")).Disabled(running).Grow(1)
					if !draft.showSecret {
						secret.Password()
					}
					eye, eyeLabel := iconEye, locale.T("显示密码")
					if draft.showSecret {
						eye, eyeLabel = iconEyeOff, locale.T("隐藏密码")
					}
					toggle := ui.Button(c, "").Children(func() {
						ui.Icon(c, eye).FontSize(14)
					}).Label(eyeLabel)
					if toggle.Clicked() {
						draft.showSecret = !draft.showSecret
					}
				})
			})
			ui.Field(c, locale.T("本机名称"), func() {
				ui.TextInput(c, &draft.config.Hostname).Placeholder(locale.T("默认使用系统主机名")).Disabled(running)
			})
			ui.Field(c, locale.T("虚拟 IP"), func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c, &draft.config.IPv4).Placeholder("10.144.0.10/24").Disabled(running).Grow(1)
					ui.Text(c, "DHCP").FontSize(12).TextColor(t.TextMuted)
					// Flipping the switch makes DHCP explicit; leaving it
					// alone keeps the old IPv4/TUN inference.
					dhcpOn := effectiveDHCP(draft.config)
					before := dhcpOn
					ui.Switch(c, &dhcpOn).Label(locale.T("DHCP 自动获取")).
						Disabled(running || draft.config.IPv4 != "")
					if dhcpOn != before {
						draft.config.DHCP = &dhcpOn
					}
				})
			})
		})
	})

	card(c, func() {
		ui.Text(c, locale.T("连接")).FontSize(14).Bold()
		ui.Form(c, func() {
			ui.Field(c, locale.T("对端节点"), func() {
				ui.TextArea(c, &draft.peersText).Height(88).
					Placeholder(locale.T("tcp://public.top:11010（每行一个）")).Disabled(running)
			})
			ui.Field(c, locale.T("监听地址"), func() {
				ui.TextArea(c, &draft.listenersText).Height(64).
					Placeholder(locale.T("留空自动选择端口，例如 tcp://0.0.0.0:11010")).Disabled(running)
			})
			ui.Field(c, locale.T("代理网段"), func() {
				ui.TextArea(c, &draft.proxyText).Height(64).
					Placeholder(locale.T("把本机局域网共享给其他节点，例如 192.168.1.0/24")).Disabled(running)
			})
			ui.Field(c, locale.T("端口转发"), func() {
				ui.TextArea(c, &draft.forwardText).Height(64).
					Placeholder(locale.T("把本机端口转发到对端，例如 tcp://127.0.0.1:5202/10.144.0.20:5201")).
					Disabled(running)
			})
		})
	})

	card(c, func() {
		ui.Text(c, locale.T("选项")).FontSize(14).Bold()
		toggleRow(c, t, locale.T("开启虚拟网卡"),
			locale.T("让本机所有程序都能访问虚拟网络里的任意主机和端口；关闭后仅端口转发生效"),
			&draft.config.EnableTUN, running)
		toggleRow(c, t, locale.T("数据加密"), locale.T("对端到端流量加密"), &draft.config.Encryption, running)
		toggleRow(c, t, locale.T("安全模式"), locale.T("使用 X25519 密钥协商，需要设置网络密码"), &draft.config.SecureMode, running)
		toggleRow(c, t, locale.T("延迟优先"), locale.T("按延迟而不是跳数选择路径"), &draft.config.LatencyFirst, running)
		previousLevel := draft.config.LogLevel
		profileID := a.selected
		key := "settings-log-level:" + profileID
		ui.Row(c).Disabled(a.asyncBusy(key)).Children(func() {
			logLevelRow(c, t, &draft.config.LogLevel)
		})
		if running && previousLevel != draft.config.LogLevel {
			address, level := snap.Portal, draft.config.LogLevel
			setLevel := easytier.SetRuntimeLogLevel
			a.runAsync(key, func() func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err := setLevel(ctx, address, level)
				cancel()
				return func() {
					if a.remoteActive || a.selected != profileID {
						return
					}
					if err != nil {
						if a.draft().config.LogLevel == level {
							a.draft().config.LogLevel = previousLevel
						}
						a.notice = err.Error()
					} else {
						a.notice = locale.T("日志级别已立即应用")
					}
				}
			})
		}
		if a.needsElevation() {
			ui.Text(c, locale.T("开启虚拟网卡后，需要以管理员身份重启客户端才能生效。")).
				FontSize(12).TextColor(t.Warning)
		}
	})

	a.advancedSettings(c, t, &draft.config, running)

	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.PrimaryButton(c, locale.T("保存设置")).Disabled(running).Clicked() {
			a.saveSettings(c)
		}
		if running {
			ui.Text(c, locale.T("连接中无法修改，请先断开")).FontSize(12).TextColor(t.TextMuted)
		} else if snap.Phase == easytier.PhaseFailed {
			ui.Text(c, locale.T("上次连接失败，可在日志页查看原因")).FontSize(12).TextColor(t.Danger)
		}
	})

	card(c, func() {
		ui.Text(c, locale.T("配置文件")).FontSize(14).Bold()
		ui.Text(c, locale.T("以 easytier-core 的 TOML 格式导入导出，方便备份与分享；")+
			locale.T("不认识的高级字段在导入时会被忽略。")).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, locale.T("导入 TOML")).Disabled(running).Clicked() {
				a.importTOMLFile()
			}
			if ui.Button(c, locale.T("导出 TOML")).Clicked() {
				a.exportTOMLFile()
			}
			if ui.Button(c, locale.T("编辑 TOML")).Disabled(running).Clicked() {
				a.openTOMLEditor()
			}
		})
	})

	a.tomlEditorModal(c, t)

	card(c, func() {
		ui.Text(c, locale.T("危险操作")).FontSize(14).Bold()
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, locale.T("删除此网络")).FontSize(13)
				ui.Text(c, locale.T("配置会被移除；断开中的网络不能删除")).FontSize(11.5).TextColor(t.TextMuted)
			})
			if ui.Button(c, locale.T("删除")).Disabled(running || len(a.profiles) <= 1).Clicked() {
				a.confirmDelete = true
			}
		})
	})

	a.deleteProfileModal(c, profile)
}

// effectiveDHCP is what the DHCP switch shows: the explicit value when one
// was set, otherwise the export's inference.
func effectiveDHCP(config easytier.Config) bool {
	if config.DHCP != nil {
		return *config.DHCP
	}
	return config.IPv4 == "" && config.EnableTUN
}

// deleteProfileModal confirms removing the selected network.
func (a *App) deleteProfileModal(c *ui.Context, profile easytier.Profile) {
	ui.Modal(c, &a.confirmDelete, func() {
		ui.Column(c).Padding(20).Gap(14).Width(360).Children(func() {
			ui.Text(c, locale.T("删除网络")).FontSize(15).Bold()
			ui.Text(c, locale.T("确定要删除「")+profile.DisplayName()+locale.T("」吗？它的配置会被移除，此操作不可撤销。")).
				FontSize(12.5)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locale.T("取消")).Clicked() {
					a.confirmDelete = false
				}
				if ui.PrimaryButton(c, locale.T("确认删除")).Clicked() {
					a.deleteProfile(profile.ID)
				}
			})
		})
	})
}

func (a *App) deleteProfile(id string) {
	a.confirmDelete = false
	if a.mgr.Running(id) || len(a.profiles) <= 1 {
		return
	}
	if err := a.mgr.Remove(id); err != nil {
		a.notice = err.Error()
		return
	}
	if err := easytier.DeleteProfile(id); err != nil {
		a.notice = err.Error()
		return
	}
	delete(a.drafts, id)
	remaining := a.profiles[:0]
	for _, profile := range a.profiles {
		if profile.ID != id {
			remaining = append(remaining, profile)
		}
	}
	a.profiles = remaining
	if a.selected == id {
		a.selected = a.profiles[0].ID
		a.page = "overview"
	}
	// A deleted network must not come back on the next launch.
	a.setNetworkEnabled(id, false)
}

func (a *App) saveSettings(c *ui.Context) {
	config := a.connectionConfig()
	if err := easytier.EnsurePortalKey(config.VPNPortal); err != nil {
		c.Toast(err.Error())
		return
	}
	// The secure-mode identity is generated once and then saved with the
	// profile, so exports and restarts keep the same node.
	if err := easytier.EnsureSecureModeKeys(&config); err != nil {
		c.Toast(err.Error())
		return
	}
	if err := config.Validate(); err != nil {
		c.Toast(err.Error())
		return
	}
	profile := a.selectedProfile()
	profile.Config = config
	if name := a.draft().name; name != "" {
		profile.Name = name
	}
	if err := easytier.SaveProfile(profile); err != nil {
		c.Toast(err.Error())
		return
	}
	for index := range a.profiles {
		if a.profiles[index].ID == profile.ID {
			a.profiles[index] = profile
		}
	}
	// Re-read the draft from what was saved, so the editors show exactly
	// what the next run will use.
	a.drafts[profile.ID] = newDraft(profile)
	c.Toast(locale.T("设置已保存"))
}
