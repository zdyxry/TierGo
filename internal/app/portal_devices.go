package app

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func portalDeviceState(state int) string {
	switch state {
	case 1:
		return locale.T("离线")
	case 2:
		return locale.T("连接中")
	case 3:
		return locale.T("在线")
	case 4:
		return locale.T("错误")
	default:
		return locale.T("未知")
	}
}

// portalClientCIDR is the network device addresses are suggested from: the
// configured client CIDR for the local profile, else the network inferred
// from an existing device (device mode clears the shared CIDR, and a remote
// instance's config is not loaded here).
func (a *App) portalClientCIDR() string {
	if !a.remoteActive {
		if portal := a.draft().config.VPNPortal; portal != nil && portal.ClientCIDR != "" {
			return portal.ClientCIDR
		}
	}
	for _, client := range a.portalInfo.Clients {
		if prefix, err := netip.ParsePrefix(client.VirtualIP); err == nil {
			return prefix.Masked().String()
		}
	}
	return ""
}

// portalDeclaredGroups names the ACL groups the current instance's
// configuration declares; empty when no ACL is loaded (a remote instance's
// ACL arrives on the ACL page).
func (a *App) portalDeclaredGroups() []string {
	acl := a.draft().config.ACL
	if a.remoteActive {
		acl = a.remoteACL
	}
	if acl == nil {
		return nil
	}
	names := make([]string, 0, len(acl.V1.Group.Declares))
	for _, group := range acl.V1.Group.Declares {
		if group.Name != "" {
			names = append(names, group.Name)
		}
	}
	return names
}

// mergeDeviceGroups combines the checked declared groups (in declaration
// order) with the manually typed ones, dropping duplicates.
func mergeDeviceGroups(declared []string, checked map[string]bool, manual string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range declared {
		if checked[name] && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	for _, name := range splitComma(manual) {
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}

func (a *App) portalDeviceControls(c *ui.Context, t *ui.Theme) {
	ui.Text(c, locale.T("设备列表（页面打开时每 3 秒自动刷新在线状态）")).Bold()
	owner := a.portalContextKey()
	busy := a.asyncBusy("portal-device:" + owner)
	for _, device := range a.portalInfo.Clients {
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, fmt.Sprintf("%s · %s · %s", device.Name, device.VirtualIP, portalDeviceState(device.State))).Clicked() {
				a.portalDeviceSelected = device.Name
			}
			if ui.Button(c, locale.T("删除设备 ")+device.Name).Clicked() {
				a.portalDeviceDelete = device.Name
			}
		})
		if device.Error != "" {
			ui.Text(c, device.Error).TextColor(t.Danger)
		}
	}
	ui.Field(c, locale.T("设备名称"), func() { ui.TextInput(c, &a.portalDeviceName).Placeholder("laptop") })
	// Suggest the first free address of the client network; the suggestion
	// never overwrites what the user typed.
	used := make([]string, 0, len(a.portalInfo.Clients))
	for _, device := range a.portalInfo.Clients {
		used = append(used, device.VirtualIP)
	}
	suggestion := ""
	if cidr := a.portalClientCIDR(); cidr != "" {
		if suggested, err := easytier.SuggestDeviceIP(cidr, used); err == nil {
			suggestion = suggested
		}
	}
	ipPlaceholder := "10.14.14.2/24"
	if suggestion != "" {
		ipPlaceholder = suggestion
	}
	ui.Field(c, locale.T("设备虚拟 IP/CIDR"), func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.portalDeviceIP).Placeholder(ipPlaceholder).Grow(1)
			if suggestion != "" {
				if ui.Button(c, locale.T("使用建议地址")).Clicked() {
					a.portalDeviceIP = suggestion
				}
			}
		})
	})
	// Declared ACL groups become checkboxes; the text field stays for names
	// the configuration does not declare.
	declared := a.portalDeclaredGroups()
	if len(declared) > 0 {
		if a.portalDeviceChecked == nil {
			a.portalDeviceChecked = map[string]bool{}
		}
		ui.Text(c, locale.T("设备 ACL 组（勾选已声明的组）")).FontSize(12.5)
		ui.Row(c).Gap(12).Children(func() {
			for _, name := range declared {
				checked := a.portalDeviceChecked[name]
				ui.Checkbox(c, &checked, name)
				a.portalDeviceChecked[name] = checked
			}
		})
		ui.Field(c, locale.T("其他组（逗号分隔，可选）"), func() { ui.TextInput(c, &a.portalDeviceGroups) })
	} else {
		ui.Text(c, locale.T("当前配置未声明 ACL 组，可在访问控制页声明后直接勾选。")).FontSize(12).TextColor(t.TextMuted)
		ui.Field(c, locale.T("设备 ACL 组（逗号分隔）"), func() { ui.TextInput(c, &a.portalDeviceGroups) })
	}
	if ui.Button(c, locale.T("添加 WireGuard 设备")).Disabled(busy).Clicked() {
		device := easytier.PortalDevice{
			Name:      a.portalDeviceName,
			VirtualIP: a.portalDeviceIP,
			Groups:    mergeDeviceGroups(declared, a.portalDeviceChecked, a.portalDeviceGroups),
		}
		a.patchPortalDeviceAsync(owner, device, false)
	}
	open := a.portalDeviceDelete != ""
	ui.Modal(c, &open, func() {
		ui.Column(c).Padding(20).Gap(10).Children(func() {
			ui.Text(c, locale.T("删除设备将撤销其接入权限：")+a.portalDeviceDelete)
			if ui.Button(c, locale.T("取消删除设备")).Clicked() {
				a.portalDeviceDelete = ""
			}
			if ui.PrimaryButton(c, locale.T("确认删除设备")).Disabled(busy).Clicked() {
				a.patchPortalDeviceAsync(owner, easytier.PortalDevice{Name: a.portalDeviceDelete}, true)
				a.portalDeviceDelete = ""
			}
		})
	})
	if !open {
		a.portalDeviceDelete = ""
	}
}

func (a *App) patchPortalDeviceAsync(owner string, device easytier.PortalDevice, remove bool) {
	patch := a.portalDevicePatcher()
	a.runAsync("portal-device:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := patch(ctx, device, remove)
		cancel()
		return func() {
			if owner != a.portalContextKey() {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			if !remove {
				a.portalDeviceSelected = device.Name
				a.portalDeviceChecked = nil
			}
			a.refreshPortalDevices()
		}
	})
}

func (a *App) refreshPortalDevices() {
	owner := a.portalContextKey()
	read := a.portalReader()
	a.runAsync("portal-read:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		info, err := read(ctx)
		cancel()
		return func() {
			if owner != a.portalContextKey() {
				return
			}
			if err != nil {
				a.notice = err.Error()
				return
			}
			a.applyPortalInfo(info)
		}
	})
}
