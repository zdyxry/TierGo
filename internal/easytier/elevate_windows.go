//go:build windows

package easytier

import (
	"errors"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// isElevated reports whether this process can create a TUN device.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// relaunchElevated starts this application again through the UAC prompt and
// returns once it is launched.
func RelaunchElevated(instanceRoot string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err := windows.UTF16PtrFromString(self)
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString("-instance-root " + quoteArgument(instanceRoot))
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, executable, parameters, nil, windows.SW_SHOWNORMAL); err != nil {
		return errors.New(locale.T("用户取消了授权，或无法以管理员身份启动：") + err.Error())
	}
	return nil
}

// quoteArgument wraps a value for a Windows command line, as
// CommandLineToArgvW reads it back.
func quoteArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\"") {
		return value
	}
	var builder strings.Builder
	builder.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		switch r {
		case '\\':
			backslashes++
			builder.WriteRune(r)
		case '"':
			builder.WriteString(strings.Repeat("\\", backslashes+1))
			builder.WriteRune('"')
			backslashes = 0
		default:
			backslashes = 0
			builder.WriteRune(r)
		}
	}
	builder.WriteString(strings.Repeat("\\", backslashes))
	builder.WriteByte('"')
	return builder.String()
}

var _ = unsafe.Pointer(nil)
