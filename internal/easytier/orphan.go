package easytier

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// A core that outlives the window holds the TUN device, the ports and the
// overlay address. The client records the process it started, so a run that
// begins after a crash can end it before starting a new one.

func PidFilePath(workDir string) string { return filepath.Join(workDir, "core.pid") }

func writePIDFile(workDir string, pid int) {
	if pid <= 0 {
		return
	}
	// Best effort: a missing pid file only loses the cleanup.
	_ = writeFileDurable(PidFilePath(workDir), []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func removePIDFile(workDir string) {
	_ = os.Remove(PidFilePath(workDir))
}

// reapOrphan ends a core left behind by an earlier run.
//
// It only touches a process whose name is easytier-core, so a recycled
// process number cannot make it kill something else. Where the name cannot be
// read it does nothing and says so.
func reapOrphan(workDir string, logf func(kind, format string, args ...any)) {
	data, err := os.ReadFile(PidFilePath(workDir))
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return
	}
	if !processAlive(pid) {
		removePIDFile(workDir)
		return
	}
	matches, err := processNamed(pid, coreName)
	switch {
	case err != nil:
		logf("warn", locale.T("发现上次遗留的内核进程 %d，但无法确认它的身份，未处理：%v"), pid, err)
		return
	case !matches:
		// The number now belongs to something else.
		removePIDFile(workDir)
		return
	}
	if err := killProcess(pid); err != nil {
		logf("warn", locale.T("结束上次遗留的内核进程 %d 失败：%v"), pid, err)
		return
	}
	logf("warn", locale.T("已结束上次遗留的内核进程 %d"), pid)
	removePIDFile(workDir)
}

var _ = fmt.Sprint
