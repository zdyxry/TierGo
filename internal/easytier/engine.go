package easytier

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// The desktop never runs a process itself: it asks a coreEngine to launch the
// EasyTier core and hands it a coreSpec. The real engine starts
// easytier-core; tests provide their own, so the whole state machine can be
// driven without the official binaries.
//
// This boundary is what makes "the desktop only writes a TOML" untrue: the
// engine owns launch, probing and stopping, and the controller owns the
// policy around them.

// coreSpec is everything the engine needs to launch one core.
type coreSpec struct {
	core       string
	cli        string
	workDir    string
	configPath string
	logDir     string
	portal     string
	args       []string
	// readyTimeout is how long the core may take to answer at all.
	readyTimeout time.Duration
}

// coreStatus is what a probe observed. Unreadable names the parts the core
// did not answer for: the window says so rather than presenting stale values
// as current ones.
type coreStatus struct {
	Node       NodeView
	Peers      []PeerView
	Routes     []RouteView
	Unreadable []string
	Counters   map[string]ConnCounter
	Events     []NetworkEvent
}

// coreRun is one running core.
type coreRun interface {
	// portal is the management address the core was told to listen on.
	portal() string
	// lines streams the core's console log until it exits.
	lines() <-chan string
	// exited is closed when the process is gone.
	exited() <-chan struct{}
	// wait blocks until the process is gone and reports why it ended.
	wait() error
	// pid is the process number, or 0 when there is none.
	pid() int
	// ready blocks until the core answers on its management port.
	ready(ctx context.Context) error
	// read probes the core for its state.
	read(ctx context.Context) (coreStatus, error)
	// stop asks the core to exit, so it removes its TUN device and routes.
	stop()
	// kill ends the core at once.
	kill()
}

type coreEngine interface {
	start(spec coreSpec) (coreRun, error)
}

// nativeEngine runs the official easytier-core.
type nativeEngine struct{}

func (nativeEngine) start(spec coreSpec) (coreRun, error) {
	process, err := startCoreProcess(spec.core, spec.args, spec.workDir)
	if err != nil {
		return nil, err
	}
	return &nativeRun{
		process:      process,
		cli:          spec.cli,
		address:      spec.portal,
		readyTimeout: spec.readyTimeout,
	}, nil
}

type nativeRun struct {
	process      *coreProcess
	cli          string
	address      string
	readyTimeout time.Duration
}

func (r *nativeRun) portal() string          { return r.address }
func (r *nativeRun) lines() <-chan string    { return r.process.output() }
func (r *nativeRun) exited() <-chan struct{} { return r.process.done }
func (r *nativeRun) wait() error             { return r.process.Wait() }
func (r *nativeRun) stop()                   { r.process.Stop() }
func (r *nativeRun) kill()                   { r.process.Kill() }
func (r *nativeRun) pid() int                { return r.process.pid() }

// ready waits until the management port accepts connections. Answering it is
// not the same as working, which is what the probe after it is for.
func (r *nativeRun) ready(ctx context.Context) error {
	return waitForPortal(ctx, r.address, r.readyTimeout)
}

// read asks easytier-cli for this node, its peers and its routes. Only the
// node is required: without it there is nothing to show. The other two name
// themselves in Unreadable when they fail.
func (r *nativeRun) read(ctx context.Context) (coreStatus, error) {
	var status coreStatus

	var node cliNode
	if err := cliQuery(ctx, r.cli, r.address, "node", &node); err != nil {
		return status, fmt.Errorf(locale.T("读取节点信息失败：%w"), err)
	}
	status.Node = nodeView(node)

	var peers []cliPeer
	if err := cliQuery(ctx, r.cli, r.address, "peer", &peers); err != nil {
		status.Unreadable = append(status.Unreadable, locale.T("节点列表"))
	} else {
		status.Peers = peerViews(peers)
	}

	var routes []cliRoute
	if err := cliQuery(ctx, r.cli, r.address, "route", &routes); err != nil {
		status.Unreadable = append(status.Unreadable, locale.T("路由表"))
	} else {
		status.Routes = routeViews(routes)
	}
	var details []cliPeerDetails
	if err := cliQueryArgs(ctx, r.cli, r.address, &details, "-v", "peer"); err == nil {
		status.Counters = rawCounters(details)
		for _, detail := range details {
			if detail.Route == nil || detail.Route.Features == nil {
				continue
			}
			for i := range status.Peers {
				if status.Peers[i].PeerID == fmt.Sprint(detail.Route.PeerID) {
					status.Peers[i].PublicServer = detail.Route.Features.IsPublicServer
					status.Peers[i].AvoidRelay = detail.Route.Features.AvoidRelayData
				}
			}
		}
	} else {
		status.Unreadable = append(status.Unreadable, locale.T("流量计数"))
	}
	if events, err := collectCoreEvents(ctx, r.address); err == nil {
		status.Events = events
	} else {
		status.Unreadable = append(status.Unreadable, locale.T("Core 事件"))
	}
	return status, nil
}

// waitForPortal waits until the management port accepts connections.
func waitForPortal(ctx context.Context, portal string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		connection, err := net.DialTimeout("tcp", portal, 500*time.Millisecond)
		if err == nil {
			connection.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf(locale.T("等待 %s 超时"), portal)
}

func nodeView(node cliNode) NodeView {
	view := NodeView{
		Hostname:   node.Hostname,
		IPv4:       node.IPv4Addr,
		PeerID:     fmt.Sprint(node.PeerID),
		Version:    node.Version,
		InstID:     node.InstID,
		Listeners:  node.Listeners,
		ProxyCIDRs: node.ProxyCIDRs,
	}
	if node.StunInfo != nil {
		view.PublicIPs = node.StunInfo.PublicIP
		view.UDPNATType = node.StunInfo.UDPNATType
		view.TCPNATType = node.StunInfo.TCPNATType
	}
	if node.IPList != nil {
		for _, ip := range node.IPList.IPv4s {
			view.InterfaceIPv4s = append(view.InterfaceIPv4s, net.IPv4(byte(ip.Addr>>24), byte(ip.Addr>>16), byte(ip.Addr>>8), byte(ip.Addr)).String())
		}
		for _, ip := range node.IPList.IPv6s {
			view.InterfaceIPv6s = append(view.InterfaceIPv6s, ip.String())
		}
		if node.IPList.PublicIPv6 != nil {
			view.PublicIPv6 = node.IPList.PublicIPv6.String()
		}
	}
	var config struct {
		Flags struct {
			DevName string `toml:"dev_name"`
		} `toml:"flags"`
	}
	if toml.Unmarshal([]byte(node.Config), &config) == nil {
		view.TUNName = config.Flags.DevName
	}
	return view
}
