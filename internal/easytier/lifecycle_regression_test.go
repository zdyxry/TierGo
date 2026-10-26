package easytier

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentStartReservesOnlyOneCore(t *testing.T) {
	ctl, engine := testController(t)
	var starts sync.WaitGroup
	for range 40 {
		starts.Go(func() { ctl.Start(testConfig()) })
	}
	starts.Wait()
	waitForPhase(t, ctl, PhaseRunning, 5*time.Second)
	engine.mu.Lock()
	count := len(engine.started)
	engine.mu.Unlock()
	if count != 1 {
		t.Fatalf("started %d cores", count)
	}
}
func TestImmediateStopDoesNotLoseCancellation(t *testing.T) {
	ctl, _ := testController(t)
	ctl.Start(testConfig())
	if err := ctl.StopAndWait(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if snap := ctl.Snapshot(); snap.Phase != PhaseStopped {
		t.Fatalf("immediate stop left %s: %s", snap.Phase, snap.Err)
	}
}

func TestControllerReportsProcessThatSurvivesKill(t *testing.T) {
	ctl, engine := testController(t)
	run := engine.newRun()
	run.holdExit = true
	ctl.budgets.graceful = 10 * time.Millisecond
	ctl.Start(testConfig())
	waitForPhase(t, ctl, PhaseRunning, 5*time.Second)
	before := time.Now()
	if err := ctl.StopAndWait(time.Second); err == nil {
		t.Fatal("a process that survived kill was reported as stopped")
	}
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("stop remained blocked for %v", elapsed)
	}
	if snap := ctl.Snapshot(); snap.Phase != PhaseFailed || snap.Err == "" {
		t.Fatalf("stop failure was not retained: %+v", snap)
	}
}

func TestProfileRejectsPathTraversal(t *testing.T) {
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	for _, id := range []string{"../escape", "..", "/tmp/escape", "foo/bar", `foo\bar`, ""} {
		p := NewProfile("bad")
		p.ID = id
		if SaveProfile(p) == nil {
			t.Fatalf("accepted id %q", id)
		}
		if DeleteProfile(id) == nil {
			t.Fatalf("delete accepted id %q", id)
		}
	}
}
