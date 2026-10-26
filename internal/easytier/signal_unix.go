//go:build !windows

package easytier

import "syscall"

// stopSoft asks the core to exit, so it removes its TUN device and routes.
func (p *coreProcess) stopSoft() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}
}
