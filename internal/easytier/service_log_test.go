package easytier

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyConnectionLine(t *testing.T) {
	for line, want := range map[string]int{
		// v2.6.4 phrasing.
		"successfully connected to tunnel{...}":                     1,
		"failed to connect to the server, retrying in 5 seconds...": -1,
		"heartbeat failed: timeout":                                 -1,
		// main phrasing.
		"connected to config server":                                1,
		"failed to connect to config server; retrying":              -1,
		"failed to reconnect secure config-server tunnel; retrying": -1,
		// Unrelated lines.
		"new connection from peer":   0,
		"listening on tcp://0.0.0.0": 0,
	} {
		if got := classifyConnectionLine(line); got != want {
			t.Errorf("classifyConnectionLine(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestScanConnectionLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, serviceLogName)
	log := `2026-10-23T10:00:00.000000+08:00  INFO easytier: starting
2026-10-23T10:00:05.000000+08:00  INFO easytier::web_client: Successfully connected to tunnel{tcp}
2026-10-23T10:05:00.000000+08:00  WARN easytier::web_client: Failed to connect to the server, retrying in 5 seconds...
`
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := scanConnectionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Found || !status.Known || status.Connected {
		t.Fatalf("status = %+v", status)
	}
	if !strings.Contains(status.Line, "retrying") {
		t.Fatalf("deciding line = %q", status.Line)
	}
	if status.At.Hour() != 10 || status.At.Minute() != 5 {
		t.Fatalf("deciding time = %v", status.At)
	}

	// The newest connection line wins.
	log += "2026-10-23T10:06:00.000000+08:00  INFO easytier::web_client: Successfully connected to tunnel{tcp}\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = scanConnectionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Connected {
		t.Fatalf("the newest line did not win: %+v", status)
	}

	// A log without connection lines is found but has no known state.
	if err := os.WriteFile(path, []byte("2026-10-23T10:00:00+08:00 INFO starting\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = scanConnectionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Found || status.Known {
		t.Fatalf("status = %+v", status)
	}

	// A missing log file is ErrNotExist, so the caller tries the next
	// candidate.
	if _, err := scanConnectionLog(filepath.Join(dir, "none.log")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing log: %v", err)
	}
}

// Only the tail is read: a huge log with a recent answer still resolves.
func TestScanConnectionLogReadsTheTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, serviceLogName)
	var log strings.Builder
	log.WriteString("2026-10-23T09:00:00+08:00 INFO Successfully connected to tunnel\n")
	for log.Len() < serviceLogTail+1024 {
		log.WriteString("2026-10-23T09:01:00+08:00 DEBUG noise\n")
	}
	log.WriteString("2026-10-23T09:02:00+08:00 WARN Failed to connect to the server, retrying in 5 seconds...\n")
	if err := os.WriteFile(path, []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := scanConnectionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Known || status.Connected {
		t.Fatalf("the tail was not scanned: %+v", status)
	}
}

func TestScanServiceConnectionLog(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()

	// No log anywhere: no channel.
	status, err := ScanServiceConnectionLog(ServiceSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Found {
		t.Fatalf("found a log that does not exist: %+v", status)
	}

	// A configured log directory is used.
	logDir := t.TempDir()
	line := time.Now().UTC().Format(time.RFC3339Nano) + "  INFO web_client: Successfully connected to tunnel\n"
	if err := os.WriteFile(filepath.Join(logDir, serviceLogName), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = ScanServiceConnectionLog(ServiceSettings{LogDir: logDir})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Found || !status.Known || !status.Connected {
		t.Fatalf("status = %+v", status)
	}

	// Without a configured directory, the default config directory of a
	// per-profile install is found.
	root, err := InstanceRoot()
	if err != nil {
		t.Fatal(err)
	}
	defaultDir := filepath.Join(root, "service-configs")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultDir, serviceLogName), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = ScanServiceConnectionLog(ServiceSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Found || !status.Connected {
		t.Fatalf("default directory not scanned: %+v", status)
	}
}
