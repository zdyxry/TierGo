package easytier

import (
	"encoding/binary"
	"net/netip"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

type cliIPv6 struct{ Part1, Part2, Part3, Part4 uint32 }

func (ip cliIPv6) String() string {
	var b [16]byte
	for i, part := range []uint32{ip.Part1, ip.Part2, ip.Part3, ip.Part4} {
		binary.BigEndian.PutUint32(b[i*4:], part)
	}
	return netip.AddrFrom16(b).String()
}

// Counters come from `easytier-cli -v peer`, not the humanized peer table.
// Per-connection baselines avoid spikes when a connection disappears or is
// replaced. Samples are bytes/second of physical peer connections (including
// overlay overhead), not virtual-interface application throughput.
type ConnCounter struct{ Rx, Tx uint64 }
type TrafficSample struct {
	At                       time.Time
	RxPerSecond, TxPerSecond float64
}

// DiffTraffic turns two raw counter reads into bytes/second. Connections
// missing from the baseline and counters that went backwards are skipped, so
// a reconnect never shows as a spike.
func DiffTraffic(previous, current map[string]ConnCounter, elapsed float64) (rx, tx float64) {
	if elapsed <= 0 {
		return 0, 0
	}
	for id, now := range current {
		old, ok := previous[id]
		if !ok {
			continue
		}
		if now.Rx >= old.Rx {
			rx += float64(now.Rx - old.Rx)
		}
		if now.Tx >= old.Tx {
			tx += float64(now.Tx - old.Tx)
		}
	}
	return rx / elapsed, tx / elapsed
}

// AppendTrafficSample keeps the rate history bounded at 120 samples.
func AppendTrafficSample(samples []TrafficSample, sample TrafficSample) []TrafficSample {
	samples = append(samples, sample)
	if len(samples) > 120 {
		samples = samples[len(samples)-120:]
	}
	return samples
}

type NetworkEvent struct {
	At                   time.Time
	Kind, Detail, Source string
}

func (ctl *Controller) monitorLocked(status coreStatus, now time.Time) {
	if status.Counters != nil {
		if elapsed := now.Sub(ctl.sampleAt).Seconds(); !ctl.sampleAt.IsZero() && elapsed > 0 {
			rx, tx := DiffTraffic(ctl.counters, status.Counters, elapsed)
			ctl.snap.Traffic = AppendTrafficSample(ctl.snap.Traffic, TrafficSample{now, rx, tx})
		}
		ctl.counters, ctl.sampleAt = status.Counters, now
	} else {
		ctl.counters, ctl.sampleAt = nil, time.Time{}
	}
	for _, event := range status.Events {
		seen := false
		for _, old := range ctl.snap.Events {
			if old.Source == event.Source && old.Kind == event.Kind && old.At.Equal(event.At) && old.Detail == event.Detail {
				seen = true
				break
			}
		}
		if !seen {
			ctl.snap.Events = append(ctl.snap.Events, event)
		}
	}
	if len(ctl.snap.Events) > 200 {
		ctl.snap.Events = ctl.snap.Events[len(ctl.snap.Events)-200:]
	}
	// A fallback for older cores without the management event API. Mark as
	// observed snapshot changes, not authoritative Core event messages.
	peersReadable := true
	for _, part := range status.Unreadable {
		if part == locale.T("节点列表") {
			peersReadable = false
		}
	}
	if ctl.observed && peersReadable && status.Events == nil {
		oldPeers := map[string]bool{}
		newPeers := map[string]bool{}
		for _, p := range ctl.snap.Peers {
			oldPeers[p.PeerID] = true
		}
		for _, p := range status.Peers {
			newPeers[p.PeerID] = true
			if !oldPeers[p.PeerID] {
				ctl.eventLocked(now, locale.T("节点出现"), p.Name)
			}
		}
		{
			for _, p := range ctl.snap.Peers {
				if !newPeers[p.PeerID] {
					ctl.eventLocked(now, locale.T("节点离开"), p.Name)
				}
			}
		}
		if ctl.snap.Node.IPv4 != status.Node.IPv4 {
			ctl.eventLocked(now, locale.T("虚拟 IP 变化"), status.Node.IPv4)
		}
		oldListeners := map[string]bool{}
		for _, listener := range ctl.snap.Node.Listeners {
			oldListeners[listener] = true
		}
		for _, listener := range status.Node.Listeners {
			if !oldListeners[listener] {
				ctl.eventLocked(now, locale.T("监听器出现"), listener)
			}
		}
	}
	ctl.observed = true
}
func (ctl *Controller) eventLocked(at time.Time, kind, detail string) {
	ctl.snap.Events = append(ctl.snap.Events, NetworkEvent{at, kind, detail, locale.T("状态轮询")})
	if len(ctl.snap.Events) > 200 {
		ctl.snap.Events = ctl.snap.Events[len(ctl.snap.Events)-200:]
	}
}

type cliPeerDetails struct {
	Route *struct {
		PeerID   uint32      `json:"peer_id"`
		Features *cliFeature `json:"feature_flag"`
	} `json:"route"`
	Peer *struct {
		Conns []struct {
			ID     string `json:"conn_id"`
			Closed bool   `json:"is_closed"`
			Stats  *struct {
				Rx uint64 `json:"rx_bytes"`
				Tx uint64 `json:"tx_bytes"`
			} `json:"stats"`
		} `json:"conns"`
	} `json:"peer"`
}

func rawCounters(details []cliPeerDetails) map[string]ConnCounter {
	out := map[string]ConnCounter{}
	for _, entry := range details {
		if entry.Peer != nil {
			for _, conn := range entry.Peer.Conns {
				if !conn.Closed && conn.Stats != nil {
					out[conn.ID] = ConnCounter{conn.Stats.Rx, conn.Stats.Tx}
				}
			}
		}
	}
	return out
}
