//go:build linux

package easytier

import (
	"errors"
	"os"
	"os/exec"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// isElevated reports whether this process can create a TUN device.
func IsElevated() bool { return os.Geteuid() == 0 }

// relaunchElevated starts this application again with administrative rights
// and returns once it is launched, so the caller can quit and let the
// elevated window take over.
func RelaunchElevated(instanceRoot string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	arguments := []string{self, "-instance-root", instanceRoot}
	// pkexec asks through the desktop's authentication agent, which is what
	// a windowed app should use; sudo is the fallback for a terminal.
	for _, command := range []struct {
		name string
		args []string
	}{
		{"pkexec", arguments},
		{"sudo", append([]string{"-n"}, arguments...)},
	} {
		path, err := exec.LookPath(command.name)
		if err != nil {
			continue
		}
		process := exec.Command(path, command.args...)
		process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := process.Start(); err != nil {
			return err
		}
		return nil
	}
	return errors.New(locale.T("找不到 pkexec 或 sudo，请用 sudo 手动启动"))
}
