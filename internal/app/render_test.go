package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// TestRenderScreens renders every page to a PNG for design review:
//
//	RENDER=1 go test -run TestRenderScreens
func TestRenderScreens(t *testing.T) {
	if os.Getenv("RENDER") == "" {
		t.Skip("set RENDER=1 to write screenshots into /tmp/easytier-shots")
	}
	dir := "/tmp/easytier-shots"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, shot := range []struct {
		name   string
		page   string
		width  int
		height int
		setup  func(a *App)
	}{
		{name: "01-overview-disconnected", page: "overview", width: 1040, height: 700},
		{name: "02-overview-connected", page: "overview", width: 1040, height: 700, setup: connected},
		{name: "03-peers", page: "peers", width: 1040, height: 700, setup: connected},
		{name: "04-routes", page: "routes", width: 1040, height: 700, setup: connected},
		{name: "05-settings", page: "settings", width: 1040, height: 1100},
		{name: "06-logs", page: "logs", width: 1040, height: 700, setup: connected},
		{name: "07-about", page: "about", width: 1040, height: 700},
		{name: "08-preferences", page: "preferences", width: 1040, height: 700},
		{name: "09-service", page: "service", width: 1040, height: 900},
	} {
		t.Run(shot.name, func(t *testing.T) {
			a := newTestApp(t)
			a.page = shot.page
			if shot.setup != nil {
				shot.setup(a)
			}
			image := ui.Render(a.view, shot.width, shot.height, 2)
			file, err := os.Create(filepath.Join(dir, shot.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, image); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// connected installs a snapshot as if the instance were up with two peers.
func connected(a *App) {
	a.draft().config.NetworkName = "office"
	a.ctl().SetSnapshotForTest(easytier.Snapshot{
		Phase: easytier.PhaseRunning,
		Since: time.Now().Add(-42 * time.Minute),
		TUN:   true,
		Node: easytier.NodeView{
			Hostname:   "yiran-laptop",
			IPv4:       "10.144.0.10/24",
			PeerID:     "1234567",
			Version:    "2.6.4-8428a89d",
			InstID:     "b1f2c3d4-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
			Listeners:  []string{"tcp://0.0.0.0:11101", "udp://0.0.0.0:11101"},
			ProxyCIDRs: []string{"192.168.1.0/24"},
			PublicIPs:  []string{"61.173.162.201"},
			UDPNATType: 3,
			TCPNATType: 3,
		},
		Peers: []easytier.PeerView{
			{Name: "yiran-laptop", IPv4: "10.144.0.10", PeerID: "1234567", Local: true, Direct: true, Version: "2.6.4-8428a89d"},
			{Name: "office-nas", IPv4: "10.144.0.2", PeerID: "22", LatencyMS: 0.11, Direct: true, Tunnel: "tcp", RxBytes: "2.69 kB", TxBytes: "3.01 kB", NATType: "FullCone", Version: "2.6.4-8428a89d"},
			{Name: "thinkpad", IPv4: "10.144.0.20", PeerID: "33", LatencyMS: 48, Direct: false, Tunnel: "tcp", RxBytes: "18.2 MB", TxBytes: "1.44 MB", NATType: "Symmetric", Version: "2.6.3-1a2b3c4d"},
		},
		Routes: []easytier.RouteView{
			{Dest: "10.144.0.2/24", NextHop: "DIRECT", NextHopName: "DIRECT", Hops: 1, LatencyMS: 0.11, Name: "office-nas", Version: "2.6.4-8428a89d"},
			{Dest: "10.144.0.20/24", NextHop: "10.144.0.2", NextHopName: "office-nas", Hops: 2, LatencyMS: 48, Name: "thinkpad", ProxyCIDRs: "192.168.2.0/24", Version: "2.6.3-1a2b3c4d"},
			{Dest: "10.144.0.30/24", NextHop: "10.144.0.2", NextHopName: "office-nas", Hops: 3, LatencyMS: 231, Name: "raspberry", Version: "2.6.4-8428a89d"},
		},
		Logs: []easytier.LogEntry{
			{At: time.Now().Add(-5 * time.Minute), Kind: "info", Message: "已加入网络 \"office\""},
			{At: time.Now().Add(-4 * time.Minute), Kind: "info", Message: "虚拟网卡 et0 已就绪"},
			{At: time.Now().Add(-3 * time.Minute), Kind: "info", Message: "新节点 office-nas (10.144.0.2)"},
			{At: time.Now().Add(-2 * time.Minute), Kind: "warn", Message: "与 thinkpad 的中继连接延迟偏高"},
			{At: time.Now().Add(-1 * time.Minute), Kind: "info", Message: "建立直连：office-nas udp"},
		},
	})
}
