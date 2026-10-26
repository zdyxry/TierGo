package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

type remotePollResult struct {
	address, selected string
	networks          []easytier.RemoteNetwork
	snapshot          easytier.Snapshot
	err               error
}

func pollRemote(ctx context.Context, address, selected string) remotePollResult {
	result := remotePollResult{address: address, selected: selected}
	result.networks, result.err = easytier.ListRemoteNetworks(ctx, address)
	if result.err != nil {
		return result
	}
	found := false
	for _, network := range result.networks {
		if network.ID == selected {
			found = true
		}
	}
	if !found && len(result.networks) > 0 {
		result.selected = result.networks[0].ID
	}
	for _, network := range result.networks {
		if network.ID == result.selected {
			result.snapshot, result.err = easytier.ReadRemoteStatus(ctx, address, network)
		}
	}
	return result
}

// Only the UI thread mutates App. Background readers deliver immutable
// results through a channel; stale address/selection results are discarded.
func (a *App) refreshRemote(c *ui.Context) {
	if a.remotePending != nil {
		select {
		case result := <-a.remotePending:
			a.remotePending = nil
			if result.address == a.settings.RemoteRPC && a.remoteRequestedSelection == a.remoteSelected {
				a.remoteChecked = time.Now()
				a.remoteNetworks = result.networks
				if a.remoteSelected != result.selected {
					// The selected instance is gone; counters of another
					// instance must not diff against its baseline.
					a.resetRemoteTraffic()
				}
				a.remoteSnapshot = result.snapshot
				a.remoteSelected = result.selected
				a.remoteErr = ""
				if result.err != nil {
					a.remoteErr = result.err.Error()
					a.remoteSnapshot = easytier.Snapshot{Traffic: a.remoteTraffic}
				} else {
					a.remoteSnapshot = a.withRemoteTraffic(result.snapshot)
				}
			}
		default:
		}
	}
	c.After(250 * time.Millisecond)
	if a.remotePending != nil || time.Since(a.remoteChecked) < 3*time.Second {
		return
	}
	address, selected := a.settings.RemoteRPC, a.remoteSelected
	a.remoteRequestedSelection = selected
	results := make(chan remotePollResult, 1)
	a.remotePending = results
	win := a.win
	poll := a.remotePoll
	if poll == nil {
		poll = pollRemote
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results <- poll(ctx, address, selected)
		if win != nil {
			win.Invalidate()
		}
	}()
}

// withRemoteTraffic diffs the raw counters of a poll against the previous
// one and keeps the rate history across snapshot replacements.
func (a *App) withRemoteTraffic(snap easytier.Snapshot) easytier.Snapshot {
	now := time.Now()
	if snap.Counters == nil {
		a.remoteCounters, a.remoteSampleAt = nil, time.Time{}
	} else {
		if !a.remoteSampleAt.IsZero() {
			if elapsed := now.Sub(a.remoteSampleAt).Seconds(); elapsed > 0 {
				rx, tx := easytier.DiffTraffic(a.remoteCounters, snap.Counters, elapsed)
				a.remoteTraffic = easytier.AppendTrafficSample(a.remoteTraffic,
					easytier.TrafficSample{At: now, RxPerSecond: rx, TxPerSecond: tx})
			}
		}
		a.remoteCounters, a.remoteSampleAt = snap.Counters, now
	}
	snap.Traffic = append([]easytier.TrafficSample(nil), a.remoteTraffic...)
	return snap
}

// resetRemoteTraffic drops the rate history and counter baseline, for a
// switch of instance or endpoint.
func (a *App) resetRemoteTraffic() {
	a.remoteTraffic = nil
	a.remoteCounters = nil
	a.remoteSampleAt = time.Time{}
}

// selectRemote makes a remote instance the current context and forces a
// refresh of its state.
func (a *App) selectRemote(id string) {
	a.remoteActive = true
	if a.remoteSelected != id {
		a.remoteSelected = id
		a.resetRemoteTraffic()
	}
	a.remoteChecked = time.Time{}
	a.page = "overview"
	a.notice = ""
}

// viewRemote connects the inspector to an endpoint and makes its instances
// the current context immediately.
func (a *App) viewRemote(address string) {
	if a.settings.RemoteRPC != address {
		a.remoteSelected = ""
		a.resetRemoteTraffic()
	}
	a.settings.RemoteRPC = address
	a.remoteConnected = true
	a.remoteActive = true
	a.remoteChecked = time.Time{}
	a.page = "overview"
	a.notice = ""
}
