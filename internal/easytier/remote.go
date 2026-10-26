package easytier

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
	"google.golang.org/protobuf/encoding/protowire"
)

// RemoteNetwork is an instance advertised by a management endpoint. It is
// not a local profile: disconnecting the GUI never deletes or stops it.
type RemoteNetwork struct {
	ID, Name, InstanceName string
	Permissions            uint32
	Disabled               bool
}

func NormalizeRPCAddress(address string) (string, error) {
	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil || u.Scheme != "tcp" || u.User != nil || u.Path != "" || u.RawQuery != "" {
			return "", fmt.Errorf("%s", locale.T("RPC 地址格式应为 tcp://主机:端口"))
		}
		address = u.Host
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return "", fmt.Errorf("%s", locale.T("RPC 地址格式应为 主机:端口"))
	}
	return address, nil
}
func ListRemoteNetworks(ctx context.Context, address string) ([]RemoteNetwork, error) {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return nil, err
	}
	// Meta requires an explicit ID list; an empty request means no results.
	ids, err := managementRPC(ctx, address, "WebClientService", 5, nil)
	if err != nil {
		return nil, err
	}
	listed, err := wireFields(ids)
	if err != nil {
		return nil, err
	}
	var request []byte
	for _, id := range listed {
		if id.number == 1 {
			request = wireBytes(request, 1, id.bytes)
		}
	}
	if len(request) == 0 {
		return listDisabledRemote(address)
	}
	data, err := managementRPC(ctx, address, "WebClientService", 8, request)
	if err != nil {
		return nil, err
	}
	fields, err := wireFields(data)
	if err != nil {
		return nil, err
	}
	var out []RemoteNetwork
	for _, field := range fields {
		if field.number != 1 {
			continue
		}
		meta, err := wireFields(field.bytes)
		if err != nil {
			return nil, err
		}
		network := RemoteNetwork{}
		for _, f := range meta {
			switch f.number {
			case 1:
				network.ID, err = decodeUUID(f.bytes)
				if err != nil {
					return nil, err
				}
			case 2:
				network.Name = string(f.bytes)
			case 3:
				network.Permissions = uint32(f.value)
			case 4:
				network.InstanceName = string(f.bytes)
			}
		}
		out = append(out, network)
	}
	disabled, err := listDisabledRemote(address)
	if err != nil {
		return nil, err
	}
	for _, saved := range disabled {
		found := false
		for _, running := range out {
			if running.ID == saved.ID {
				found = true
				break
			}
		}
		if !found {
			out = append(out, saved)
		}
	}
	return out, nil
}
func encodeUUID(id string) ([]byte, error) {
	bytes, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil || len(bytes) != 16 {
		return nil, fmt.Errorf("invalid UUID")
	}
	var out []byte
	for i := 0; i < 4; i++ {
		out = wireInt(out, protowire.Number(i+1), uint64(binary.BigEndian.Uint32(bytes[i*4:])))
	}
	return out, nil
}
func decodeUUID(data []byte) (string, error) {
	fields, err := wireFields(data)
	if err != nil {
		return "", err
	}
	var bytes [16]byte
	for _, f := range fields {
		if f.number >= 1 && f.number <= 4 {
			binary.BigEndian.PutUint32(bytes[int(f.number-1)*4:], uint32(f.value))
		}
	}
	s := hex.EncodeToString(bytes[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}

// ReadRemoteStatus uses CLI's instance selector for the state tables, then
// the native API for the event ring. No process belongs to this reader.
func ReadRemoteStatus(ctx context.Context, address string, network RemoteNetwork) (Snapshot, error) {
	if network.Disabled {
		return Snapshot{Phase: PhaseStopped}, nil
	}
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return Snapshot{}, err
	}
	root, err := InstanceRoot()
	if err != nil {
		return Snapshot{}, err
	}
	_, cli, _, err := LocateBinaries(root)
	if err != nil {
		return Snapshot{}, err
	}
	query := func(command string, out any) error {
		return cliQueryArgs(ctx, cli, address, out, "-i", network.ID, command)
	}
	var node cliNode
	if err := query("node", &node); err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Phase: PhaseRunning, Portal: address, Node: nodeView(node)}
	var peers []cliPeer
	if query("peer", &peers) == nil {
		snap.Peers = peerViews(peers)
	} else {
		snap.Unreadable = append(snap.Unreadable, locale.T("节点列表"))
	}
	var routes []cliRoute
	if query("route", &routes) == nil {
		snap.Routes = routeViews(routes)
	} else {
		snap.Unreadable = append(snap.Unreadable, locale.T("路由表"))
	}
	// Same verbose counter query as the local monitor; the app diffs the raw
	// per-connection counts across polls to get rates.
	var details []cliPeerDetails
	if err := cliQueryArgs(ctx, cli, address, &details, "-i", network.ID, "-v", "peer"); err == nil {
		snap.Counters = rawCounters(details)
		for _, detail := range details {
			if detail.Route == nil || detail.Route.Features == nil {
				continue
			}
			for i := range snap.Peers {
				if snap.Peers[i].PeerID == fmt.Sprint(detail.Route.PeerID) {
					snap.Peers[i].PublicServer = detail.Route.Features.IsPublicServer
					snap.Peers[i].AvoidRelay = detail.Route.Features.AvoidRelayData
				}
			}
		}
	} else {
		snap.Unreadable = append(snap.Unreadable, locale.T("流量计数"))
	}
	// Filter event collection to this instance in collectCoreEventsFor.
	if events, err := collectCoreEventsFor(ctx, address, network.ID); err == nil {
		snap.Events = events
	} else {
		snap.Unreadable = append(snap.Unreadable, locale.T("Core 事件"))
	}
	return snap, nil
}
