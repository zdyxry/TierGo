package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"testing"
)

func TestDevicePortalShowsStatesGroupsAndQRCode(t *testing.T) {
	a := newTestApp(t)
	a.page = "portal"
	a.draft().config.VPNPortal = &easytier.VPNPortal{DeviceMode: true, Listen: "127.0.0.1:51820"}
	a.portalOwner = a.selected
	a.portalDeviceSelected = "laptop"
	a.portalInfo = easytier.PortalInfo{Listener: "127.0.0.1:51820", Clients: []easytier.PortalDeviceInfo{{PortalDevice: easytier.PortalDevice{Name: "laptop", VirtualIP: "10.14.14.2/24", Groups: []string{"admin"}}, State: 3, ClientConfig: "[Interface]\nPrivateKey = secret\n"}}}
	a.ctl().SetSnapshotForTest(easytier.Snapshot{Phase: easytier.PhaseRunning})
	tt := ui.NewTester(a.view, 1200, 2200)
	for _, text := range []string{"laptop", "在线", "设备 ACL 组", "复制客户端配置", "下载客户端配置"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %s", text)
		}
	}
	if a.portalQR == nil {
		t.Fatal("QR bitmap missing")
	}
	if err := tt.Click("删除设备 laptop"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("确认删除设备") {
		t.Fatal("device removal lacks confirmation")
	}
}
