package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestPortalEnableAndSave(t *testing.T) {
	a := newTestApp(t)
	a.page = "portal"
	tt := ui.NewTester(a.view, 1200, 1000)
	if err := tt.Click("启用 VPN Portal"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存 Portal 设置"); err != nil {
		t.Fatal(err)
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Config.VPNPortal == nil {
		t.Fatal("Portal configuration not saved")
	}
}
