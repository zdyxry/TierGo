//go:build unix

package easytier

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func processAlive(pid int) bool {
	// Signal 0 asks the kernel whether the process exists, without
	// delivering anything.
	return syscall.Kill(pid, 0) == nil
}

func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// processNamed reports whether a process runs the given program.
func processNamed(pid int, name string) (bool, error) {
	if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		return strings.TrimSpace(string(comm)) == name, nil
	}
	// macOS and the BSDs have no /proc; ps is always there.
	output, err := commandOutput("ps", "-o", "comm=", "-p", fmt.Sprint(pid))
	if err != nil {
		return false, err
	}
	comm := strings.TrimSpace(output)
	if comm == "" {
		return false, nil
	}
	return strings.TrimPrefix(filepath.Base(comm), "(") == name ||
		strings.Contains(comm, name), nil
}

func syncDir(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	defer handle.Close()
	_ = handle.Sync()
}

// commandOutput runs a command and returns its standard output.
func commandOutput(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).Output()
	return string(output), err
}
