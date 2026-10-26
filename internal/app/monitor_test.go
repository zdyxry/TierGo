package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestMonitorDisplaysRatesAndObservedEvents(t *testing.T) {
	a := newTestApp(t)
	a.page = "monitor"
	now := time.Now()
	a.ctl().SetSnapshotForTest(easytier.Snapshot{Phase: easytier.PhaseRunning,
		Traffic: []easytier.TrafficSample{{At: now.Add(-time.Second), RxPerSecond: 100}, {At: now, RxPerSecond: 200, TxPerSecond: 300}},
		Events:  []easytier.NetworkEvent{{At: now, Kind: "节点出现", Detail: "nas", Source: "状态轮询"}},
	})
	tt := ui.NewTester(a.view, 1200, 1000)
	for _, text := range []string{"200 B/s", "300 B/s", "节点出现", "nas", "Core 仅保留最近 20 条"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q", text)
		}
	}
}
