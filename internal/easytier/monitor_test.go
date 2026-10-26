package easytier

import (
	"testing"
	"time"
)

func TestTrafficRatesIgnoreResetsAndNewConnections(t *testing.T) {
	ctl := NewController("monitor")
	now := time.Now()
	ctl.monitorLocked(coreStatus{Counters: map[string]ConnCounter{"a": {100, 200}}}, now)
	ctl.monitorLocked(coreStatus{Counters: map[string]ConnCounter{"a": {300, 600}, "new": {999999, 999999}}}, now.Add(2*time.Second))
	got := ctl.snap.Traffic[0]
	if got.RxPerSecond != 100 || got.TxPerSecond != 200 {
		t.Fatalf("rate = %+v", got)
	}
	ctl.monitorLocked(coreStatus{Counters: map[string]ConnCounter{"a": {10, 20}}}, now.Add(4*time.Second))
	if last := ctl.snap.Traffic[1]; last.RxPerSecond != 0 || last.TxPerSecond != 0 {
		t.Fatalf("counter reset created spike: %+v", last)
	}
	ctl.monitorLocked(coreStatus{}, now.Add(6*time.Second))
	ctl.monitorLocked(coreStatus{Counters: map[string]ConnCounter{"a": {1000, 2000}}}, now.Add(8*time.Second))
	if len(ctl.snap.Traffic) != 2 {
		t.Fatal("missing data should reset baseline")
	}
}
func TestMonitorEventsAreBoundedAndLabelled(t *testing.T) {
	ctl := NewController("events")
	ctl.applyLocked(coreStatus{Node: NodeView{IPv4: "10.0.0.1"}})
	ctl.applyLocked(coreStatus{Node: NodeView{IPv4: "10.0.0.2"}, Peers: []PeerView{{PeerID: "2", Name: "new"}}})
	if len(ctl.snap.Events) != 2 || ctl.snap.Events[0].Source != "状态轮询" {
		t.Fatalf("events = %+v", ctl.snap.Events)
	}
	for range 300 {
		ctl.eventLocked(time.Now(), "test", "bounded")
	}
	if len(ctl.snap.Events) != 200 {
		t.Fatal("unbounded event buffer")
	}
	snap := ctl.Snapshot()
	snap.Events[0].Kind = "mutated"
	if ctl.Snapshot().Events[0].Kind == "mutated" {
		t.Fatal("snapshot aliases events")
	}
}
func TestDiffTrafficSkipsNewAndResetConnections(t *testing.T) {
	previous := map[string]ConnCounter{"a": {100, 100}, "gone": {5, 5}}
	current := map[string]ConnCounter{"a": {300, 500}, "new": {9999, 9999}}
	rx, tx := DiffTraffic(previous, current, 2)
	if rx != 100 || tx != 200 {
		t.Fatalf("rx/tx = %v/%v", rx, tx)
	}
	if rx, tx := DiffTraffic(current, map[string]ConnCounter{"a": {10, 10}}, 2); rx != 0 || tx != 0 {
		t.Fatalf("counter reset created traffic: %v/%v", rx, tx)
	}
	if rx, tx := DiffTraffic(previous, current, 0); rx != 0 || tx != 0 {
		t.Fatalf("zero elapsed created traffic: %v/%v", rx, tx)
	}
}

func TestAppendTrafficSampleIsBounded(t *testing.T) {
	var samples []TrafficSample
	for i := range 200 {
		samples = AppendTrafficSample(samples, TrafficSample{At: time.Unix(int64(i), 0)})
	}
	if len(samples) != 120 || samples[0].At != time.Unix(80, 0) {
		t.Fatalf("samples = %d from %v", len(samples), samples[0].At)
	}
}

func TestNodeInterfacesDecodeFixture(t *testing.T) {
	var node cliNode
	readFixture(t, "cli-node.json", &node)
	view := nodeView(node)
	if len(view.InterfaceIPv4s) != 5 || view.InterfaceIPv4s[0] != "192.168.8.97" || len(view.InterfaceIPv6s) != 1 {
		t.Fatalf("interfaces = %+v", view)
	}
}
