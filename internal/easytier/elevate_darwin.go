//go:build darwin

package easytier

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// isElevated reports whether this process can create a utun device.
func IsElevated() bool { return os.Geteuid() == 0 }

// relaunchElevated starts this application again with administrative rights
// through the system's authorization prompt, and returns once it is running.
//
// A mygo app is a bundle: its executable is Foo.app/Contents/MacOS/foo, which
// still shows its window when it runs as root in the user's session.
func RelaunchElevated(instanceRoot string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// The shell string needs the command quoted for the shell itself, and
	// the whole thing quoted for AppleScript.
	command := fmt.Sprintf("%s -instance-root %s", shellQuote(self), shellQuote(instanceRoot))
	script := fmt.Sprintf("do shell script %s with administrator privileges",
		strconv.Quote(command))
	process := exec.Command("/usr/bin/osascript", "-e", script)
	process.Stdin, process.Stdout, process.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := process.Start(); err != nil {
		return err
	}
	return nil
}

// shellQuote wraps a value in single quotes, escaping any it holds.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
