package easytier

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeEngine runs a core that is scripted by the test, so the whole state
// machine — launch, readiness, probe, crash, unreadable status — is exercised
// without the official binaries.
type fakeEngine struct {
	mu      sync.Mutex
	started []coreSpec
	run     *fakeRun
	err     error
}

func (e *fakeEngine) start(spec coreSpec) (coreRun, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.started = append(e.started, spec)
	if e.err != nil {
		return nil, e.err
	}
	if e.run == nil {
		e.run = newFakeRun()
	}
	return e.run, nil
}

func (e *fakeEngine) lastSpec() coreSpec {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.started) == 0 {
		return coreSpec{}
	}
	return e.started[len(e.started)-1]
}

type fakeRun struct {
	mu       sync.Mutex
	status   coreStatus
	readErr  error
	readyErr error
	probes   int // reads that fail before one succeeds
	reads    int
	stopped  bool
	killed   bool
	holdExit bool
	lineCh   chan string
	exitCh   chan struct{}
	exitErr  error
	gone     bool
}

func newFakeRun() *fakeRun {
	return &fakeRun{
		status: coreStatus{Node: NodeView{Hostname: "fake-node", IPv4: "10.1.2.3/24", Version: "0.0.0-fake"}},
		lineCh: make(chan string, 64),
		exitCh: make(chan struct{}),
	}
}

func (r *fakeRun) portal() string          { return "127.0.0.1:1" }
func (r *fakeRun) lines() <-chan string    { return r.lineCh }
func (r *fakeRun) exited() <-chan struct{} { return r.exitCh }
func (r *fakeRun) pid() int                { return 4242 }

func (r *fakeRun) ready(context.Context) error { return r.readyErr }

func (r *fakeRun) wait() error {
	<-r.exitCh
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exitErr
}

func (r *fakeRun) read(context.Context) (coreStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.probes > 0 {
		r.probes--
		return coreStatus{}, errors.New("还没有准备好")
	}
	if r.readErr != nil {
		return coreStatus{}, r.readErr
	}
	return r.status, nil
}

func (r *fakeRun) stop() {
	r.mu.Lock()
	r.stopped = true
	hold := r.holdExit
	r.mu.Unlock()
	if !hold {
		r.crash(nil)
	}
}

func (r *fakeRun) kill() {
	r.mu.Lock()
	r.killed = true
	hold := r.holdExit
	r.mu.Unlock()
	if !hold {
		r.crash(errors.New("killed"))
	}
}

// crash makes the core exit, as a real one would when it dies.
func (r *fakeRun) crash(err error) {
	r.mu.Lock()
	if r.gone {
		r.mu.Unlock()
		return
	}
	r.gone, r.exitErr = true, err
	r.mu.Unlock()
	close(r.exitCh)
	close(r.lineCh)
}

func (r *fakeRun) log(message string) { r.lineCh <- message }

func (r *fakeRun) wasStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

func (r *fakeRun) wasKilled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.killed
}

// setStatus replaces what the next probe reports.
func (r *fakeRun) setStatus(status coreStatus) {
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
}

func (r *fakeRun) failReads(err error) {
	r.mu.Lock()
	r.readErr = err
	r.mu.Unlock()
}

func (r *fakeRun) failProbes(n int) {
	r.mu.Lock()
	r.probes = n
	r.mu.Unlock()
}

// newRun is the core the engine will hand out, made before it is started so
// a test can script it.
func (e *fakeEngine) newRun() *fakeRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.run == nil {
		e.run = newFakeRun()
	}
	return e.run
}

// testController is a controller wired to a scripted core, with budgets
// short enough for tests.
func testController(t *testing.T) (*Controller, *fakeEngine) {
	t.Helper()
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))

	engine := &fakeEngine{}
	controller := NewController("test")
	controller.engine = engine
	controller.budgets = budgets{
		portal:   2 * time.Second,
		probe:    2 * time.Second,
		graceful: time.Second,
		poll:     30 * time.Millisecond,
	}
	t.Cleanup(func() { controller.StopAndWait(2 * time.Second) })
	return controller, engine
}

func testConfig() Config {
	config := DefaultConfig()
	config.NetworkName = "test-net"
	config.NetworkSecret = "test-secret"
	config.Hostname = "test-host"
	config.EnableTUN = false
	return config
}
