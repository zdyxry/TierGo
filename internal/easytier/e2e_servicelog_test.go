package easytier

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestE2EServiceLogInference runs a real easytier-core in Config Server mode
// against a dead server with file logging on, and checks that the tail scan
// reads the connection state out of the service log — the same inference the
// remote page shows for the system service. No privileges are needed: Config
// Server mode starts no network and no TUN by itself.
func TestE2EServiceLogInference(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test with the real easytier-core")
	}
	core, _ := requireBinaries(t)

	// A port that nothing listens on: every connect fails, and the web
	// client logs its retries instead of ever connecting.
	dead := func() int {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		return port
	}
	portal := freePortHere(t)

	logDir := t.TempDir()
	configDir := t.TempDir()
	cmd := exec.Command(core,
		"--config-server", fmt.Sprintf("tcp://127.0.0.1:%d/token", dead()),
		"--machine-id", NewMachineID(),
		"--config-dir", configDir,
		"--rpc-portal", fmt.Sprintf("127.0.0.1:%d", portal),
		"--file-log-level", "info",
		"--file-log-dir", logDir,
		"--console-log-level", "off",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// The core retries the dead server; its file log must end up reporting
	// the disconnected transport, with the deciding line and its time.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		status, err := scanConnectionLog(filepath.Join(logDir, serviceLogName))
		if err == nil && status.Known && !status.Connected {
			if status.Line == "" || status.At.IsZero() {
				t.Fatalf("the inference lacks its basis: %+v", status)
			}
			return
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Fatal("the core exited before writing connection lines")
		}
		time.Sleep(500 * time.Millisecond)
	}
	content, _ := os.ReadFile(filepath.Join(logDir, serviceLogName))
	t.Fatalf("the service log never reported the failed connection; log:\n%s", content)
}

func freePortHere(t *testing.T) int {
	t.Helper()
	port, err := freePort("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return port
}
