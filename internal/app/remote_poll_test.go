package app

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestRemotePollingDoesNotBlockAndDiscardsStaleResults(t *testing.T) {
	a := newTestApp(t)
	a.page = "remote"
	a.remoteConnected = true
	a.settings.RemoteRPC = "127.0.0.1:12345"
	gate := make(chan struct{})
	a.remotePoll = func(ctx context.Context, address, selected string) remotePollResult {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return remotePollResult{address: address, networks: []easytier.RemoteNetwork{{ID: "old"}}, selected: "old"}
	}
	start := time.Now()
	tt := ui.NewTester(a.view, 1200, 1600)
	if time.Since(start) > time.Second {
		t.Fatal("remote request blocked UI")
	}
	if a.remotePending == nil {
		t.Fatal("poll not scheduled")
	}
	// The user changes endpoints before the first result arrives.
	a.settings.RemoteRPC = "127.0.0.1:23456"
	old := a.remotePending
	close(gate)
	deadline := time.Now().Add(time.Second)
	for len(old) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if a.remoteSelected == "old" {
		t.Fatal("stale result changed current selection")
	}
	a.remoteConnected = false
}
