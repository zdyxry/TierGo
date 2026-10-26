package app

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// remoteStub serves canned remote polls; counters grow per call so rates can
// be diffed.
type remoteStub struct {
	calls    int
	networks []easytier.RemoteNetwork
	snapshot func(call int) easytier.Snapshot
}

func (s *remoteStub) poll(ctx context.Context, address, selected string) remotePollResult {
	s.calls++
	result := remotePollResult{address: address, selected: selected, networks: s.networks}
	found := false
	for _, network := range s.networks {
		if network.ID == selected {
			found = true
		}
	}
	if !found && len(s.networks) > 0 {
		result.selected = s.networks[0].ID
	}
	result.snapshot = s.snapshot(s.calls)
	return result
}

func (s *remoteStub) snapshotWithHostname(hostname string) easytier.Snapshot {
	return easytier.Snapshot{
		Phase:    easytier.PhaseRunning,
		Node:     easytier.NodeView{Hostname: hostname, IPv4: "10.9.0.8/24"},
		Counters: map[string]easytier.ConnCounter{"c1": {Rx: uint64(s.calls * 600), Tx: uint64(s.calls * 300)}},
	}
}

// pumpRemote waits for the scheduled poll to deliver and renders the frame
// that consumes it.
func pumpRemote(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	pending := a.remotePending
	if pending == nil {
		t.Fatal("no remote poll scheduled")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(pending) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if a.remotePending != nil {
		t.Fatal("poll result was not consumed")
	}
}

// activeRemote installs a remote context without any RPC; remoteConnected
// stays false so the view layer does not start a real poll against the
// dummy address.
func activeRemote(a *App) {
	a.remoteActive = true
	a.settings.RemoteRPC = "127.0.0.1:12345"
	a.remoteNetworks = []easytier.RemoteNetwork{{ID: "r1", Name: "办公"}}
	a.remoteSelected = "r1"
	a.remoteSnapshot = easytier.Snapshot{Phase: easytier.PhaseRunning, Portal: "127.0.0.1:12345"}
}

func TestRemoteSidebarGroupAndContextSwitch(t *testing.T) {
	a := newTestApp(t)
	a.settings.RemoteRPC = "127.0.0.1:12345"
	a.remoteConnected = true
	stub := &remoteStub{
		networks: []easytier.RemoteNetwork{{ID: "r1", Name: "办公"}, {ID: "r2", Name: "旧网", Disabled: true}},
	}
	stub.snapshot = func(call int) easytier.Snapshot { return stub.snapshotWithHostname("remote-host") }
	a.remotePoll = stub.poll

	// The poll runs on the remote page before any remote instance is active.
	a.page = "remote"
	tt := ui.NewTester(a.view, 1200, 1000)
	pumpRemote(t, a, tt)
	if !tt.HasText("远端实例") || !tt.HasText("办公") || !tt.HasText("旧网") {
		t.Fatalf("sidebar has no remote group: %q", tt.Texts())
	}
	if !tt.HasText("已停用") {
		t.Fatalf("disabled remote is not marked: %q", tt.Texts())
	}
	if a.remoteActive {
		t.Fatal("listing alone activated a remote context")
	}

	if err := tt.Click("办公"); err != nil {
		t.Fatal(err)
	}
	if !a.remoteActive || a.remoteSelected != "r1" || a.page != "overview" {
		t.Fatalf("remoteActive=%v selected=%q page=%q", a.remoteActive, a.remoteSelected, a.page)
	}
	if !tt.HasText("remote-host") || !tt.HasText("远端 办公") {
		t.Fatalf("overview does not show the remote instance: %q", tt.Texts())
	}
	if !tt.HasText("停用远端网络") {
		t.Fatalf("topbar keeps the local connect button: %q", tt.Texts())
	}

	// Peers and monitor pages now belong to the remote instance.
	if err := tt.Click("节点"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("「远端 办公」中的其他设备") {
		t.Fatalf("peers heading is not remote: %q", tt.Texts())
	}

	// Clicking a local profile switches back.
	if err := tt.Click("测试网络"); err != nil {
		t.Fatal(err)
	}
	if a.remoteActive {
		t.Fatal("selecting a local profile kept the remote context")
	}
}

func TestRemoteTrafficSamplingAcrossPolls(t *testing.T) {
	a := newTestApp(t)
	a.settings.RemoteRPC = "127.0.0.1:12345"
	a.remoteConnected = true
	a.remoteActive = true
	a.remoteSelected = "r1"
	stub := &remoteStub{networks: []easytier.RemoteNetwork{{ID: "r1", Name: "办公"}}}
	stub.snapshot = func(call int) easytier.Snapshot { return stub.snapshotWithHostname("remote-host") }
	a.remotePoll = stub.poll

	tt := ui.NewTester(a.view, 1200, 1000)
	pumpRemote(t, a, tt)
	if len(a.remoteSnapshot.Traffic) != 0 {
		t.Fatal("the first poll is a baseline and must not emit a sample")
	}
	a.remoteChecked = time.Time{}
	tt.Frame()
	pumpRemote(t, a, tt)
	if len(a.remoteSnapshot.Traffic) != 1 {
		t.Fatalf("samples = %v", a.remoteSnapshot.Traffic)
	}
	if a.remoteSnapshot.Traffic[0].RxPerSecond <= 0 || a.remoteSnapshot.Traffic[0].TxPerSecond <= 0 {
		t.Fatalf("sample = %+v", a.remoteSnapshot.Traffic[0])
	}
	// The history lives in the app, so an error snapshot keeps it.
	if got := (&App{remoteTraffic: a.remoteTraffic}); len(got.remoteTraffic) != 1 {
		t.Fatal("history lost")
	}

	if err := tt.Click("监控"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("实时流量（字节/秒）") || !tt.HasText("远端运行时日志级别") {
		t.Fatalf("monitor page is not remote-aware: %q", tt.Texts())
	}
}

func TestRemoteACLPageReadEditSave(t *testing.T) {
	a := newTestApp(t)
	activeRemote(a)
	a.page = "acl"

	acl := easytier.DefaultACL()
	acl.V1.Group.Members = []string{"ops"}
	remoteConfig, err := (easytier.RemoteConfig{Network: a.remoteNetworks[0]}).WithACL(acl)
	if err != nil {
		t.Fatal(err)
	}
	var saved *easytier.ACL
	a.getRemoteConfig = func(context.Context, string, easytier.RemoteNetwork) (easytier.RemoteConfig, error) {
		return remoteConfig, nil
	}
	a.saveRemoteConfig = func(ctx context.Context, address string, config easytier.RemoteConfig) error {
		parsed, err := config.ACL()
		saved = parsed
		return err
	}

	tt := ui.NewTester(a.view, 1200, 2400)
	if err := tt.Click("读取远端 ACL"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	tt.Frame()
	if a.remoteACL == nil || len(a.remoteACL.V1.Chains) != 3 {
		t.Fatalf("remote ACL not loaded: %+v", a.remoteACL)
	}
	if a.remoteACLMembers != "ops" {
		t.Fatalf("members = %q", a.remoteACLMembers)
	}
	if err := tt.Click("添加 Inbound 规则"); err != nil {
		t.Fatal(err)
	}
	a.aclRule.value.Name = "web"
	a.aclRule.ports = "443"
	if err := tt.Click("应用规则"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存远端 ACL"); err != nil {
		t.Fatal(err)
	}
	if !a.remoteACLSave {
		t.Fatal("saving a remote ACL must ask for confirmation")
	}
	if err := tt.Click("确认保存远端 ACL"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	if saved == nil || len(saved.V1.Chains[0].Rules) != 1 || saved.V1.Chains[0].Rules[0].Ports[0] != "443" {
		t.Fatalf("saved = %+v", saved)
	}
	if len(saved.V1.Group.Members) != 1 || saved.V1.Group.Members[0] != "ops" {
		t.Fatalf("members not applied: %+v", saved.V1.Group)
	}

	// A read-only remote network offers no editing at all.
	a.remoteNetworks[0].Permissions = 1
	a.remoteACL = nil
	a.remoteACLKey = ""
	tt.Frame()
	if !tt.HasText("远端网络配置只读") {
		t.Fatalf("read-only remote not explained: %q", tt.Texts())
	}
	if err := tt.Click("读取远端 ACL"); err == nil {
		t.Fatal("read-only config load unexpectedly offered")
	}
}

func TestRemotePortalPageRendersRemoteData(t *testing.T) {
	a := newTestApp(t)
	activeRemote(a)
	a.page = "portal"
	tt := ui.NewTester(a.view, 1200, 1400)
	if !tt.HasText("读取远端 Portal 配置") {
		t.Fatalf("portal page is not remote-aware: %q", tt.Texts())
	}
	a.portalInfo = easytier.PortalInfo{
		ClientConfig: "[Interface]\nPrivateKey = secret\n",
		Listener:     "0.0.0.0:51820",
		Clients: []easytier.PortalDeviceInfo{
			{PortalDevice: easytier.PortalDevice{Name: "laptop", VirtualIP: "10.14.14.2/24"}, State: 3},
		},
	}
	a.portalOwner = a.portalContextKey()
	tt.Frame()
	if !tt.HasText("laptop") || !tt.HasText("10.14.14.2/24") {
		t.Fatalf("remote portal devices not shown: %q", tt.Texts())
	}
	if !tt.HasText("在线") {
		t.Fatalf("device state not shown: %q", tt.Texts())
	}
	// The local config editor must not appear for a remote instance.
	if tt.HasText("启用 VPN Portal") {
		t.Fatalf("local portal editor shown for a remote instance: %q", tt.Texts())
	}
}

func TestRemoteTopbarDisableAsksConfirmation(t *testing.T) {
	a := newTestApp(t)
	activeRemote(a)
	tt := ui.NewTester(a.view, 1200, 900)
	if !tt.HasText("远端实例") {
		t.Fatalf("no remote badge: %q", tt.Texts())
	}
	if err := tt.Click("停用远端网络"); err != nil {
		t.Fatal(err)
	}
	if !a.remoteStateConfirm || a.remoteStateTarget.ID != "r1" {
		t.Fatal("disabling a remote network must go through the confirmation")
	}
	if err := tt.Click("取消状态更改"); err != nil {
		t.Fatal(err)
	}

	// Read-only/no-delete remotes disable the button.
	a.remoteNetworks[0].Permissions = 3
	tt.Frame()
	if err := tt.Click("停用远端网络"); err != nil {
		t.Fatal(err)
	}
	if a.remoteStateConfirm {
		t.Fatal("a disabled button opened the confirmation")
	}
}

func TestLocalOnlyPagesExplainRemoteContext(t *testing.T) {
	a := newTestApp(t)
	activeRemote(a)
	tt := ui.NewTester(a.view, 1200, 900)
	if err := tt.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("远端实例的配置请在「远程管理」页") {
		t.Fatalf("settings page has no remote hint: %q", tt.Texts())
	}
	if err := tt.Click("日志"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("远端实例的进程日志不在本机") {
		t.Fatalf("logs page has no remote hint: %q", tt.Texts())
	}
}

func TestViewRemoteEntryActivatesContext(t *testing.T) {
	a := newTestApp(t)
	a.viewRemote("127.0.0.1:15888")
	if !a.remoteConnected || !a.remoteActive || a.page != "overview" {
		t.Fatalf("connected=%v active=%v page=%q", a.remoteConnected, a.remoteActive, a.page)
	}
	if a.settings.RemoteRPC != "127.0.0.1:15888" {
		t.Fatalf("address = %q", a.settings.RemoteRPC)
	}
	// Changing the endpoint invalidates the selection and the baseline.
	a.remoteSelected = "r1"
	a.remoteTraffic = []easytier.TrafficSample{{}}
	a.viewRemote("127.0.0.1:25888")
	if a.remoteSelected != "" || len(a.remoteTraffic) != 0 {
		t.Fatal("endpoint change kept stale remote state")
	}
}
