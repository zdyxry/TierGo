package easytier

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCentralConfigValidationAndInstall(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)
	config := CentralConfig{Endpoint: "udp://127.0.0.1:22020/test", MachineID: NewMachineID()}
	if err := ServiceInstallCentral(context.Background(), config, ServiceSettings{}); err != nil {
		t.Fatal(err)
	}
	args := strings.Join((*calls)[0], " ")
	for _, want := range []string{"--config-server " + config.Endpoint, "--machine-id " + config.MachineID, "central-configs"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q: %s", want, args)
		}
	}
	if _, err := ServiceRPCAddress(); err != nil {
		t.Fatal(err)
	}
	config.MachineID = "bad"
	if config.Validate() == nil {
		t.Fatal("invalid machine ID accepted")
	}
	config.MachineID = NewMachineID()
	config.Endpoint = "http://example.com/x"
	if config.Validate() == nil {
		t.Fatal("invalid transport accepted")
	}
}
func TestE2ECentralArgumentsAcceptedByCore(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	core, _ := requireBinaries(t)
	config := CentralConfig{Endpoint: "udp://127.0.0.1:22020/test", MachineID: NewMachineID()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "central-configs"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "empty.toml")
	if err := os.WriteFile(path, []byte("listeners = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append(config.args(root, ""), "--config-file", path, "--check-config")
	if output, err := exec.CommandContext(ctx, core, args...).CombinedOutput(); err != nil {
		t.Fatalf("Core rejected central arguments: %v %s", err, output)
	}
}

// The transport state is inferred from Core lines; each change records when
// and on which (redacted) line, so the window can show its basis.
func TestObserveLineRecordsEvidence(t *testing.T) {
	controller := NewCentralController()
	config := CentralConfig{Endpoint: "udp://example.com:22020/token123", MachineID: NewMachineID()}

	controller.observeLine("2026-10-23T10:00:00+08:00 INFO web_client: Successfully connected to udp://example.com:22020/token123", config)
	snap := controller.Snapshot()
	if !snap.TransportConnected || snap.TransportAt.IsZero() {
		t.Fatalf("connected line not recorded: %+v", snap)
	}
	if strings.Contains(snap.TransportLine, "token123") || !strings.Contains(snap.TransportLine, "[Config Server]") {
		t.Fatalf("evidence line not redacted: %q", snap.TransportLine)
	}

	controller.observeLine("2026-10-23T10:05:00+08:00 WARN web_client: Failed to connect to the server, retrying in 5 seconds...", config)
	snap = controller.Snapshot()
	if snap.TransportConnected {
		t.Fatal("a retry line must drop the transport state")
	}
	if !strings.Contains(snap.TransportLine, "retrying") {
		t.Fatalf("the retry line is not the recorded basis: %q", snap.TransportLine)
	}
}

func TestCentralStopAndWaitReportsTimeout(t *testing.T) {
	controller := NewCentralController()
	_, cancel := context.WithCancel(context.Background())
	controller.cancel = cancel
	controller.done = make(chan struct{})
	if err := controller.StopAndWait(10 * time.Millisecond); err == nil {
		t.Fatal("an unfinished central core was reported as stopped")
	}
}
