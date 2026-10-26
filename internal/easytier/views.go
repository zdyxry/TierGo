package easytier

import (
	"sort"
	"strconv"
	"strings"
)

// peerViews maps the CLI's peer table to what the window shows.
func peerViews(peers []cliPeer) []PeerView {
	views := make([]PeerView, 0, len(peers))
	for _, peer := range peers {
		latency, _ := strconv.ParseFloat(peer.LatencyMS, 64)
		view := PeerView{
			Name:        peer.Hostname,
			IPv4:        peer.IPv4,
			PeerID:      peer.ID,
			LatencyMS:   latency,
			LatencyText: peer.LatencyMS,
			LossRate:    peer.LossRate,
			RxBytes:     peer.RxBytes,
			TxBytes:     peer.TxBytes,
			Tunnel:      peer.TunnelProto,
			NATType:     peer.NATType,
			Version:     peer.Version,
			Local:       strings.EqualFold(peer.Cost, "local"),
			Direct:      strings.EqualFold(peer.Cost, "p2p") || strings.EqualFold(peer.Cost, "local"),
		}
		if view.Name == "" {
			view.Name = peer.IPv4
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Local != views[j].Local {
			return views[i].Local
		}
		return views[i].LatencyMS < views[j].LatencyMS
	})
	return views
}

func routeViews(routes []cliRoute) []RouteView {
	views := make([]RouteView, 0, len(routes))
	for _, route := range routes {
		latency := route.NextHopLat
		if latency == 0 && route.PathLatency != 0 {
			latency = float64(route.PathLatency)
		}
		views = append(views, RouteView{
			Dest:        route.IPv4,
			NextHop:     route.NextHopIPv4,
			NextHopName: route.NextHopName,
			Hops:        route.PathLen,
			LatencyMS:   latency,
			Name:        route.Hostname,
			ProxyCIDRs:  route.ProxyCIDRs,
			Version:     route.Version,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Dest < views[j].Dest
	})
	return views
}

// parseLogLine reads one easytier-core log line:
//
//	2026-10-22T17:05:17.82+08:00 WARN easytier::tunnel::wireguard: wg connect: …
//	2026-10-22T17:05:17.82+08:00  INFO CORE::INSTANCE: [id] listener added
//
// and returns its level and the part worth showing. The window already has a
// time column and a level badge, so the timestamp and level go, and the
// module keeps only its last name. The core pads the level with a space on
// some lines, hence the trimming.
func parseLogLine(line string) (level, message string) {
	rest := strings.TrimLeft(line, " ")
	stamp, remaining, found := cutField(rest)
	if !found || !isTimestamp(stamp) {
		return "info", line
	}
	candidate, message, found := cutField(remaining)
	if !found || !parsedLevel(strings.ToLower(candidate)) {
		return "info", line
	}
	return strings.ToLower(candidate), shortModule(message)
}

// cutField splits the first space-separated field off a line.
func cutField(line string) (field, rest string, found bool) {
	index := strings.IndexByte(line, ' ')
	if index < 0 {
		return "", "", false
	}
	return line[:index], strings.TrimLeft(line[index:], " "), true
}

// isTimestamp spots the RFC 3339 stamp the core starts its lines with.
func isTimestamp(value string) bool {
	return len(value) >= 20 && value[4] == '-' && value[10] == 'T'
}

func parsedLevel(level string) bool {
	switch level {
	case "trace", "debug", "info", "warn", "error":
		return true
	}
	return false
}

// shortModule turns "easytier::peers::peer_ospf_route: …" into
// "peer_ospf_route: …", which is as much as anyone needs.
func shortModule(message string) string {
	module, rest, found := strings.Cut(message, ": ")
	if !found {
		return message
	}
	if index := strings.LastIndex(module, "::"); index >= 0 {
		module = module[index+2:]
	}
	return module + ": " + rest
}
