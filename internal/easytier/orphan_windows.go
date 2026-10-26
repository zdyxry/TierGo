//go:build windows

package easytier

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// stillActive is STILL_ACTIVE, which a process that is running reports as
// its exit code.
const stillActive = 259

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}

func killProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// processNamed reports whether a process runs the given program, read from
// the task list.
func processNamed(pid int, name string) (bool, error) {
	output, err := commandOutput("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(output), strings.ToLower(name+".exe")), nil
}

func syncDir(string) {
	// Windows has no unprivileged directory flush; the rename is atomic and
	// the contents are flushed before it.
}

func commandOutput(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).Output()
	return string(output), err
}
