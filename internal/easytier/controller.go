package easytier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/zdyxry/easytier-mygo/internal/locale"
	"github.com/zdyxry/easytier-mygo/internal/owner"
)

// Phase is the lifecycle of the EasyTier core the window shows.
type Phase string

const (
	PhaseStopped  Phase = "stopped"
	PhaseStarting Phase = "starting"
	PhaseRunning  Phase = "running"
	PhaseStopping Phase = "stopping"
	PhaseFailed   Phase = "failed"
)

// LogEntry is one line of the activity log. Repeats of the same line raise
// Count instead of filling the log with copies.
type LogEntry struct {
	At      time.Time
	Kind    string
	Message string
	Count   int
}

// NodeView is this device as the network sees it.
type NodeView struct {
	Hostname                       string
	IPv4                           string
	PeerID                         string
	Version                        string
	InstID                         string
	Listeners                      []string
	ProxyCIDRs                     []string
	PublicIPs                      []string
	UDPNATType                     int
	TCPNATType                     int
	InterfaceIPv4s, InterfaceIPv6s []string
	PublicIPv6, TUNName            string
}

// PeerView is one other device in the network.
type PeerView struct {
	Name                     string
	IPv4                     string
	PeerID                   string
	LatencyMS                float64
	LatencyText              string
	LossRate                 string
	RxBytes                  string
	TxBytes                  string
	Tunnel                   string
	NATType                  string
	Version                  string
	Direct                   bool
	Local                    bool
	PublicServer, AvoidRelay bool
}

// RouteView is one entry of the overlay routing table.
type RouteView struct {
	Dest        string
	NextHop     string
	NextHopName string
	Hops        int
	LatencyMS   float64
	Name        string
	ProxyCIDRs  string
	Version     string
}

// Snapshot is the state the view draws. It is a copy, safe to read on the UI
// thread while the controller keeps working.
type Snapshot struct {
	Phase Phase
	Err   string
	// EmbeddedCore is the version of the core this build carries, empty
	// when it uses one installed on the machine.
	EmbeddedCore string
	// Unreadable names the parts of the state the core did not answer for.
	// The window says so instead of showing old values as current ones.
	Unreadable []string
	TUN        bool
	ConfigPath string
	LogPath    string
	Portal     string
	Node       NodeView
	Peers      []PeerView
	Routes     []RouteView
	Logs       []LogEntry
	Since      time.Time
	Traffic    []TrafficSample
	Events     []NetworkEvent
	// Counters are the raw per-connection byte counters the Traffic rates
	// are diffed from. Only remote status reads carry them: the local
	// controller keeps its baseline internally.
	Counters map[string]ConnCounter
}

const maxLogEntries = 400

// budgets are the time limits the controller works with, in one place.
type budgets struct {
	portal   time.Duration // the management port must answer within this
	probe    time.Duration // the first complete status read must succeed
	graceful time.Duration // the core may take this long to exit on its own
	poll     time.Duration // how often the state is refreshed
}

var defaultBudgets = budgets{
	portal:   30 * time.Second,
	probe:    30 * time.Second,
	graceful: 5 * time.Second,
	poll:     2 * time.Second,
}

// unreadableLimit is how many refreshes in a row may fail before the core is
// treated as unhealthy rather than merely slow.
const unreadableLimit = 3

// Controller owns the easytier-core process of one network profile. Its
// methods are safe from any goroutine; the view reads snapshots.
type Controller struct {
	win *mygo.Window

	// key identifies the instance: its working directory and the name the
	// core runs under.
	key     string
	engine  coreEngine
	budgets budgets

	mu       sync.Mutex
	snap     Snapshot
	current  coreRun
	cancel   context.CancelFunc
	stopErr  error
	failures int
	counters map[string]ConnCounter
	sampleAt time.Time
	observed bool
}

func NewController(key string) *Controller {
	return &Controller{
		key:     key,
		engine:  nativeEngine{},
		budgets: defaultBudgets,
		snap:    Snapshot{Phase: PhaseStopped},
	}
}

// SetSnapshotForTest installs a state without running a core, so the window
// can be drawn and checked where a real network would be needed to reach it.
func (ctl *Controller) SetSnapshotForTest(snapshot Snapshot) {
	ctl.mu.Lock()
	ctl.snap = snapshot
	ctl.mu.Unlock()
}

// AttachWindow lets the controller redraw the window when state changes.
func (ctl *Controller) AttachWindow(win *mygo.Window) {
	ctl.mu.Lock()
	ctl.win = win
	ctl.mu.Unlock()
}

func (ctl *Controller) Snapshot() Snapshot {
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	snap := ctl.snap
	snap.Traffic = append([]TrafficSample(nil), ctl.snap.Traffic...)
	snap.Events = append([]NetworkEvent(nil), ctl.snap.Events...)
	snap.Logs = append([]LogEntry(nil), ctl.snap.Logs...)
	snap.Peers = append([]PeerView(nil), ctl.snap.Peers...)
	snap.Routes = append([]RouteView(nil), ctl.snap.Routes...)
	snap.Unreadable = append([]string(nil), ctl.snap.Unreadable...)
	return snap
}

// Running reports whether the core is up or on its way.
func (ctl *Controller) Running() bool {
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	switch ctl.snap.Phase {
	case PhaseStarting, PhaseRunning:
		return true
	}
	return false
}

// Start brings the core up in the background.
func (ctl *Controller) Start(config Config) {
	ctl.mu.Lock()
	if ctl.cancel != nil {
		ctl.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctl.cancel = cancel // reserve the run before scheduling the goroutine
	ctl.snap = Snapshot{Phase: PhaseStarting, Since: time.Now()}
	ctl.counters, ctl.sampleAt, ctl.observed = nil, time.Time{}, false
	ctl.failures = 0
	ctl.stopErr = nil
	ctl.mu.Unlock()
	ctl.publish()
	go ctl.run(ctx, cancel, config)
}

// Stop asks the core to exit in the background.
func (ctl *Controller) Stop() {
	ctl.mu.Lock()
	cancel := ctl.cancel
	if cancel != nil {
		ctl.snap.Phase = PhaseStopping
	}
	ctl.mu.Unlock()
	if cancel == nil {
		return
	}
	ctl.publish()
	cancel()
}

// StopAndWait stops the core and waits until it is gone, for a quit that must
// not leave a TUN device behind.
func (ctl *Controller) StopAndWait(timeout time.Duration) error {
	ctl.Stop()
	deadline := time.Now().Add(timeout)
	for {
		ctl.mu.Lock()
		running, err := ctl.cancel != nil, ctl.stopErr
		ctl.mu.Unlock()
		if !running {
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf(locale.T("停止实例 %s 超时"), ctl.key)
		}
		time.Sleep(min(50*time.Millisecond, remaining))
	}
}

// ClearLogs empties the activity log.
func (ctl *Controller) ClearLogs() {
	ctl.mu.Lock()
	ctl.snap.Logs = nil
	ctl.mu.Unlock()
	ctl.publish()
}

func (ctl *Controller) run(ctx context.Context, cancel context.CancelFunc, config Config) {
	defer func() {
		cancel()
		ctl.mu.Lock()
		ctl.cancel, ctl.current = nil, nil
		// A failure keeps its reason on screen until the user connects
		// again; only a stop that worked returns to "stopped".
		if ctl.snap.Phase != PhaseFailed {
			ctl.snap.Phase = PhaseStopped
		}
		ctl.failures = 0
		ctl.mu.Unlock()
		ctl.publish()
	}()

	fail := func(err error) {
		if ctx.Err() != nil {
			return
		}
		ctl.logf("error", "%v", err)
		ctl.mu.Lock()
		ctl.snap.Phase = PhaseFailed
		ctl.snap.Err = err.Error()
		ctl.mu.Unlock()
		ctl.publish()
	}

	if ctx.Err() != nil {
		return
	}
	if !safeInstanceKey(ctl.key) {
		fail(errors.New(locale.T("无效实例 ID")))
		return
	}
	root, err := InstanceRoot()
	if err != nil {
		fail(err)
		return
	}

	// 1. The core's own tools: this build's embedded copy first, so a
	// release needs nothing installed, then whatever the machine has.
	core, cli, embeddedVersion, err := LocateBinaries(root)
	if err != nil {
		fail(err)
		return
	}
	if err := checkPortalVersion(ctx, core, config.VPNPortal); err != nil {
		fail(err)
		return
	}
	if embeddedVersion != "" {
		ctl.mu.Lock()
		ctl.snap.EmbeddedCore = embeddedVersion
		ctl.mu.Unlock()
		ctl.logf("info", locale.T("使用内嵌的 easytier-core %s"), embeddedVersion)
	}

	// 2. Ports of our own: the defaults may belong to another EasyTier.
	listenPort, portalPort, err := pickPorts(config.Listeners)
	if err != nil {
		fail(fmt.Errorf(locale.T("找不到可用端口：%w"), err))
		return
	}
	portal := fmt.Sprintf("127.0.0.1:%d", portalPort)

	// 3. The instance's own directory, with the configuration the core
	// reads. Every profile gets one of its own, so parallel networks never
	// share a pid file, a log or a config.
	workDir := filepath.Join(root, "instances", ctl.key)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		fail(fmt.Errorf(locale.T("创建实例目录失败：%w"), err))
		return
	}
	owner.Keep(root, workDir)
	secretValues := []string{config.NetworkSecret}
	if config.VPNPortal != nil {
		secretValues = append(secretValues, config.VPNPortal.PrivateKey)
	}
	if config.ACL != nil {
		for _, group := range config.ACL.V1.Group.Declares {
			secretValues = append(secretValues, group.Secret)
		}
	}
	replaceSecrets(ctl.key, secretValues...)
	configPath := filepath.Join(workDir, "config.toml")
	document, err := buildInstanceTOML(config, listenPort, instanceName(ctl.key))
	if err != nil {
		fail(err)
		return
	}
	if err := writeFileDurable(configPath, []byte(document), 0o600); err != nil {
		fail(fmt.Errorf(locale.T("写入配置失败：%w"), err))
		return
	}
	logDir := filepath.Join(workDir, "logs")

	// A core left behind by a crash holds the TUN device and the ports;
	// end it before starting a new one.
	reapOrphan(workDir, ctl.logf)

	ctl.logf("info", locale.T("配置已写入 %s"), configPath)
	ctl.mu.Lock()
	ctl.snap.ConfigPath = configPath
	ctl.snap.LogPath = logDir
	ctl.snap.Portal = portal
	ctl.mu.Unlock()

	// 4. The TUN device is the core's to create, and it needs
	// administrative rights. Without them it still joins the network and
	// forwards ports, so this is reported rather than fatal.
	if config.EnableTUN && !IsElevated() {
		fail(errors.New(locale.T("创建虚拟网卡需要管理员权限，请在设置里关闭虚拟网卡后重试，或以管理员身份重启客户端")))
		return
	}
	if config.EnableTUN {
		ctl.mu.Lock()
		ctl.snap.TUN = true
		ctl.mu.Unlock()
	}

	// 5. Launch.
	spec := coreSpec{
		core:         core,
		cli:          cli,
		workDir:      workDir,
		configPath:   configPath,
		logDir:       logDir,
		portal:       portal,
		readyTimeout: ctl.budgets.portal,
		args: []string{
			"--config-file", configPath,
			"--rpc-portal", portal,
			"--console-log-level", config.coreLogLevel(),
			// The log file is for looking at after something went wrong, so
			// it is bounded: the core's default is 100 MB per file and ten
			// files, which is a lot of writing for a background client.
			"--file-log-dir", logDir,
			"--file-log-level", config.coreLogLevel(),
			"--file-log-size", "5",
			"--file-log-count", "2",
		},
	}
	if ctx.Err() != nil {
		return
	}
	current, err := ctl.engine.start(spec)
	if err != nil {
		fail(fmt.Errorf(locale.T("启动 easytier-core 失败：%w"), err))
		return
	}
	ctl.mu.Lock()
	ctl.current = current
	ctl.mu.Unlock()
	writePIDFile(workDir, current.pid())

	// 6. Its log, and a watch on its exit: a core that dies must not leave
	// the window showing a connection that is gone.
	go ctl.forwardLogs(current)
	go ctl.watch(ctx, current)

	defer func() {
		current.stop()
		stopped := true
		select {
		case <-current.exited():
		case <-time.After(ctl.budgets.graceful):
			current.kill()
			select {
			case <-current.exited():
			case <-time.After(ctl.budgets.graceful):
				stopped = false
				err := fmt.Errorf("%s", locale.T("强制结束后 easytier-core 仍未退出"))
				ctl.mu.Lock()
				ctl.stopErr = err
				ctl.snap.Phase = PhaseFailed
				ctl.snap.Err = err.Error()
				ctl.mu.Unlock()
				ctl.logf("error", "%v", err)
			}
		}
		if stopped {
			removePIDFile(workDir)
		}
	}()

	// 7. The management port, then a real probe. Answering the port is not
	// the same as working, so the state is only published once a complete
	// status read has succeeded.
	if err := current.ready(ctx); err != nil {
		fail(fmt.Errorf(locale.T("easytier-core 没有就绪：%w"), err))
		return
	}
	status, err := ctl.probe(ctx, current)
	if err != nil {
		fail(fmt.Errorf(locale.T("easytier-core 已启动但没有响应：%w"), err))
		return
	}
	ctl.logf("info", locale.T("easytier-core 已启动（管理端口 %s）"), portal)

	ctl.mu.Lock()
	if ctx.Err() != nil {
		ctl.mu.Unlock()
		return
	}
	ctl.snap.Phase = PhaseRunning
	ctl.snap.Err = ""
	ctl.applyLocked(status)
	ctl.mu.Unlock()
	ctl.publish()

	// 8. Keep it fresh.
	ctl.poll(ctx, current)

	ctl.logf("info", "%s", locale.T("正在断开…"))
}

// watch notices the core dying on its own and takes the window out of the
// connected state with it.
func (ctl *Controller) watch(ctx context.Context, run coreRun) {
	err := run.wait()
	select {
	case <-ctx.Done():
		return // stopped on purpose
	default:
	}

	message := locale.T("内核进程已退出")
	if err != nil {
		message = fmt.Sprintf(locale.T("内核进程已退出：%v"), err)
	}
	ctl.logf("error", "%s", message)

	ctl.mu.Lock()
	if ctl.snap.Phase == PhaseRunning || ctl.snap.Phase == PhaseStarting {
		ctl.snap.Phase = PhaseFailed
		ctl.snap.Err = message
	}
	cancel := ctl.cancel
	ctl.mu.Unlock()
	ctl.publish()

	// Unwind the poll loop and the run's cleanup.
	if cancel != nil {
		cancel()
	}
}

func (ctl *Controller) poll(ctx context.Context, run coreRun) {
	ticker := time.NewTicker(ctl.budgets.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctl.refresh(ctx, run)
		}
	}
}

// probe waits for the first complete status read.
func (ctl *Controller) probe(ctx context.Context, run coreRun) (coreStatus, error) {
	deadline := time.Now().Add(ctl.budgets.probe)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return coreStatus{}, err
		}
		status, err := run.read(ctx)
		if err == nil && status.Node.Hostname != "" {
			return status, nil
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New(locale.T("内核没有返回节点信息"))
	}
	return coreStatus{}, lastErr
}

// refresh reads the current state, and treats repeated failure as the core
// being unhealthy rather than as nothing to report.
func (ctl *Controller) refresh(ctx context.Context, run coreRun) {
	status, err := run.read(ctx)

	ctl.mu.Lock()
	if err != nil {
		ctl.failures++
		ctl.snap.Unreadable = []string{locale.T("节点信息")}
		if ctl.failures >= unreadableLimit && ctl.snap.Phase == PhaseRunning {
			ctl.snap.Phase = PhaseFailed
			ctl.snap.Err = fmt.Sprintf(locale.T("连续 %d 次读取内核状态失败：%v"), ctl.failures, err)
			message := ctl.snap.Err
			ctl.mu.Unlock()
			ctl.logf("error", "%s", message)
			return
		}
		ctl.mu.Unlock()
		return
	}
	ctl.failures = 0
	// A run that was declared failed for unreadable status comes back to
	// life when the management port answers again: the connection to the
	// core recovers on its own, without the user reconnecting.
	recovered := ctl.snap.Phase == PhaseFailed && ctl.cancel != nil
	if recovered {
		ctl.snap.Phase = PhaseRunning
		ctl.snap.Err = ""
	}
	ctl.applyLocked(status)
	ctl.mu.Unlock()
	if recovered {
		ctl.logf("info", "%s", locale.T("已恢复与内核管理端口的连接"))
	}
	ctl.publish()
}

// applyLocked stores a probed status. The caller holds the lock.
func (ctl *Controller) applyLocked(status coreStatus) {
	ctl.monitorLocked(status, time.Now())
	ctl.snap.Node = status.Node
	ctl.snap.Peers = status.Peers
	ctl.snap.Routes = status.Routes
	ctl.snap.Unreadable = status.Unreadable
}

// repeatWindow is how long the same line counts as a repeat of the one
// before it.
const repeatWindow = 30 * time.Second

func (ctl *Controller) logf(kind string, format string, args ...any) {
	message := redact(fmt.Sprintf(format, args...))
	now := time.Now()

	ctl.mu.Lock()
	if count := len(ctl.snap.Logs); count > 0 {
		last := &ctl.snap.Logs[count-1]
		if last.Kind == kind && last.Message == message && now.Sub(last.At) < repeatWindow {
			if last.Count == 0 {
				last.Count = 1
			}
			last.Count++
			last.At = now
			ctl.mu.Unlock()
			return
		}
	}
	ctl.snap.Logs = append(ctl.snap.Logs, LogEntry{
		At: now, Kind: kind, Message: message, Count: 1,
	})
	if len(ctl.snap.Logs) > maxLogEntries {
		ctl.snap.Logs = ctl.snap.Logs[len(ctl.snap.Logs)-maxLogEntries:]
	}
	ctl.mu.Unlock()
	ctl.publish()
}

// forwardLogs turns the core's output into log entries.
func (ctl *Controller) forwardLogs(run coreRun) {
	for line := range run.lines() {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		level, message := parseLogLine(line)
		ctl.logf(level, "%s", message)
	}
}

func (ctl *Controller) publish() {
	ctl.mu.Lock()
	win := ctl.win
	ctl.mu.Unlock()
	if win != nil {
		win.Invalidate()
	}
}
