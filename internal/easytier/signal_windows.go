//go:build windows

package easytier

// Windows has no SIGTERM: ending the core is the only way to stop it, and it
// removes its TUN device and routes when it exits.
func (p *coreProcess) stopSoft() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}
