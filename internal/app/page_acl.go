package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) aclPage(c *ui.Context, t *ui.Theme) {
	if a.remoteActive {
		a.remoteACLPage(c, t)
		return
	}
	a.localACLPage(c, t)
}

// aclEditing is the ACL the editor acts on: the local draft, or the copy
// read from the remote instance.
func (a *App) aclEditing() *easytier.ACL {
	if a.remoteActive {
		return a.remoteACL
	}
	return a.draft().config.ACL
}

func (a *App) aclMembersBuffer() *string {
	if a.remoteActive {
		return &a.remoteACLMembers
	}
	return &a.draft().aclMembersText
}

func (a *App) localACLPage(c *ui.Context, t *ui.Theme) {
	config := &a.draft().config
	running := a.ctl().Running()
	card(c, func() {
		ui.Text(c, locale.T("ACL 与访问控制组")).FontSize(14).Bold()
		ui.Text(c, locale.T("优先级越大越先匹配；规则限制虚拟网络流量。修改后保存设置，下次连接生效。")).FontSize(12).TextColor(t.TextMuted)
		if config.ACL == nil {
			if ui.PrimaryButton(c, locale.T("配置 ACL")).Disabled(running).Clicked() {
				config.ACL = easytier.DefaultACL()
			}
			return
		}
		if ui.Button(c, locale.T("移除 ACL")).Disabled(running).Clicked() {
			a.removeACLOpen = true
		}
	})
	if config.ACL == nil {
		a.aclModals(c, t)
		return
	}
	a.aclEditor(c, t, config.ACL, running)
	if ui.PrimaryButton(c, locale.T("保存 ACL")).Disabled(running).Clicked() {
		a.saveSettings(c)
	}
	a.aclModals(c, t)
}

// remoteACLPage reuses the editor for the selected remote instance. Reading
// and saving go through the remote config RPC; saving replaces the running
// instance after Core-side validation.
func (a *App) remoteACLPage(c *ui.Context, t *ui.Theme) {
	network, ok := a.remoteSelectedNetwork()
	if !ok {
		a.emptyState(c, t, locale.T("请先在左侧选择远端实例"))
		return
	}
	readOnly := network.Permissions&1 != 0
	key := a.settings.RemoteRPC + "/" + network.ID
	busy := a.asyncBusy("remote-acl:" + key)
	card(c, func() {
		ui.Text(c, locale.T("远端 ACL 与访问控制组")).FontSize(14).Bold()
		ui.Text(c, locale.T("直接读取并修改远端实例的 ACL；保存会先在远端校验，再替换运行中的实例。")).FontSize(12).TextColor(t.Warning)
		if a.remoteACLKey != key {
			if readOnly {
				ui.Text(c, locale.T("远端网络配置只读")).FontSize(12).TextColor(t.Warning)
				return
			}
			if ui.PrimaryButton(c, locale.T("读取远端 ACL")).Disabled(busy).Clicked() {
				a.loadRemoteACL(network)
			}
			return
		}
		if a.remoteACL == nil {
			ui.Text(c, locale.T("远端未配置 ACL")).FontSize(12).TextColor(t.TextMuted)
			if ui.PrimaryButton(c, locale.T("配置 ACL")).Disabled(readOnly || busy).Clicked() {
				a.remoteACL = easytier.DefaultACL()
			}
			return
		}
		ui.Row(c).Gap(10).Children(func() {
			if ui.Button(c, locale.T("移除 ACL")).Disabled(readOnly || busy).Clicked() {
				a.removeACLOpen = true
			}
			if ui.Button(c, locale.T("重新读取远端 ACL")).Disabled(busy).Clicked() {
				a.loadRemoteACL(network)
			}
		})
	})
	if a.remoteACLKey != key || a.remoteACL == nil {
		a.aclModals(c, t)
		return
	}
	a.aclEditor(c, t, a.remoteACL, readOnly || busy)
	if ui.PrimaryButton(c, locale.T("保存远端 ACL")).Disabled(readOnly || busy).Clicked() {
		a.remoteACLSave = true
	}
	a.aclModals(c, t)
	ui.Modal(c, &a.remoteACLSave, func() {
		ui.Column(c).Padding(20).Gap(12).Width(420).Children(func() {
			ui.Text(c, locale.T("保存远端 ACL")).FontSize(15).Bold()
			ui.Text(c, locale.T("保存会替换远端运行实例的 ACL 配置并立即生效。确定继续吗？")).FontSize(12.5)
			ui.Row(c).Gap(10).Children(func() {
				if ui.Button(c, locale.T("取消")).Clicked() {
					a.remoteACLSave = false
				}
				if ui.PrimaryButton(c, locale.T("确认保存远端 ACL")).Disabled(busy).Clicked() {
					a.saveRemoteACL(network)
					a.remoteACLSave = false
				}
			})
		})
	})
}

func (a *App) loadRemoteACL(network easytier.RemoteNetwork) {
	address := a.settings.RemoteRPC
	owner := address + "/" + network.ID
	get := a.getRemoteConfig
	if get == nil {
		get = easytier.GetRemoteConfig
	}
	a.runAsync("remote-acl:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		config, err := get(ctx, address, network)
		cancel()
		var acl *easytier.ACL
		if err == nil {
			acl, err = config.ACL()
		}
		return func() {
			if !a.remoteActive || a.settings.RemoteRPC != address || a.remoteSelected != network.ID {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			a.remoteACL = acl
			a.remoteACLKey = owner
			a.remoteACLMembers = ""
			if acl != nil {
				a.remoteACLMembers = strings.Join(acl.V1.Group.Members, ", ")
			}
		}
	})
}

func (a *App) saveRemoteACL(network easytier.RemoteNetwork) {
	var acl *easytier.ACL
	if a.remoteACL != nil {
		data, err := json.Marshal(a.remoteACL)
		if err != nil {
			a.notice = err.Error()
			return
		}
		if err := json.Unmarshal(data, &acl); err != nil {
			a.notice = err.Error()
			return
		}
		acl.V1.Group.Members = splitComma(a.remoteACLMembers)
	}
	if err := acl.Validate(); err != nil {
		a.notice = err.Error()
		return
	}
	address := a.settings.RemoteRPC
	owner := address + "/" + network.ID
	get := a.getRemoteConfig
	if get == nil {
		get = easytier.GetRemoteConfig
	}
	save := a.saveRemoteConfig
	if save == nil {
		save = easytier.SaveRemoteConfig
	}
	a.runAsync("remote-acl:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// Re-read so sections edited elsewhere since the load are not reverted.
		config, err := get(ctx, address, network)
		if err == nil {
			var updated easytier.RemoteConfig
			updated, err = config.WithACL(acl)
			if err == nil {
				err = save(ctx, address, updated)
			}
		}
		return func() {
			if !a.remoteActive || a.settings.RemoteRPC != address || a.remoteSelected != network.ID {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			a.remoteACL = acl
			a.remoteChecked = time.Time{}
			a.notice = locale.T("远端 ACL 已保存")
		}
	})
}

// aclEditor is the chain/rule/group editor shared by the local draft and
// the remote copy.
func (a *App) aclEditor(c *ui.Context, t *ui.Theme, acl *easytier.ACL, disabled bool) {
	deleteChain := -1
	for i := range acl.V1.Chains {
		chain := &acl.V1.Chains[i]
		card(c, func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, chain.Name).FontSize(14).Bold().Grow(1)
				if ui.Button(c, fmt.Sprintf(locale.T("删除链 #%d"), i+1)).Disabled(disabled).Clicked() {
					deleteChain = i
				}
			})
			toggleRow(c, t, locale.T("启用链"), locale.T("关闭后这条链不参与匹配"), &chain.Enabled, disabled)
			ui.Field(c, locale.T("链名称"), func() { ui.TextInput(c, &chain.Name).Disabled(disabled) })
			ui.Field(c, locale.T("链描述"), func() { ui.TextInput(c, &chain.Description).Disabled(disabled) })
			actionInput(c, locale.T("默认动作"), &chain.DefaultAction, disabled)
			deleteRule := -1
			for j := range chain.Rules {
				rule := chain.Rules[j]
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%d · %s", rule.Priority, rule.Name)).Grow(1)
					if ui.Button(c, fmt.Sprintf(locale.T("编辑 #%d/%d"), i+1, j+1)).Disabled(disabled).Clicked() {
						a.editACLRule(i, j)
					}
					// Reordering sets explicit priority so Core and UI agree.
					if ui.Button(c, fmt.Sprintf(locale.T("上移 #%d/%d"), i+1, j+1)).Disabled(disabled || j == 0).Clicked() {
						chain.Rules[j], chain.Rules[j-1] = chain.Rules[j-1], chain.Rules[j]
						for n := range chain.Rules {
							chain.Rules[n].Priority = uint32(len(chain.Rules) - n)
						}
						easytier.SortChainRules(chain)
					}
					if ui.Button(c, fmt.Sprintf(locale.T("删除 #%d/%d"), i+1, j+1)).Disabled(disabled).Clicked() {
						deleteRule = j
					}
				})
			}
			if deleteRule >= 0 {
				chain.Rules = append(chain.Rules[:deleteRule:deleteRule], chain.Rules[deleteRule+1:]...)
			}
			if ui.Button(c, locale.T("添加 ")+chain.Name+locale.T(" 规则")).Disabled(disabled).Clicked() {
				a.editACLRule(i, -1)
			}
		})
	}
	if deleteChain >= 0 {
		acl.V1.Chains = append(acl.V1.Chains[:deleteChain:deleteChain], acl.V1.Chains[deleteChain+1:]...)
	}
	card(c, func() {
		ui.Row(c).Gap(10).Children(func() {
			for chainType, name := range []string{"Inbound", "Outbound", "Forward"} {
				if ui.Button(c, fmt.Sprintf(locale.T("添加 %s 链"), name)).Disabled(disabled).Clicked() {
					acl.V1.Chains = append(acl.V1.Chains, easytier.NewChain(chainType+1, acl.V1.Chains))
				}
			}
		})
	})
	card(c, func() {
		ui.Text(c, locale.T("访问控制组")).Bold()
		ui.Text(c, locale.T("声明用于验证组成员的名称和密钥；成员列表填写本节点所属组。空密钥表示公开组。")).FontSize(12)
		ui.Text(c, locale.T("组名通过重命名按钮一次生效，所有规则的引用同步更新。")).FontSize(12).TextColor(t.TextMuted)
		target := "local/" + a.selected
		if a.remoteActive {
			target = "remote/" + a.remoteACLKey
		}
		if a.aclGroupNamesFor != target || len(a.aclGroupNames) != len(acl.V1.Group.Declares) {
			a.aclGroupNamesFor = target
			a.aclGroupNames = make([]string, len(acl.V1.Group.Declares))
			for i, group := range acl.V1.Group.Declares {
				a.aclGroupNames[i] = group.Name
			}
		}
		deleteGroup := -1
		for i := range acl.V1.Group.Declares {
			group := &acl.V1.Group.Declares[i]
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &a.aclGroupNames[i]).Label(fmt.Sprintf(locale.T("组名 %d"), i+1)).Disabled(disabled)
				ui.TextInput(c, &group.Secret).Label(fmt.Sprintf(locale.T("组密钥 %d"), i+1)).Disabled(disabled)
				if ui.Button(c, fmt.Sprintf(locale.T("重命名组 %d"), i+1)).Disabled(disabled).Clicked() {
					if err := easytier.RenameGroup(acl, group.Name, a.aclGroupNames[i]); err != nil {
						a.notice = err.Error()
					}
					a.aclGroupNames[i] = group.Name
				}
				if ui.Button(c, fmt.Sprintf(locale.T("移除组 %d"), i+1)).Disabled(disabled).Clicked() {
					deleteGroup = i
				}
			})
		}
		if deleteGroup >= 0 {
			acl.V1.Group.Declares = append(acl.V1.Group.Declares[:deleteGroup:deleteGroup], acl.V1.Group.Declares[deleteGroup+1:]...)
			a.aclGroupNames = append(a.aclGroupNames[:deleteGroup:deleteGroup], a.aclGroupNames[deleteGroup+1:]...)
		}
		if ui.Button(c, locale.T("声明组")).Disabled(disabled).Clicked() {
			acl.V1.Group.Declares = append(acl.V1.Group.Declares, easytier.ACLGroupIdentity{})
			a.aclGroupNames = append(a.aclGroupNames, "")
		}
		ui.Field(c, locale.T("本节点组成员（逗号分隔）"), func() { ui.TextInput(c, a.aclMembersBuffer()).Disabled(disabled) })
	})
}

// aclModals are the rule editor and the remove confirmation, shared by the
// local and remote ACL pages.
func (a *App) aclModals(c *ui.Context, t *ui.Theme) {
	a.aclRuleModal(c, t)
	ui.Modal(c, &a.removeACLOpen, func() {
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locale.T("移除 ACL 将解除所有访问限制，确定继续吗？"))
			if ui.Button(c, locale.T("取消")).Clicked() {
				a.removeACLOpen = false
			}
			if ui.PrimaryButton(c, locale.T("确认移除 ACL")).Clicked() {
				if a.remoteActive {
					a.remoteACL = nil
				} else {
					a.draft().config.ACL = nil
				}
				a.removeACLOpen = false
			}
		})
	})
}

func actionInput(c *ui.Context, label string, value *int, disabled bool) {
	choice := *value - 1
	ui.Field(c, label, func() { ui.Segmented(c, &choice, "Allow", "Drop").Disabled(disabled) })
	*value = choice + 1
}

// Text inputs keep the trailing comma while typing. The editor uses text
// buffers and converts to arrays only on Apply.
type aclRuleDraft struct {
	value                                                                          easytier.ACLRule
	ports, sourcePorts, sourceIPs, destinationIPs, sourceGroups, destinationGroups string
}

func (a *App) editACLRule(chain, rule int) {
	a.aclChainIndex, a.aclRuleIndex = chain, rule
	value := easytier.ACLRule{Name: locale.T("新规则"), Enabled: true, Protocol: 5, Action: 1, Priority: 1000}
	if rule >= 0 {
		value = a.aclEditing().V1.Chains[chain].Rules[rule]
	}
	a.aclRule = aclRuleDraft{value: value, ports: strings.Join(value.Ports, ", "), sourcePorts: strings.Join(value.SourcePorts, ", "),
		sourceIPs: strings.Join(value.SourceIPs, ", "), destinationIPs: strings.Join(value.DestinationIPs, ", "),
		sourceGroups: strings.Join(value.SourceGroups, ", "), destinationGroups: strings.Join(value.DestinationGroups, ", ")}
	a.aclRuleOpen, a.aclErr = true, ""
}

func (a *App) applyACLRule() {
	rule := a.aclRule.value
	rule.Ports, rule.SourcePorts = splitComma(a.aclRule.ports), splitComma(a.aclRule.sourcePorts)
	rule.SourceIPs, rule.DestinationIPs = splitComma(a.aclRule.sourceIPs), splitComma(a.aclRule.destinationIPs)
	rule.SourceGroups, rule.DestinationGroups = splitComma(a.aclRule.sourceGroups), splitComma(a.aclRule.destinationGroups)
	acl := easytier.DefaultACL()
	acl.V1.Chains[0].Rules = []easytier.ACLRule{rule}
	if err := acl.Validate(); err != nil {
		a.aclErr = err.Error()
		return
	}
	chain := &a.aclEditing().V1.Chains[a.aclChainIndex]
	if a.aclRuleIndex < 0 {
		chain.Rules = append(chain.Rules, rule)
	} else {
		chain.Rules[a.aclRuleIndex] = rule
	}
	// A hand-edited priority changes the match order; keep the list in it.
	easytier.SortChainRules(chain)
	a.aclRuleOpen = false
}

func (a *App) aclRuleModal(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &a.aclRuleOpen, func() {
		ui.Column(c).Padding(20).Gap(8).Width(640).Children(func() {
			ui.Text(c, locale.T("ACL 规则")).Bold()
			ui.Scroll(c).Height(390).Children(func() {
				ui.Form(c, func() {
					rule := &a.aclRule.value
					ui.Field(c, locale.T("规则名称"), func() { ui.TextInput(c, &rule.Name) })
					ui.Field(c, locale.T("描述"), func() { ui.TextInput(c, &rule.Description) })
					ui.Field(c, locale.T("优先级"), func() { uint32Input(c, &rule.Priority, 65535) })
					ui.Field(c, locale.T("启用规则"), func() { ui.Switch(c, &rule.Enabled) })
					protocol := rule.Protocol - 1
					ui.Field(c, locale.T("协议"), func() { ui.Segmented(c, &protocol, "TCP", "UDP", "ICMP", "ICMPv6", "Any") })
					rule.Protocol = protocol + 1
					for _, field := range []struct {
						label string
						text  *string
					}{
						{locale.T("源 IP/CIDR"), &a.aclRule.sourceIPs}, {locale.T("目的 IP/CIDR"), &a.aclRule.destinationIPs},
						{locale.T("源端口/范围"), &a.aclRule.sourcePorts}, {locale.T("目的端口/范围"), &a.aclRule.ports},
						{locale.T("源组"), &a.aclRule.sourceGroups}, {locale.T("目的组"), &a.aclRule.destinationGroups},
					} {
						ui.Field(c, field.label, func() { ui.TextInput(c, field.text).Placeholder(locale.T("逗号分隔；空表示任意")) })
					}
					actionInput(c, locale.T("规则动作"), &rule.Action, false)
					ui.Field(c, "Stateful", func() { ui.Switch(c, &rule.Stateful) })
					ui.Field(c, locale.T("PPS（0 不限）"), func() { uint32Input(c, &rule.RateLimit, 1<<32-1) })
					ui.Field(c, "Burst", func() { uint32Input(c, &rule.BurstLimit, 1<<32-1) })
				})
			})
			if a.aclErr != "" {
				ui.Text(c, a.aclErr).TextColor(t.Danger)
			}
			ui.Row(c).Gap(10).Children(func() {
				if ui.Button(c, locale.T("取消规则")).Clicked() {
					a.aclRuleOpen = false
				}
				if ui.PrimaryButton(c, locale.T("应用规则")).Clicked() {
					a.applyACLRule()
				}
			})
		})
	})
}

func splitComma(text string) []string {
	var values []string
	for _, part := range strings.Split(text, ",") {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}
func uint32Input(c *ui.Context, value *uint32, max float64) {
	v := float64(*value)
	ui.NumberInput(c, &v, 0, max, 1)
	*value = uint32(v)
}
