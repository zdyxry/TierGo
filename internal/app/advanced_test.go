package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestAdvancedMainFlagsEditAndPersist(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	a.advancedOpen = true
	tt := ui.NewTester(a.view, 1200, 7000)
	for _, label := range []string{"不转发数据", "优先凭证节点转发", "出站套接字标记", "提供公网 IPv6 地址"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %s", label)
		}
	}
	if tt.HasText("SO_MARK 值") {
		t.Fatal("socket mark value shows while the mark is unset")
	}
	for _, label := range []string{"不转发数据", "优先凭证节点转发", "出站套接字标记", "提供公网 IPv6 地址"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	tt.Frame()
	if !tt.HasText("SO_MARK 值") {
		t.Fatal("enabling the mark did not reveal its value field")
	}
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	config := profiles[0].Config
	if !config.DisableRelayData || !config.PreferPeerRelay || !config.IPv6PublicProvider {
		t.Fatalf("main flags not saved: %+v", config)
	}
	if config.SocketMark == nil || *config.SocketMark != 0 {
		t.Fatalf("socket mark not saved: %+v", config.SocketMark)
	}
}

func TestAdvancedSettingsEditAndPersist(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	a.advancedOpen = true
	tt := ui.NewTester(a.view, 1200, 7000)
	for _, label := range []string{"TUN 设备名", "接收限速", "SOCKS5", "P2P", "禁用 UPnP", "Magic DNS"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %s", label)
		}
	}
	if err := tt.Click("TUN 设备名"); err != nil {
		t.Fatal(err)
	}
	tt.Type("et-office")
	if err := tt.Click("手动路由"); err != nil {
		t.Fatal(err)
	}
	tt.Type("192.168.0.0/16\n172.16.0.0/16")
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Config.DevName != "et-office" || len(profiles[0].Config.ManualRoutes) != 2 {
		t.Fatalf("settings not saved: %+v", profiles)
	}
}
