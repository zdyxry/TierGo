package easytier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testManager wires a manager whose every instance gets its own scripted
// core, so parallel networks are exercised without the official binaries.
func testManager(t *testing.T) (*Manager, map[string]*fakeEngine) {
	t.Helper()
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))

	engines := map[string]*fakeEngine{}
	manager := NewManager()
	manager.newController = func(key string) *Controller {
		controller := NewController(key)
		engine := &fakeEngine{}
		controller.engine = engine
		controller.budgets = budgets{
			portal:   2 * time.Second,
			probe:    2 * time.Second,
			graceful: time.Second,
			poll:     30 * time.Millisecond,
		}
		engines[key] = engine
		return controller
	}
	t.Cleanup(func() { manager.StopAllAndWait(2 * time.Second) })
	return manager, engines
}

// Two networks run at the same time, each as its own core with its own
// state: what one reports never leaks into the other.
func TestManagerRunsTwoNetworksAtOnce(t *testing.T) {
	manager, engines := testManager(t)

	office := NewProfile("公司")
	office.Config = testConfig()
	home := NewProfile("家庭")
	home.Config = testConfig()
	home.Config.NetworkName = "home-net"

	manager.Start(office)
	manager.Start(home)

	waitForPhase(t, manager.Controller(office.ID), PhaseRunning, 5*time.Second)
	waitForPhase(t, manager.Controller(home.ID), PhaseRunning, 5*time.Second)

	if !manager.Running(office.ID) || !manager.Running(home.ID) {
		t.Fatal("both networks should be running")
	}
	if ids := manager.RunningIDs(); len(ids) != 2 {
		t.Fatalf("running ids = %v", ids)
	}

	// Give each core its own view of the world and wait for the poll.
	engines[office.ID].newRun().setStatus(coreStatus{
		Node: NodeView{Hostname: "office-node", IPv4: "10.144.0.10/24"},
	})
	engines[home.ID].newRun().setStatus(coreStatus{
		Node: NodeView{Hostname: "home-node", IPv4: "10.144.0.20/24"},
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		o := manager.Snapshot(office.ID)
		h := manager.Snapshot(home.ID)
		if o.Node.Hostname == "office-node" && h.Node.Hostname == "home-node" {
			// Each core got its own directory and ports.
			if engines[office.ID].lastSpec().workDir == engines[home.ID].lastSpec().workDir {
				t.Fatal("two instances share a working directory")
			}
			if engines[office.ID].lastSpec().portal == engines[home.ID].lastSpec().portal {
				t.Fatal("two instances share a management port")
			}
			goto isolated
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("instances did not report their own state: %q / %q",
		manager.Snapshot(office.ID).Node.Hostname, manager.Snapshot(home.ID).Node.Hostname)
isolated:

	// Stopping one leaves the other alone.
	manager.Stop(office.ID)
	waitForPhase(t, manager.Controller(office.ID), PhaseStopped, 5*time.Second)
	if !manager.Running(home.ID) {
		t.Fatal("stopping one network stopped the other")
	}
}

// A profile that never ran reads as stopped, and stopping it is a no-op.
func TestManagerNeverStartedIsStopped(t *testing.T) {
	manager, _ := testManager(t)
	if got := manager.Snapshot("nobody").Phase; got != PhaseStopped {
		t.Fatalf("phase = %q", got)
	}
	manager.Stop("nobody") // must not panic
	if manager.AnyRunning() {
		t.Fatal("nothing runs yet")
	}
}

// Quitting stops every network, not just the one on screen.
func TestManagerStopAllAndWait(t *testing.T) {
	manager, engines := testManager(t)
	one := NewProfile("一")
	one.Config = testConfig()
	two := NewProfile("二")
	two.Config = testConfig()

	manager.Start(one)
	manager.Start(two)
	waitForPhase(t, manager.Controller(one.ID), PhaseRunning, 5*time.Second)
	waitForPhase(t, manager.Controller(two.ID), PhaseRunning, 5*time.Second)

	if err := manager.StopAllAndWait(5 * time.Second); err != nil {
		t.Fatal(err)
	}

	if manager.AnyRunning() {
		t.Fatal("a network survived StopAllAndWait")
	}
	if !engines[one.ID].newRun().wasStopped() || !engines[two.ID].newRun().wasStopped() {
		t.Fatal("not every core was asked to stop")
	}
}

func TestManagerStopAllSharesOneDeadline(t *testing.T) {
	manager, engines := testManager(t)
	profiles := []Profile{NewProfile("一"), NewProfile("二"), NewProfile("三")}
	for _, profile := range profiles {
		profile.Config = testConfig()
		manager.Controller(profile.ID).budgets.graceful = 10 * time.Millisecond
		engines[profile.ID].newRun().holdExit = true
		manager.Start(profile)
		waitForPhase(t, manager.Controller(profile.ID), PhaseRunning, 5*time.Second)
	}
	before := time.Now()
	if err := manager.StopAllAndWait(100 * time.Millisecond); err == nil {
		t.Fatal("unstoppable cores were reported as stopped")
	}
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("three cores used sequential stop timeouts: %v", elapsed)
	}
}

// A launch reconnects the networks that were enabled, skips ones that are
// gone or already running, and still tries the rest after a failure.
func TestManagerRestore(t *testing.T) {
	manager, engines := testManager(t)

	one := NewProfile("一")
	one.Config = testConfig()
	two := NewProfile("二")
	two.Config = testConfig()
	two.Config.NetworkName = "two-net"

	if err := manager.Restore([]Profile{one, two}, []string{one.ID, two.ID, "missing"}); err != nil {
		t.Fatal(err)
	}
	waitForPhase(t, manager.Controller(one.ID), PhaseRunning, 5*time.Second)
	waitForPhase(t, manager.Controller(two.ID), PhaseRunning, 5*time.Second)

	// Restoring again does not restart what already runs.
	if err := manager.Restore([]Profile{one, two}, []string{one.ID, two.ID}); err != nil {
		t.Fatal(err)
	}
	engines[one.ID].mu.Lock()
	starts := len(engines[one.ID].started)
	engines[one.ID].mu.Unlock()
	if starts != 1 {
		t.Fatalf("the running network was started again: %d starts", starts)
	}
}

// A device-mode Portal key made during restore is saved with the profile, so
// the WireGuard identity does not change on every launch.
func TestManagerRestoreKeepsThePortalKey(t *testing.T) {
	manager, _ := testManager(t)

	profile := NewProfile("门户")
	profile.Config = testConfig()
	profile.Config.VPNPortal = &VPNPortal{ClientCIDR: "10.14.14.0/24", Listen: "0.0.0.0:51820", DeviceMode: true}

	if err := manager.Restore([]Profile{profile}, []string{profile.ID}); err != nil {
		t.Fatal(err)
	}
	profiles, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, loaded := range profiles {
		if loaded.ID == profile.ID {
			if loaded.Config.VPNPortal == nil || loaded.Config.VPNPortal.PrivateKey == "" {
				t.Fatal("the generated portal key was not saved")
			}
			return
		}
	}
	t.Fatalf("the profile was not saved: %v", profiles)
}

// The instance name lands in the configuration the core reads, so the CLI
// can tell parallel instances apart.
func TestControllerNamesItsInstance(t *testing.T) {
	controller, engine := testController(t)
	controller.Start(testConfig())
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)
	spec := engine.lastSpec()
	data, err := os.ReadFile(spec.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `instance_name = "mygo-test"`) {
		t.Errorf("the instance name is missing:\n%s", data)
	}
}
