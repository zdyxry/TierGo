package app

import (
	"strings"
	"testing"

	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestTrayModelUsesLocalNetworks(t *testing.T) {
	a := newTestApp(t)
	model := a.trayModel()
	if model.phase != easytier.PhaseStopped || model.status != "未连接" {
		t.Fatalf("initial model = %+v", model)
	}
	connected(a)
	model = a.trayModel()
	if model.phase != easytier.PhaseRunning || model.status != "已连接到 测试网络" {
		t.Fatalf("connected model = %+v", model)
	}
	if !strings.Contains(model.detail, "10.144.0.10/24") || !strings.Contains(model.detail, "2 个在线节点") {
		t.Fatalf("missing address and peer count: %q", model.detail)
	}
}

func TestTrayModelDoesNotPresentRemoteAsLocal(t *testing.T) {
	a := newTestApp(t)
	a.remoteActive = true
	a.remoteSnapshot = easytier.Snapshot{Phase: easytier.PhaseRunning, Node: easytier.NodeView{IPv4: "10.99.0.1"}}
	model := a.trayModel()
	if model.phase != easytier.PhaseStopped || strings.Contains(model.detail, "10.99.0.1") {
		t.Fatalf("remote state leaked into the local tray: %+v", model)
	}
}

func TestTrayModelFallsBackToAnotherRunningNetwork(t *testing.T) {
	a := newTestApp(t)
	other := easytier.NewProfile("家庭")
	a.profiles = append(a.profiles, other)
	a.mgr.Controller(other.ID).SetSnapshotForTest(easytier.Snapshot{
		Phase: easytier.PhaseRunning,
		Node:  easytier.NodeView{IPv4: "10.2.0.1/24"},
	})
	model := a.trayModel()
	if model.status != "已连接到 家庭" || !strings.Contains(model.detail, "10.2.0.1/24") {
		t.Fatalf("another running network was not selected: %+v", model)
	}
	if trayKey(model) == "" {
		t.Fatal("empty tray revision")
	}
}
