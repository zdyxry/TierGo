package app

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

type portalPollResult struct {
	owner string
	info  easytier.PortalInfo
	err   error
}

// pollPortalDevices re-reads the device list every few seconds while the
// Portal page shows a loaded device-mode portal, so online states update
// without the manual refresh button. It mirrors refreshRemote: only the UI
// thread mutates App, background reads arrive over a channel, and results
// for another instance context are discarded.
func (a *App) pollPortalDevices(c *ui.Context) {
	if a.portalPollPending != nil {
		select {
		case result := <-a.portalPollPending:
			a.portalPollPending = nil
			if result.owner == a.portalContextKey() && result.err == nil {
				a.applyPortalInfo(result.info)
			}
		default:
		}
	}
	c.After(500 * time.Millisecond)
	if a.portalPollPending != nil || time.Since(a.portalPollAt) < 3*time.Second {
		return
	}
	// Only poll a portal whose info the page is actually showing.
	if a.portalOwner == "" || a.portalOwner != a.portalContextKey() {
		return
	}
	if a.currentSnapshot().Phase != easytier.PhaseRunning {
		return
	}
	a.portalPollAt = time.Now()
	owner := a.portalOwner
	read := a.portalPoll
	if read == nil {
		read = a.readPortal
	}
	results := make(chan portalPollResult, 1)
	a.portalPollPending = results
	win := a.win
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := read(ctx)
		results <- portalPollResult{owner: owner, info: info, err: err}
		if win != nil {
			win.Invalidate()
		}
	}()
}

// applyPortalInfo shows a freshly read portal; for the local device-mode
// profile it also persists client changes, so device additions made through
// the running core (here or elsewhere) survive the next launch. The profile
// is written only when the client list actually changed — the periodic
// refresh must not rewrite it every tick.
func (a *App) applyPortalInfo(info easytier.PortalInfo) {
	a.portalInfo = info
	if a.remoteActive {
		return
	}
	portal := a.draft().config.VPNPortal
	if portal == nil || !portal.DeviceMode {
		return
	}
	if portalDevicesEqual(portal.Clients, info.Clients) {
		return
	}
	portal.Clients = nil
	for _, client := range info.Clients {
		portal.Clients = append(portal.Clients, client.PortalDevice)
	}
	profile := a.selectedProfile()
	profile.Config = a.connectionConfig()
	if err := easytier.SaveProfile(profile); err != nil {
		a.notice = err.Error()
		return
	}
	for i := range a.profiles {
		if a.profiles[i].ID == profile.ID {
			a.profiles[i] = profile
		}
	}
}

func portalDevicesEqual(saved []easytier.PortalDevice, loaded []easytier.PortalDeviceInfo) bool {
	if len(saved) != len(loaded) {
		return false
	}
	for i := range saved {
		if saved[i].Name != loaded[i].Name || saved[i].VirtualIP != loaded[i].VirtualIP ||
			strings.Join(saved[i].Groups, ",") != strings.Join(loaded[i].Groups, ",") {
			return false
		}
	}
	return true
}
