package easytier

import (
	"context"
	"testing"
	"time"
)

func TestLogLevelsAreAccepted(t *testing.T) {
	for _, level := range []string{"off", "error", "warn", "info", "debug", "trace"} {
		if !validLogLevel(level) {
			t.Fatalf("missing level %s", level)
		}
	}
	if err := SetRuntimeLogLevel(context.Background(), "unused", "invalid"); err == nil {
		t.Fatal("invalid level accepted")
	}
}
func TestE2ERuntimeLoggerReload(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	ctl, _ := testController(t)
	ctl.engine = nativeEngine{}
	ctl.Start(testConfig())
	snap := waitForPhase(t, ctl, PhaseRunning, 30*time.Second)
	for _, level := range []string{"off", "error", "warn", "info", "debug", "trace"} {
		if err := SetRuntimeLogLevel(context.Background(), snap.Portal, level); err != nil {
			t.Fatal(err)
		}
		data, err := managementRPC(context.Background(), snap.Portal, "LoggerRpc", 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := wireFields(data)
		if err != nil {
			t.Fatal(err)
		}
		var got uint64
		for _, f := range fields {
			if f.number == 1 {
				got = f.value
			}
		}
		want := map[string]uint64{"off": 0, "error": 1, "warn": 2, "info": 3, "debug": 4, "trace": 5}[level]
		if got != want {
			t.Fatalf("Core log level %s = %d", level, got)
		}
	}
}
