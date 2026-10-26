package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// The DHCP switch follows the export's inference until the user touches it,
// which makes the value explicit; a fixed IPv4 locks it.
func TestDHCPSwitch(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	tt := ui.NewTester(a.view, 1200, 1800)
	if !tt.HasText("DHCP") {
		t.Fatalf("the DHCP switch is not shown: %q", tt.Texts())
	}
	// Default draft (no IPv4, TUN on): the switch shows on; flipping it
	// writes an explicit dhcp = false.
	if err := tt.Click("DHCP 自动获取"); err != nil {
		t.Fatal(err)
	}
	if dhcp := a.draft().config.DHCP; dhcp == nil || *dhcp {
		t.Fatalf("DHCP = %v, want explicit false", dhcp)
	}
	// A fixed IPv4 disables the switch: clicking changes nothing.
	a.draft().config.IPv4 = "10.144.0.10/24"
	a.draft().config.DHCP = nil
	tt.Frame()
	if err := tt.Click("DHCP 自动获取"); err != nil {
		t.Fatal(err)
	}
	if a.draft().config.DHCP != nil {
		t.Fatal("the disabled DHCP switch changed the configuration")
	}
}

// The manual-routes switch decides what an empty list means; a non-empty
// list forces it on and locks it.
func TestManualRoutesSwitch(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	a.advancedOpen = true
	tt := ui.NewTester(a.view, 1200, 7000)
	if !tt.HasText("启用手动路由") {
		t.Fatalf("the manual-routes switch is not shown: %q", tt.Texts())
	}
	if !tt.HasText("列表为空时开启会导出 routes = []，停用自动学习的路由") {
		t.Fatalf("the empty-list hint is missing: %q", tt.Texts())
	}
	if err := tt.Click("启用手动路由"); err != nil {
		t.Fatal(err)
	}
	if !a.draft().config.ManualRoutesOn {
		t.Fatal("the switch did not enable manual routes")
	}
	// Entries in the list force the switch on and disable it.
	a.draft().manualRoutesText = "192.168.0.0/16"
	tt.Frame()
	if !tt.HasText("列表非空时始终启用手动路由") {
		t.Fatalf("the locked hint is missing: %q", tt.Texts())
	}
	if err := tt.Click("启用手动路由"); err != nil {
		t.Fatal(err)
	}
	if !a.draft().config.ManualRoutesOn {
		t.Fatal("the locked switch turned manual routes off")
	}
}

// The SOCKS5 bind address is editable once a port is set.
func TestSocks5BindAddressInput(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	a.advancedOpen = true
	a.draft().config.Socks5Port = 11080
	tt := ui.NewTester(a.view, 1200, 7000)
	if !tt.HasText("SOCKS5 绑定地址") {
		t.Fatalf("the bind address field is not shown: %q", tt.Texts())
	}
	if err := tt.Click("SOCKS5 绑定地址"); err != nil {
		t.Fatal(err)
	}
	tt.Type("127.0.0.1")
	if a.draft().config.Socks5Host != "127.0.0.1" {
		t.Fatalf("Socks5Host = %q", a.draft().config.Socks5Host)
	}
	// Without a port the field is disabled.
	a.draft().config.Socks5Port = 0
	a.draft().config.Socks5Host = ""
	tt.Frame()
	if err := tt.Click("SOCKS5 绑定地址"); err != nil {
		t.Fatal(err)
	}
	tt.Type("127.0.0.1")
	if a.draft().config.Socks5Host != "" {
		t.Fatal("the disabled bind address field accepted input")
	}
}

// Saving with secure mode on generates the identity once and keeps it.
func TestSaveGeneratesSecureModeKeysOnce(t *testing.T) {
	a := newTestApp(t)
	a.draft().config.SecureMode = true
	a.draft().config.NetworkSecret = "s3cret"
	tt := ui.NewTester(a.view, 1200, 1800)
	if err := tt.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	saved := profiles[0].Config
	if saved.SecureModePrivateKey == "" || saved.SecureModePublicKey == "" {
		t.Fatal("the secure-mode identity was not saved")
	}
	if err := saved.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	profiles, err = easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if profiles[0].Config.SecureModePrivateKey != saved.SecureModePrivateKey {
		t.Fatal("saving again changed the identity")
	}
}
