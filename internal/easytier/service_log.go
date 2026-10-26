package easytier

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The official GUI gets the Config Server state only in ordinary mode, where
// the GUI process embeds the WebClient itself and polls its in-process
// `is_connected` flag (easytier-gui `is_web_client_connected`; the flag is
// transport-level, set when the tunnel connects). In service mode the
// official GUI shows "unknown": no RPC method exposes the state, and the
// service's core is a separate process here, so there is no in-process flag
// either. What remains is the service's file log — off by default
// (FileLoggerConfig.level defaults to OFF), enabled through the service
// options — whose tail reports the same WebClient lifecycle lines the
// ordinary mode watches on stdout. This scan is inference, labeled as such
// in the UI, never heartbeat proof.

// serviceLogName is the file the Core's rolling file appender currently
// writes; rotated copies get numeric suffixes and are not scanned.
const serviceLogName = "easytier.log"

// serviceLogTail limits how far back the scan reads.
const serviceLogTail = 256 * 1024

// ServiceLogStatus is the Config Server connectivity inferred from the tail
// of the service's file log.
type ServiceLogStatus struct {
	// Found is false when no service log file exists (file logging off or
	// never installed with it): there is no channel to infer from.
	Found bool
	// Known is false when the log has no connection line yet.
	Known     bool
	Connected bool
	// At is the deciding line's timestamp, or the log's modification time
	// when the line carries none.
	At time.Time
	// Line is the deciding line, trimmed and capped.
	Line string
	// Path is the log file the answer came from.
	Path string
}

// classifyConnectionLine maps a lower-cased Core log line to the transport
// state it reports: 1 connected, -1 disconnected, 0 unrelated. It matches
// both phrasings: v2.6.4 logs "Successfully connected to ..." / "Failed to
// connect to the server, retrying in N seconds..." / "heartbeat failed: ...",
// main logs "connected to config server" / "...; retrying".
func classifyConnectionLine(lower string) int {
	switch {
	case strings.Contains(lower, "successfully connected"),
		strings.Contains(lower, "connected to config server"):
		return 1
	case strings.Contains(lower, "retrying"),
		strings.Contains(lower, "heartbeat failed"),
		strings.Contains(lower, "connecterror"),
		strings.Contains(lower, "failed to connect"):
		return -1
	}
	return 0
}

// ScanServiceConnectionLog finds the service's file log and infers the last
// reported Config Server transport state from its tail. The log location is
// the configured log directory, or the Core's default (its working
// directory): the config directory for per-profile installs, the instance
// root for Config Server mode installs.
func ScanServiceConnectionLog(settings ServiceSettings) (ServiceLogStatus, error) {
	root, err := InstanceRoot()
	if err != nil {
		return ServiceLogStatus{}, err
	}
	var candidates []string
	if settings.LogDir != "" {
		candidates = []string{settings.LogDir}
	} else {
		candidates = serviceStateDirs(settings, root)
	}
	for _, dir := range candidates {
		status, err := scanConnectionLog(filepath.Join(dir, serviceLogName))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return status, err
	}
	return ServiceLogStatus{}, nil
}

// scanConnectionLog reads the tail of one log file, newest lines last, and
// takes the state from the most recent connection line.
func scanConnectionLog(path string) (ServiceLogStatus, error) {
	file, err := os.Open(path)
	if err != nil {
		return ServiceLogStatus{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ServiceLogStatus{}, err
	}
	start := info.Size() - serviceLogTail
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return ServiceLogStatus{}, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return ServiceLogStatus{}, err
	}
	status := ServiceLogStatus{Found: true, Path: path, At: info.ModTime()}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		switch classifyConnectionLine(strings.ToLower(line)) {
		case 1:
			status.Known, status.Connected = true, true
		case -1:
			status.Known, status.Connected = true, false
		default:
			continue
		}
		status.Line = line
		if at, ok := logLineTime(line); ok {
			status.At = at
		}
		return status, nil
	}
	return status, nil
}

// logLineTime reads the RFC3339 timestamp the Core's file logger prefixes
// every line with (get_logger_timer_rfc3339).
func logLineTime(line string) (time.Time, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, fields[0])
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}
