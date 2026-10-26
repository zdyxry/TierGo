package easytier

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The state machine is exercised against a scripted core, so it is tested
// without the official binaries and without waiting on a real network.

// waitForPhase waits until the controller reaches a phase, failing the test
// on a timeout or an unexpected failure.
func waitForPhase(t *testing.T, controller *Controller, phase Phase, timeout time.Duration) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		if snapshot.Phase == phase {
			return snapshot
		}
		if snapshot.Phase == PhaseFailed && phase != PhaseFailed {
			t.Fatalf("reached the failed phase: %s", snapshot.Err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("phase %q was not reached, still %q (%s)",
		phase, controller.Snapshot().Phase, controller.Snapshot().Err)
	return Snapshot{}
}

func hasLog(entries []LogEntry, contains string) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Message, contains) {
			return true
		}
	}
	return false
}

func TestControllerProbesBeforeRunning(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()
	run.failProbes(2) // the first two reads fail, as a slow core's would

	controller.Start(testConfig())
	snapshot := waitForPhase(t, controller, PhaseRunning, 5*time.Second)

	if snapshot.Node.Hostname != "fake-node" {
		t.Fatalf("node = %+v", snapshot.Node)
	}
	if snapshot.ConfigPath == "" {
		t.Error("the configuration path was not recorded")
	}
	// And the configuration it generated is on disk.
	data, err := os.ReadFile(snapshot.ConfigPath)
	if err != nil {
		t.Fatalf("configuration was not written durably: %v", err)
	}
	if !strings.Contains(string(data), `network_name = "test-net"`) {
		t.Errorf("configuration does not hold the network name:\n%s", data)
	}
}

// A core that starts but never answers must not be shown as connected.
func TestControllerFailsWhenTheCoreNeverAnswers(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()
	run.failReads(errors.New("connection refused"))
	controller.budgets.probe = 500 * time.Millisecond

	controller.Start(testConfig())
	snapshot := waitForPhase(t, controller, PhaseFailed, 5*time.Second)

	if !strings.Contains(snapshot.Err, "没有响应") {
		t.Errorf("failure reason = %q", snapshot.Err)
	}
	if !hasLog(snapshot.Logs, "没有响应") {
		t.Errorf("the failure was not logged: %v", snapshot.Logs)
	}
}

func TestControllerFailsWhenTheCoreNeverStarts(t *testing.T) {
	controller, engine := testController(t)
	engine.mu.Lock()
	engine.err = errors.New("exec: file not found")
	engine.mu.Unlock()

	controller.Start(testConfig())
	snapshot := waitForPhase(t, controller, PhaseFailed, 5*time.Second)
	if !strings.Contains(snapshot.Err, "file not found") {
		t.Errorf("failure reason = %q", snapshot.Err)
	}
}

// A core that dies on its own takes the window out of the connected state.
func TestControllerNoticesTheCoreDying(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()

	controller.Start(testConfig())
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)

	run.crash(errors.New("signal: killed"))

	snapshot := waitForPhase(t, controller, PhaseFailed, 5*time.Second)
	if !strings.Contains(snapshot.Err, "内核进程已退出") {
		t.Errorf("failure reason = %q", snapshot.Err)
	}
	// The failure stays on screen, but the run is over: nothing is holding
	// the process, and a new connection can be made.
	waitForUnwound(t, controller, 5*time.Second)
	if controller.Running() {
		t.Error("the controller still reports a running core")
	}
	if snapshot.Phase != PhaseFailed {
		t.Errorf("the failure reason was dropped: %q", snapshot.Phase)
	}
}

// What the core does not answer for is named, not silently replaced by the
// values of the last successful read.
func TestControllerReportsUnreadableParts(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()

	controller.Start(testConfig())
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)

	status := coreStatus{
		Node:       NodeView{Hostname: "fake-node"},
		Unreadable: []string{"节点列表", "路由表"},
	}
	run.setStatus(status)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := controller.Snapshot()
		if len(snapshot.Unreadable) == 2 {
			if snapshot.Node.Hostname != "fake-node" {
				t.Errorf("the readable part was lost: %+v", snapshot.Node)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("unreadable parts were not reported: %+v", controller.Snapshot())
}

// Repeated failure is the core being unhealthy, not "nothing to report".
func TestControllerFailsAfterRepeatedReadFailures(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()

	controller.Start(testConfig())
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)

	run.failReads(errors.New("rpc timeout"))

	snapshot := waitForPhase(t, controller, PhaseFailed, 5*time.Second)
	if !strings.Contains(snapshot.Err, "连续") {
		t.Errorf("failure reason = %q", snapshot.Err)
	}
}

// Stopping asks the core to leave; it is not killed while it cooperates.
func TestControllerStopsPolitely(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()

	controller.Start(testConfig())
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)

	controller.Stop()
	waitForPhase(t, controller, PhaseStopped, 5*time.Second)

	if !run.wasStopped() {
		t.Error("the core was not asked to stop")
	}
	if run.wasKilled() {
		t.Error("the core was killed although it stopped on its own")
	}
	// The pid file must not survive a clean stop: a stale one would make the
	// next start kill a process it does not own.
	root, err := InstanceRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PidFilePath(filepath.Join(root, "instances", "test"))); err == nil {
		t.Error("the pid file was left behind")
	}
}

// A line that keeps repeating is counted, not copied: the core repeats the
// same warnings every second while it looks for paths.
func TestLogRepeatsAreCounted(t *testing.T) {
	controller, _ := testController(t)
	for range 5 {
		controller.logf("warn", "bind device dev_idx=0 dev_name=%q", "en0")
	}
	logs := controller.Snapshot().Logs
	if len(logs) != 1 {
		t.Fatalf("the same line was logged %d times", len(logs))
	}
	if logs[0].Count != 5 {
		t.Errorf("count = %d, want 5", logs[0].Count)
	}

	// A different line starts a new entry.
	controller.logf("warn", "another warning")
	if got := len(controller.Snapshot().Logs); got != 2 {
		t.Errorf("a different line did not start an entry: %d", got)
	}
}

// The network secret must never reach the activity log.
// waitForUnwound waits until the run has released the core.
func waitForUnwound(t *testing.T, controller *Controller, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		controller.mu.Lock()
		cancel := controller.cancel
		controller.mu.Unlock()
		if cancel == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the controller never released the core")
}

// When the management port answers again after repeated failures, the
// window returns to connected on its own: a temporary RPC outage is not a
// reason to make the user reconnect.
func TestControllerRecoversWhenTheCoreAnswersAgain(t *testing.T) {
	controller, engine := testController(t)
	run := engine.newRun()

	controller.Start(testConfig())
	waitForPhase(t, controller, PhaseRunning, 5*time.Second)

	run.failReads(errors.New("rpc timeout"))
	waitForPhase(t, controller, PhaseFailed, 5*time.Second)

	run.failReads(nil)
	// The controller passes through the failed phase on the way back, so
	// wait for running without treating it as fatal.
	deadline := time.Now().Add(5 * time.Second)
	var snapshot Snapshot
	for time.Now().Before(deadline) {
		snapshot = controller.Snapshot()
		if snapshot.Phase == PhaseRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snapshot.Phase != PhaseRunning {
		t.Fatalf("never recovered, still %q (%s)", snapshot.Phase, snapshot.Err)
	}
	if snapshot.Err != "" {
		t.Errorf("the failure reason survived the recovery: %q", snapshot.Err)
	}
	if !hasLog(snapshot.Logs, "已恢复") {
		t.Errorf("the recovery was not logged: %v", snapshot.Logs)
	}
}

func TestLogRedactsTheNetworkSecret(t *testing.T) {
	rememberSecret("super-secret-value")
	got := redact("config loaded: network_secret = \"super-secret-value\"")
	if strings.Contains(got, "super-secret-value") {
		t.Fatalf("the secret was not redacted: %q", got)
	}
	if !strings.Contains(got, "****") {
		t.Errorf("nothing replaced the secret: %q", got)
	}
	// Short values are left alone, so redaction cannot mangle ordinary text.
	if redact("port 11010") != "port 11010" {
		t.Error("redaction changed unrelated text")
	}
}
