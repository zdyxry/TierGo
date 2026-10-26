package easytier

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The end-to-end test drives a real easytier-core. It needs the official
// binaries; without them it is skipped rather than failed:
//
//	EASYTIER_CORE=/path/to/easytier-core EASYTIER_CLI=/path/to/easytier-cli go test -run TestE2E
//
// It runs the core with the TUN device off, so it needs no privileges: the
// core still joins the network, learns peers and routes and serves port
// forwards. Only the kernel device is left out.
func TestE2EControllerWithRealCore(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test with the real easytier-core")
	}
	requireBinaries(t)

	// The client owns its directory: point it at a temporary one.
	configFile = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configFile = "" })

	config := DefaultConfig()
	config.NetworkName = "easytier-mygo-e2e"
	config.NetworkSecret = "e2e-secret"
	config.Hostname = "mygo-e2e"
	config.EnableTUN = false // no privileges needed

	controller := NewController("test")
	controller.Start(config)
	t.Cleanup(func() {
		controller.Stop()
		waitForPhase(t, controller, PhaseStopped, 30*time.Second)
	})

	snapshot := waitForPhase(t, controller, PhaseRunning, 60*time.Second)
	if snapshot.Err != "" {
		t.Fatalf("start failed: %s", snapshot.Err)
	}
	if snapshot.ConfigPath == "" || snapshot.Portal == "" {
		t.Fatalf("the instance is not described: %+v", snapshot)
	}
	if _, err := os.Stat(snapshot.ConfigPath); err != nil {
		t.Errorf("configuration was not written: %v", err)
	}

	// The core reports itself a moment after it starts.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if controller.Snapshot().Node.Hostname != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	node := controller.Snapshot().Node
	if node.Hostname != "mygo-e2e" {
		t.Fatalf("hostname = %q, want mygo-e2e", node.Hostname)
	}
	if node.Version == "" {
		t.Error("the core version was not read")
	}
	if len(node.Listeners) == 0 {
		t.Error("the core reports no listeners")
	}

	// Monitor data comes from a second, verbose peer query. A standalone
	// core still returns an empty counter set, not an unreadable result.
	var details []cliPeerDetails
	_, cli := requireBinaries(t)
	if err := cliQueryArgs(context.Background(), cli, snapshot.Portal, &details, "-v", "peer"); err != nil {
		t.Fatal(err)
	}
	if rawCounters(details) == nil {
		t.Fatal("counter query returned no sample")
	}

	// The log shows what the client did.
	logs := controller.Snapshot().Logs
	if len(logs) == 0 {
		t.Fatal("no log entries")
	}
	if !hasLog(logs, "已启动") {
		t.Errorf("the log does not record the start: %v", logs)
	}

	// Stopping ends the process for real.
	controller.Stop()
	waitForPhase(t, controller, PhaseStopped, 30*time.Second)
}

// TestE2EPeerDiscovery starts a second, plain easytier-core that dials the
// one the client runs, and checks that the client sees it.
//
// EasyTier's own connector cannot reach 127.0.0.1 in some sandboxes, so the
// test dials the machine's own address instead; it is skipped when there is
// none.
func TestE2EPeerDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test with the real easytier-core")
	}
	core, cli := requireBinaries(t)
	local := localAddress(t)

	configFile = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configFile = "" })

	networkName := "easytier-mygo-e2e-" + fmt.Sprint(time.Now().UnixNano()%100000)
	config := DefaultConfig()
	config.NetworkName = networkName
	config.NetworkSecret = "e2e-secret"
	config.Hostname = "mygo-a"
	config.IPv4 = "10.250.77.1/24"
	config.EnableTUN = false

	controller := NewController("test")
	controller.Start(config)
	t.Cleanup(func() {
		controller.Stop()
		waitForPhase(t, controller, PhaseStopped, 30*time.Second)
	})
	waitForPhase(t, controller, PhaseRunning, 60*time.Second)

	// Wait for the core to report the port it chose.
	var listenPort string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		for _, listener := range controller.Snapshot().Node.Listeners {
			if strings.HasPrefix(listener, "tcp://") && !strings.Contains(listener, "[::]") {
				listenPort = strings.TrimPrefix(listener, "tcp://")
				listenPort = listenPort[strings.LastIndex(listenPort, ":")+1:]
			}
		}
		if listenPort != "" && listenPort != "0" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if listenPort == "" || listenPort == "0" {
		t.Fatalf("the core reported no listening port: %v", controller.Snapshot().Node.Listeners)
	}

	// A second core joins through a peer the client is listening on.
	second := startPlainCore(t, core, cli, plainCoreOptions{
		name:    "mygo-b",
		network: networkName,
		secret:  "e2e-secret",
		ipv4:    "10.250.77.2/24",
		peer:    "tcp://" + local + ":" + listenPort,
	})

	// The client sees it.
	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		for _, peer := range controller.Snapshot().Peers {
			if peer.Name == "mygo-b" && !peer.Local {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	second.Stop()
	t.Fatalf("the other node never appeared: %+v", controller.Snapshot().Peers)
}

// TestE2EManagerRestore does what a launch does: it reconnects the networks
// that were enabled, with the real core, and leaves running ones alone.
func TestE2EManagerRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test with the real easytier-core")
	}
	requireBinaries(t)

	configFile = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configFile = "" })

	profile := NewProfile("恢复")
	profile.Config = DefaultConfig()
	profile.Config.NetworkName = "easytier-mygo-e2e-restore"
	profile.Config.NetworkSecret = "e2e-secret"
	profile.Config.Hostname = "mygo-restore"
	profile.Config.EnableTUN = false
	if err := SaveProfile(profile); err != nil {
		t.Fatal(err)
	}

	manager := NewManager()
	t.Cleanup(func() { manager.StopAllAndWait(30 * time.Second) })

	if err := manager.Restore([]Profile{profile}, []string{profile.ID, "missing"}); err != nil {
		t.Fatal(err)
	}
	controller := manager.Controller(profile.ID)
	snapshot := waitForPhase(t, controller, PhaseRunning, 60*time.Second)
	if snapshot.Err != "" {
		t.Fatalf("restore failed: %s", snapshot.Err)
	}

	// The core reports itself a moment after it starts.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if controller.Snapshot().Node.Hostname != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if got := controller.Snapshot().Node.Hostname; got != "mygo-restore" {
		t.Fatalf("hostname = %q, want mygo-restore", got)
	}

	// A second restore does not start a second core.
	if err := manager.Restore([]Profile{profile}, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	if !manager.Running(profile.ID) {
		t.Fatal("the restored network did not stay running")
	}
}

// TestE2ESecureModeIdentityStable starts a secure-mode network on the real
// core: the generated TOML is accepted, and the identity the core runs with
// is the one repeated renders keep.
func TestE2ESecureModeIdentityStable(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test with the real easytier-core")
	}
	requireBinaries(t)

	configFile = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configFile = "" })

	config := DefaultConfig()
	config.NetworkName = "easytier-mygo-e2e-secure"
	config.NetworkSecret = "e2e-secret"
	config.Hostname = "mygo-secure"
	config.EnableTUN = false
	config.SecureMode = true
	if err := EnsureSecureModeKeys(&config); err != nil {
		t.Fatal(err)
	}
	first, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("two renders of one configuration differ:\n%s\n%s", first, second)
	}

	controller := NewController("test")
	controller.Start(config)
	t.Cleanup(func() {
		controller.Stop()
		waitForPhase(t, controller, PhaseStopped, 30*time.Second)
	})
	snapshot := waitForPhase(t, controller, PhaseRunning, 60*time.Second)
	if snapshot.Err != "" {
		t.Fatalf("start failed: %s", snapshot.Err)
	}
	data, err := os.ReadFile(snapshot.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `local_private_key = "`+config.SecureModePrivateKey+`"`) {
		t.Fatalf("the core runs with a different identity:\n%s", data)
	}
}

// A real Core accepts an ACL with two chains of the same type, which the
// editor now allows adding.
func TestE2ECoreLoadsMultiChainACL(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	requireBinaries(t)
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	config := testConfig()
	config.ACL = DefaultACL()
	config.ACL.V1.Chains = append(config.ACL.V1.Chains, NewChain(1, config.ACL.V1.Chains))
	config.ACL.V1.Chains[3].Rules = []ACLRule{{Name: "deny-rest", Priority: 1, Enabled: true, Protocol: 5, Action: 2}}
	controller := NewController("acl-multi")
	controller.Start(config)
	t.Cleanup(func() { controller.StopAndWait(10 * time.Second) })
	snapshot := waitForPhase(t, controller, PhaseRunning, 30*time.Second)
	if snapshot.Err != "" {
		t.Fatalf("start failed: %s", snapshot.Err)
	}
}

// ---- helpers ----
// requireBinaries finds the core the same way the client does, embedded copy
// included, and skips the test when there is none.
func requireBinaries(t *testing.T) (string, string) {
	t.Helper()
	core, cli, _, err := LocateBinaries(t.TempDir())
	if err != nil {
		t.Skipf("跳过：%v", err)
	}
	return core, cli
}

// localAddress is a non-loopback IPv4 address of this machine.
func localAddress(t *testing.T) string {
	t.Helper()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("跳过：%v", err)
	}
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.IsLoopback() || network.IP.To4() == nil {
			continue
		}
		return network.IP.String()
	}
	t.Skip("跳过：这台机器没有非回环 IPv4 地址")
	return ""
}

type plainCoreOptions struct {
	name    string
	network string
	secret  string
	ipv4    string
	peer    string
}

// startPlainCore runs an easytier-core of its own, to be a peer.
func startPlainCore(t *testing.T, core, cli string, options plainCoreOptions) *coreProcess {
	t.Helper()
	dir := t.TempDir()
	config := fmt.Sprintf(`instance_name = %q
hostname = %q
ipv4 = %q
listeners = ["tcp://0.0.0.0:0"]

[[peer]]
uri = %q

[network_identity]
network_name = %q
network_secret = %q

[flags]
no_tun = true
`, options.name, options.name, options.ipv4, options.peer, options.network, options.secret)
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	portal := "127.0.0.1:0"
	process, err := startCoreProcess(core, []string{
		"--config-file", path,
		"--rpc-portal", portal,
		"--console-log-level", "warn",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(process.Kill)
	return process
}
