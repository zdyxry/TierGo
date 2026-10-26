package easytier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

type CentralSnapshot struct {
	Phase         Phase
	Portal, Error string
	// TransportConnected follows Core's WebClient transport events. It is
	// deliberately distinct from server-side authenticated heartbeat evidence.
	TransportConnected bool
	// TransportAt and TransportLine record when the transport state last
	// changed and the sanitized Core log line it was inferred from, so the
	// window can show the basis of the claim instead of a bare boolean.
	TransportAt   time.Time
	TransportLine string
	Logs          []string
}
type CentralController struct {
	mu      sync.Mutex
	snap    CentralSnapshot
	cancel  context.CancelFunc
	done    chan struct{}
	stopErr error
}

func NewCentralController() *CentralController {
	return &CentralController{snap: CentralSnapshot{Phase: PhaseStopped}}
}
func (c *CentralController) Snapshot() CentralSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.snap
	s.Logs = append([]string(nil), s.Logs...)
	return s
}

// SetSnapshotForTest installs a state without running a core, so the window
// can be tested against it.
func (c *CentralController) SetSnapshotForTest(snapshot CentralSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snap = snapshot
}
func (c *CentralController) Start(config CentralConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		return fmt.Errorf("%s", locale.T("集中管理已运行"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	c.stopErr = nil
	c.snap = CentralSnapshot{Phase: PhaseStarting}
	go c.run(ctx, config, c.done)
	return nil
}
func (c *CentralController) StopAndWait(timeout time.Duration) error {
	c.mu.Lock()
	cancel, done, stopErr := c.cancel, c.done, c.stopErr
	if cancel != nil {
		c.snap.Phase = PhaseStopping
	}
	c.mu.Unlock()
	if cancel == nil {
		return stopErr
	}
	cancel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.stopErr
	case <-timer.C:
		return fmt.Errorf("%s", locale.T("停止集中管理超时"))
	}
}
func (c *CentralController) run(ctx context.Context, config CentralConfig, done chan struct{}) {
	defer func() {
		c.mu.Lock()
		c.cancel = nil
		if c.snap.Phase != PhaseFailed {
			c.snap.Phase = PhaseStopped
		}
		c.snap.TransportConnected = false
		c.mu.Unlock()
		close(done)
	}()
	fail := func(err error) {
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		c.snap.Phase = PhaseFailed
		c.snap.Error = redact(err.Error())
		c.mu.Unlock()
	}
	root, err := InstanceRoot()
	if err != nil {
		fail(err)
		return
	}
	root = filepath.Join(root, "central") // never share PID/configs with service mode
	if err := os.MkdirAll(filepath.Join(root, "central-configs"), 0700); err != nil {
		fail(err)
		return
	}
	core, _, _, err := LocateBinaries(root)
	if err != nil {
		fail(err)
		return
	}
	port, err := freePort("127.0.0.1")
	if err != nil {
		fail(err)
		return
	}
	portal := fmt.Sprintf("127.0.0.1:%d", port)
	// Capture WebClient lifecycle at info in bounded memory. Never write info
	// logs to disk or claim socket connectivity proves authenticated heartbeat.
	args := append(config.args(root, ""), "--rpc-portal", portal, "--console-log-level", "info", "--file-log-level", "off")
	if ctx.Err() != nil {
		return
	}
	reapOrphan(root, func(string, string, ...any) {})
	process, err := startCoreProcess(core, args, root)
	if err != nil {
		fail(err)
		return
	}
	writePIDFile(root, process.pid())
	var logs sync.WaitGroup
	logs.Add(1)
	go func() {
		defer logs.Done()
		for line := range process.output() {
			c.observeLine(line, config)
		}
	}()
	defer func() {
		process.Stop()
		stopped := true
		select {
		case <-process.done:
		case <-time.After(2 * time.Second):
			process.Kill()
			select {
			case <-process.done:
			case <-time.After(2 * time.Second):
				stopped = false
				err := fmt.Errorf("%s", locale.T("强制结束后 easytier-core 仍未退出"))
				c.mu.Lock()
				c.stopErr = err
				c.snap.Phase = PhaseFailed
				c.snap.Error = err.Error()
				c.mu.Unlock()
			}
		}
		if stopped {
			logs.Wait()
			removePIDFile(root)
		}
	}()
	if err := waitForPortal(ctx, portal, 10*time.Second); err != nil {
		fail(err)
		return
	}
	c.mu.Lock()
	c.snap.Phase = PhaseRunning
	c.snap.Portal = portal
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return
	case <-process.done:
		fail(fmt.Errorf(locale.T("集中管理 Core 已退出：%v"), process.Wait()))
	}
}
func (c *CentralController) observeLine(line string, config CentralConfig) {
	// Endpoint contains the enrollment token; private keys can occur in Core
	// config dumps. Only retain lifecycle lines, never arbitrary stdout.
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "server") && !strings.Contains(lower, "heartbeat") && !strings.Contains(lower, "tunnel") && !strings.Contains(lower, "successfully connected") {
		return
	}
	safe := strings.ReplaceAll(line, config.Endpoint, "[Config Server]")
	parts := strings.Split(strings.TrimRight(config.Endpoint, "/"), "/")
	token := parts[len(parts)-1]
	if token != "" {
		safe = strings.ReplaceAll(safe, token, "[token]")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// The classifier is shared with the service-log scan and matches both
	// the v2.6.4 and the main phrasing of the WebClient lifecycle lines.
	switch classifyConnectionLine(lower) {
	case 1:
		if !c.snap.TransportConnected {
			c.snap.TransportConnected = true
			c.snap.TransportAt = time.Now()
			c.snap.TransportLine = safe
		}
	case -1:
		if c.snap.TransportConnected {
			c.snap.TransportConnected = false
			c.snap.TransportAt = time.Now()
			c.snap.TransportLine = safe
		}
	}
	c.snap.Logs = append(c.snap.Logs, safe)
	if len(c.snap.Logs) > 100 {
		c.snap.Logs = c.snap.Logs[len(c.snap.Logs)-100:]
	}
}
