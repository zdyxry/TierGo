package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// credentialControls manages temporary credentials of the current instance
// (local or remote, same context as the portal). CredentialManageRpc exists
// only in main; against v2.6.4 the call fails and the error is shown.
func (a *App) credentialControls(c *ui.Context, t *ui.Theme) {
	ui.Text(c, locale.T("临时凭据")).Bold()
	ui.Text(c, locale.T("凭据用于临时设备短期接入本网络，到期自动失效；需要 v2.7/main 的 Core。")).FontSize(12).TextColor(t.TextMuted)
	owner := a.portalContextKey()
	if a.credentialOwner != owner {
		a.credentialOwner = owner
		a.credentialList = nil
		a.credentialErr = ""
		a.credentialHasGenerated = false
	}
	if a.credentialTTL == 0 {
		a.credentialTTL = 86400
	}
	ui.Form(c, func() {
		ui.Field(c, locale.T("有效期（秒）"), func() {
			ui.NumberInput(c, &a.credentialTTL, 60, 86400*365, 60)
		})
		ui.Field(c, locale.T("凭据 ACL 组（逗号分隔）"), func() {
			ui.TextInput(c, &a.credentialGroups)
		})
	})
	toggleRow(c, t, locale.T("允许中继"), locale.T("允许其他节点经该凭据设备中继"), &a.credentialAllowRelay, false)
	toggleRow(c, t, locale.T("可复用"), locale.T("取消勾选则同一凭据同时只允许一台设备在线"), &a.credentialReusable, false)
	busy := a.asyncBusy("credentials:" + owner)
	if ui.PrimaryButton(c, locale.T("生成凭据")).Disabled(busy).Clicked() {
		a.generateCredential()
	}
	if a.credentialHasGenerated {
		ui.Text(c, locale.T("凭据密钥只显示一次，请立即复制保存。")).FontSize(12).TextColor(t.Warning)
		ui.Text(c, locale.T("凭据 ID：")+a.credentialGenerated.ID).FontSize(12.5)
		secret := a.credentialGenerated.Secret
		ui.TextArea(c, &secret).Height(56).Label(locale.T("凭据密钥"))
		if ui.Button(c, locale.T("复制凭据密钥")).Clicked() {
			c.WriteClipboard(a.credentialGenerated.Secret)
			c.Toast(locale.T("已复制凭据密钥"))
		}
	}
	if ui.Button(c, locale.T("刷新凭据列表")).Disabled(busy).Clicked() {
		a.listCredentials()
	}
	if a.credentialErr != "" {
		ui.Text(c, a.credentialErr).FontSize(12).TextColor(t.Danger)
	}
	now := time.Now()
	for _, credential := range a.credentialList {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			status := locale.T("有效")
			color := t.Success
			if credential.ExpiryUnix > 0 && time.Unix(credential.ExpiryUnix, 0).Before(now) {
				status = locale.T("已过期")
				color = t.TextMuted
			}
			ui.Badge(c, status).Background(color.Alpha(0.14)).TextColor(color)
			ui.Text(c, credentialLabel(credential)).FontSize(12.5).Grow(1).SingleLine()
			if ui.Button(c, locale.T("吊销凭据")).Clicked() {
				a.credentialRevoke = credential.ID
			}
		})
	}
	revoking := a.credentialRevoke != ""
	ui.Modal(c, &revoking, func() {
		ui.Column(c).Padding(20).Gap(10).Children(func() {
			ui.Text(c, locale.T("吊销后使用该凭据的临时设备将被断开：")+a.credentialRevoke)
			if ui.Button(c, locale.T("取消")).Clicked() {
				a.credentialRevoke = ""
			}
			if ui.PrimaryButton(c, locale.T("确认吊销")).Disabled(busy).Clicked() {
				a.revokeCredential(owner, a.credentialRevoke)
				a.credentialRevoke = ""
			}
		})
	})
	if !revoking {
		a.credentialRevoke = ""
	}
}

// credentialLabel summarizes one credential for the list row.
func credentialLabel(credential easytier.Credential) string {
	parts := []string{credential.ID}
	if len(credential.Groups) > 0 {
		parts = append(parts, strings.Join(credential.Groups, ","))
	}
	if credential.ExpiryUnix > 0 {
		parts = append(parts, time.Unix(credential.ExpiryUnix, 0).Format("2006-01-02 15:04"))
	}
	if credential.Reusable {
		parts = append(parts, locale.T("可复用"))
	}
	return strings.Join(parts, " · ")
}

func (a *App) generateCredential() {
	owner := a.portalContextKey()
	generate := a.credentialGenerator()
	request := easytier.CredentialRequest{
		Groups:     splitComma(a.credentialGroups),
		AllowRelay: a.credentialAllowRelay,
		TTLSeconds: int64(a.credentialTTL),
		Reusable:   a.credentialReusable,
	}
	a.runAsync("credentials:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		generated, err := generate(ctx, request)
		cancel()
		return func() {
			if owner != a.portalContextKey() {
				return
			}
			if err != nil {
				a.credentialErr = err.Error()
				return
			}
			a.credentialErr = ""
			a.credentialGenerated = generated
			a.credentialHasGenerated = true
			a.listCredentials()
		}
	})
}

func (a *App) listCredentials() {
	owner := a.portalContextKey()
	list := a.credentialLister()
	a.runAsync("credentials:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		credentials, err := list(ctx)
		cancel()
		return func() {
			if owner != a.portalContextKey() {
				return
			}
			if err != nil {
				a.credentialErr = err.Error()
				return
			}
			a.credentialErr = ""
			a.credentialList = credentials
		}
	})
}

func (a *App) revokeCredential(owner, id string) {
	revoke := a.credentialRevoker()
	a.runAsync("credentials:"+owner, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := revoke(ctx, id)
		cancel()
		return func() {
			if owner != a.portalContextKey() {
				return
			}
			if err != nil {
				a.notice = err.Error()
			} else {
				a.listCredentials()
			}
		}
	})
}

// The seams default to the real RPCs against the current instance; tests
// replace them.
func (a *App) credentialGenerator() func(context.Context, easytier.CredentialRequest) (easytier.GeneratedCredential, error) {
	if a.credentialGenerateFn != nil {
		return a.credentialGenerateFn
	}
	if a.remoteActive {
		address := a.settings.RemoteRPC
		network, ok := a.remoteSelectedNetwork()
		return func(ctx context.Context, request easytier.CredentialRequest) (easytier.GeneratedCredential, error) {
			if !ok {
				return easytier.GeneratedCredential{}, fmt.Errorf("%s", locale.T("请先在左侧选择远端实例"))
			}
			return easytier.GenerateRemoteCredential(ctx, address, network, request)
		}
	}
	address := a.ctl().Snapshot().Portal
	return func(ctx context.Context, request easytier.CredentialRequest) (easytier.GeneratedCredential, error) {
		return easytier.GenerateCredential(ctx, address, request)
	}
}

func (a *App) credentialRevoker() func(context.Context, string) error {
	if a.credentialRevokeFn != nil {
		return a.credentialRevokeFn
	}
	if a.remoteActive {
		address := a.settings.RemoteRPC
		network, ok := a.remoteSelectedNetwork()
		return func(ctx context.Context, id string) error {
			if !ok {
				return fmt.Errorf("%s", locale.T("请先在左侧选择远端实例"))
			}
			return easytier.RevokeRemoteCredential(ctx, address, network, id)
		}
	}
	address := a.ctl().Snapshot().Portal
	return func(ctx context.Context, id string) error {
		return easytier.RevokeCredential(ctx, address, id)
	}
}

func (a *App) credentialLister() func(context.Context) ([]easytier.Credential, error) {
	if a.credentialListFn != nil {
		return a.credentialListFn
	}
	if a.remoteActive {
		address := a.settings.RemoteRPC
		network, ok := a.remoteSelectedNetwork()
		return func(ctx context.Context) ([]easytier.Credential, error) {
			if !ok {
				return nil, fmt.Errorf("%s", locale.T("请先在左侧选择远端实例"))
			}
			return easytier.ListRemoteCredentials(ctx, address, network)
		}
	}
	address := a.ctl().Snapshot().Portal
	return func(ctx context.Context) ([]easytier.Credential, error) {
		return easytier.ListCredentials(ctx, address)
	}
}
