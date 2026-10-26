package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) portalPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if a.remoteActive {
		a.remotePortalPage(c, t, snap)
		return
	}
	config := &a.draft().config
	running := a.ctl().Running()
	card(c, func() {
		ui.Text(c, "WireGuard VPN Portal").Bold()
		ui.Text(c, locale.T("v2.6.4 使用共享配置，v2.7/main 使用逐设备模式。客户端配置和二维码含私钥，请妥善保管。")).FontSize(12).TextColor(t.Warning)
		if config.VPNPortal == nil {
			if ui.Button(c, locale.T("启用 VPN Portal")).Disabled(running).Clicked() {
				config.VPNPortal = &easytier.VPNPortal{ClientCIDR: "10.14.14.0/24", Listen: "0.0.0.0:51820"}
			}
			return
		}
		toggleRow(c, t, locale.T("逐设备模式（v2.7/main）"), locale.T("与 v2.6.4 schema 不兼容，连接前会检查 Core 版本"), &config.VPNPortal.DeviceMode, running)
		if !config.VPNPortal.DeviceMode {
			ui.Field(c, locale.T("客户端网段"), func() { ui.TextInput(c, &config.VPNPortal.ClientCIDR).Disabled(running) })
		}
		ui.Field(c, locale.T("Portal 监听"), func() { ui.TextInput(c, &config.VPNPortal.Listen).Disabled(running) })
		if ui.Button(c, locale.T("关闭 VPN Portal")).Disabled(running).Clicked() {
			config.VPNPortal = nil
		}
		if ui.PrimaryButton(c, locale.T("保存 Portal 设置")).Disabled(running).Clicked() {
			a.saveSettings(c)
		}
	})
	if snap.Phase != easytier.PhaseRunning || config.VPNPortal == nil {
		return
	}
	card(c, func() {
		if ui.Button(c, locale.T("读取客户端配置")).Disabled(a.asyncBusy("portal-read:" + a.portalContextKey())).Clicked() {
			a.readPortalIntoState()
		}
		if a.portalOwner != a.portalContextKey() {
			return
		}
		a.portalClientConfig(c, t, snap)
	})
	a.pollPortalDevices(c)
	if a.portalOwner == a.portalContextKey() {
		card(c, func() {
			a.credentialControls(c, t)
		})
	}
}

// remotePortalPage is the Portal page for the selected remote instance:
// reading and per-device changes go to the remote endpoint; the Portal's
// own configuration stays with the remote page's full config editor.
func (a *App) remotePortalPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if _, ok := a.remoteSelectedNetwork(); !ok {
		a.emptyState(c, t, locale.T("请先在左侧选择远端实例"))
		return
	}
	card(c, func() {
		ui.Text(c, "WireGuard VPN Portal").Bold()
		ui.Text(c, locale.T("读取的是远端实例的 Portal；逐设备增删需要远端 Core 支持（v2.7/main）。客户端配置和二维码含私钥，请妥善保管。")).FontSize(12).TextColor(t.Warning)
		if snap.Phase != easytier.PhaseRunning {
			ui.Text(c, locale.T("远端实例未在运行，无法读取 Portal。")).FontSize(12).TextColor(t.TextMuted)
			return
		}
		if ui.PrimaryButton(c, locale.T("读取远端 Portal 配置")).Disabled(a.asyncBusy("portal-read:" + a.portalContextKey())).Clicked() {
			a.readPortalIntoState()
		}
	})
	if a.portalOwner != a.portalContextKey() {
		return
	}
	card(c, func() {
		a.portalClientConfig(c, t, snap)
	})
	a.pollPortalDevices(c)
	card(c, func() {
		a.credentialControls(c, t)
	})
}

// portalContextKey identifies whose portalInfo state is shown, so switching
// networks never presents another instance's client config.
func (a *App) portalContextKey() string {
	if a.remoteActive {
		return "remote " + a.settings.RemoteRPC + " " + a.remoteSelected
	}
	return a.selected
}

func (a *App) readPortalIntoState() {
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
			} else {
				a.portalInfo = info
				a.portalOwner = owner
			}
		}
	})
}

// portalEndpointSuggestions derives candidate public endpoints from the
// current instance's listeners: the local profile's configured listeners and
// mapped listeners, or the listeners a remote instance reports. Wildcard and
// loopback binds never make an endpoint.
func (a *App) portalEndpointSuggestions(snap easytier.Snapshot) []string {
	if a.portalInfo.Listener == "" {
		return nil
	}
	var listeners []string
	if a.remoteActive {
		listeners = snap.Node.Listeners
	} else {
		config := a.draft().config
		listeners = append(append([]string{}, config.Listeners...), config.MappedListeners...)
	}
	return easytier.SuggestPortalEndpoints(listeners, a.portalInfo.Listener)
}

func (a *App) portalReader() func(context.Context) (easytier.PortalInfo, error) {
	if a.portalPoll != nil {
		return a.portalPoll
	}
	if a.remoteActive {
		address := a.settings.RemoteRPC
		network, ok := a.remoteSelectedNetwork()
		return func(ctx context.Context) (easytier.PortalInfo, error) {
			if !ok {
				return easytier.PortalInfo{}, fmt.Errorf("%s", locale.T("请先在左侧选择远端实例"))
			}
			return easytier.ReadRemoteVPNPortal(ctx, address, network)
		}
	}
	address := a.ctl().Snapshot().Portal
	return func(ctx context.Context) (easytier.PortalInfo, error) {
		return easytier.ReadVPNPortalNative(ctx, address)
	}
}

func (a *App) readPortal(ctx context.Context) (easytier.PortalInfo, error) {
	return a.portalReader()(ctx)
}

func (a *App) portalDevicePatcher() func(context.Context, easytier.PortalDevice, bool) error {
	if a.remoteActive {
		address := a.settings.RemoteRPC
		network, ok := a.remoteSelectedNetwork()
		return func(ctx context.Context, device easytier.PortalDevice, remove bool) error {
			if !ok {
				return fmt.Errorf("%s", locale.T("请先在左侧选择远端实例"))
			}
			return easytier.PatchRemotePortalDevice(ctx, address, network, device, remove)
		}
	}
	address := a.ctl().Snapshot().Portal
	return func(ctx context.Context, device easytier.PortalDevice, remove bool) error {
		return easytier.PatchPortalDevice(ctx, address, device, remove)
	}
}

func (a *App) patchPortalDevice(ctx context.Context, device easytier.PortalDevice, remove bool) error {
	return a.portalDevicePatcher()(ctx, device, remove)
}

// portalClientConfig draws the client config, QR and device controls of the
// instance whose portalInfo is loaded; it is shared by local and remote.
func (a *App) portalClientConfig(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if a.portalInfo.Listener != "" {
		a.portalDeviceControls(c, t)
	}
	ui.TextInput(c, &a.portalEndpoint).Label(locale.T("客户端连接 Endpoint")).Placeholder(locale.T("公网主机或域名:51820"))
	// Candidate endpoints come from the instance's own listeners; the user
	// picks one explicitly, the field is never overwritten.
	for _, endpoint := range a.portalEndpointSuggestions(snap) {
		if ui.Button(c, locale.T("使用建议 ")+endpoint).Clicked() {
			a.portalEndpoint = endpoint
		}
	}
	text := a.portalInfo.ClientConfig
	for _, device := range a.portalInfo.Clients {
		if device.Name == a.portalDeviceSelected {
			text = device.ClientConfig
		}
	}
	if text == "" {
		return
	}
	if a.portalEndpoint != "" {
		updated, err := easytier.SetPortalEndpoint(text, a.portalEndpoint)
		if err != nil {
			ui.Text(c, err.Error()).TextColor(t.Danger)
		} else {
			text = updated
		}
	}
	ui.TextArea(c, &text).Height(240).Label(locale.T("WireGuard 客户端配置"))
	if a.portalQRText != text {
		a.portalQR = nil
		a.portalQRText = text
		data, err := easytier.PortalQRCode(text)
		if err != nil {
			a.portalQRError = err.Error()
		} else {
			a.portalQR, a.portalQRError = nil, ""
			a.portalQR, err = ui.DecodeBitmap(data)
			if err != nil {
				a.portalQRError = err.Error()
			}
		}
	}
	if a.portalQR != nil {
		ui.Image(c, a.portalQR.Pixelated()).Size(240, 240)
	}
	if a.portalQRError != "" {
		ui.Text(c, locale.T("二维码生成失败：")+a.portalQRError).TextColor(t.Danger)
	}
	if ui.Button(c, locale.T("复制客户端配置")).Clicked() {
		c.WriteClipboard(text)
		c.Toast(locale.T("已复制含私钥的配置"))
	}
	if ui.Button(c, locale.T("下载客户端配置")).Clicked() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: locale.T("保存 WireGuard 配置"), DefaultPath: "wireguard.conf"})
		if err != nil {
			a.notice = err.Error()
		} else if path != "" {
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				a.notice = err.Error()
			}
		}
	}
	for _, client := range a.portalInfo.ConnectedClients {
		ui.Text(c, locale.T("已连接：")+client)
	}
}
