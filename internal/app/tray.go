package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

type trayNetworkModel struct {
	id    string
	name  string
	phase easytier.Phase
	peers int
	ip    string
}

type trayViewModel struct {
	phase    easytier.Phase
	status   string
	detail   string
	networks []trayNetworkModel
}

func (a *App) trayModel() trayViewModel {
	model := trayViewModel{phase: easytier.PhaseStopped}
	for _, profile := range a.profiles {
		snap := a.mgr.Snapshot(profile.ID)
		network := trayNetworkModel{
			id: profile.ID, name: profile.DisplayName(), phase: snap.Phase,
			peers: peerCount(snap), ip: snap.Node.IPv4,
		}
		model.networks = append(model.networks, network)
		switch {
		case snap.Phase == easytier.PhaseRunning:
			model.phase = easytier.PhaseRunning
		case (snap.Phase == easytier.PhaseStarting || snap.Phase == easytier.PhaseStopping) && model.phase != easytier.PhaseRunning:
			model.phase = easytier.PhaseStarting
		case snap.Phase == easytier.PhaseFailed && model.phase != easytier.PhaseRunning && model.phase != easytier.PhaseStarting:
			model.phase = easytier.PhaseFailed
		}
	}

	selected := a.selectedProfile()
	selectedSnap := a.mgr.Snapshot(selected.ID)
	if selectedSnap.Phase != easytier.PhaseRunning {
		for _, network := range model.networks {
			if network.phase == easytier.PhaseRunning {
				selectedSnap = a.mgr.Snapshot(network.id)
				selected, _ = a.profile(network.id)
				break
			}
		}
	}
	switch {
	case selectedSnap.Phase == easytier.PhaseRunning:
		model.status = fmt.Sprintf(locale.T("已连接到 %s"), selected.DisplayName())
		model.detail = fmt.Sprintf(locale.T("本机 %s · %d 个在线节点"), selectedSnap.Node.IPv4, peerCount(selectedSnap))
	case model.phase == easytier.PhaseStarting || model.phase == easytier.PhaseFailed:
		model.status = statusLabel(model.phase)
		model.detail = selectedSnap.Err
	default:
		model.status = statusLabel(easytier.PhaseStopped)
		model.detail = locale.T("未加入任何网络")
	}
	return model
}

func (a *App) profile(id string) (easytier.Profile, bool) {
	for _, profile := range a.profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return easytier.Profile{}, false
}

func trayKey(model trayViewModel) string {
	var key strings.Builder
	key.WriteString(locale.Language())
	key.WriteString(model.status)
	key.WriteString(model.detail)
	for _, network := range model.networks {
		fmt.Fprintf(&key, "|%s:%s:%s", network.id, network.name, network.phase)
	}
	return key.String()
}

func (a *App) AttachTray(tray *mygo.Tray) {
	a.tray = tray
	a.RefreshTray()
}

func (a *App) RefreshTray() {
	if a.tray == nil {
		return
	}
	model := a.trayModel()
	key := trayKey(model)
	if key == a.trayKey {
		return
	}
	a.trayKey = key
	items := []*mygo.MenuItem{
		{Label: model.status, Disabled: true},
		{Label: model.detail, Disabled: true},
		mygo.Separator(),
	}
	for _, network := range model.networks {
		network := network
		items = append(items, &mygo.MenuItem{
			Label:    network.name + " · " + statusLabel(network.phase),
			Disabled: network.phase == easytier.PhaseStarting || network.phase == easytier.PhaseStopping,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				a.toggleTrayNetwork(network.id)
			},
		})
	}
	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{Label: locale.T("显示 TierGo"), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showTrayPage("overview")
		}},
		&mygo.MenuItem{Label: locale.T("打开日志"), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showTrayPage("logs")
		}},
		mygo.Separator(),
		&mygo.MenuItem{Label: locale.T("退出 TierGo"), Click: func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.Quit()
		}},
	)
	a.tray.SetMenu(mygo.NewMenu(items))
	a.tray.SetToolTip(model.status + "\n" + model.detail)
	if model.phase != a.trayPhase {
		if err := a.tray.SetIcon(TrayIconForPhase(model.phase), true); err != nil {
			a.notice = err.Error()
		}
		a.trayPhase = model.phase
	}
}

func (a *App) toggleTrayNetwork(id string) {
	if a.mgr.Running(id) {
		a.mgr.Stop(id)
		a.setNetworkEnabled(id, false)
		return
	}
	if _, ok := a.profile(id); !ok {
		return
	}
	a.selected = id
	a.remoteActive = false
	a.page = "overview"
	a.rememberSession()
	a.connect()
	a.showTrayPage("overview")
}

func (a *App) showTrayPage(page string) {
	a.remoteActive = false
	a.page = page
	if a.win != nil {
		a.win.Show()
		a.win.Focus()
	}
}
