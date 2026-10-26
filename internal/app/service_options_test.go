package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// The service page offers the install-time options and saves them.
func TestServicePageOptionsSave(t *testing.T) {
	a := newTestApp(t)
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceNotInstalled, nil
	}
	tt := ui.NewTester(a.view, 1040, 1600)
	if err := tt.Click("系统服务"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"服务选项", "服务配置目录", "服务 RPC 地址", "服务日志目录", "服务文件日志级别", "安装或更新时生效"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q: %q", text, tt.Texts())
		}
	}

	if err := tt.Click("服务配置目录"); err != nil {
		t.Fatal(err)
	}
	tt.Type(filepath.Join(t.TempDir(), "svc-configs"))
	if err := tt.Click("服务 RPC 地址"); err != nil {
		t.Fatal(err)
	}
	tt.Type("127.0.0.1:15999")
	if err := tt.Click("保存服务选项"); err != nil {
		t.Fatal(err)
	}

	loaded, err := easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Service.RPCAddress != "127.0.0.1:15999" || loaded.Service.ConfigDir == "" {
		t.Fatalf("service options were not saved: %+v", loaded.Service)
	}
	if a.settings.Service != loaded.Service {
		t.Fatalf("the window keeps different options: %+v", a.settings.Service)
	}
}

// An invalid option is rejected and nothing is saved.
func TestServicePageOptionsValidation(t *testing.T) {
	a := newTestApp(t)
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceNotInstalled, nil
	}
	tt := ui.NewTester(a.view, 1040, 1600)
	if err := tt.Click("系统服务"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("服务 RPC 地址"); err != nil {
		t.Fatal(err)
	}
	tt.Type("not-an-address")
	if err := tt.Click("保存服务选项"); err != nil {
		t.Fatal(err)
	}
	loaded, err := easytier.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Service.RPCAddress != "" {
		t.Fatalf("an invalid RPC address was saved: %+v", loaded.Service)
	}
}

// An embedded-core upgrade since the service install shows a banner;
// installing again rewrites the marker and the banner goes away.
func TestServicePageCoreDriftBanner(t *testing.T) {
	a := newTestApp(t)
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceRunning, nil
	}
	drifted := true
	a.serviceDriftCheck = func(easytier.ServiceSettings) (string, string, bool) {
		if !drifted {
			return "", "v2.6.4", false
		}
		return "v2.6.3", "v2.6.4", true
	}
	installed := false
	a.serviceInstallFn = func(context.Context, []easytier.Profile, easytier.ServiceSettings) error {
		installed = true
		drifted = false
		return nil
	}

	tt := ui.NewTester(a.view, 1040, 1600)
	if err := tt.Click("系统服务"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	tt.Frame()
	if !tt.HasText("内嵌 Core 已从 v2.6.3 变为 v2.6.4，点「安装或更新」同步服务配置") {
		t.Fatalf("the drift banner is missing: %q", tt.Texts())
	}
	if err := tt.Click("安装或更新"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	tt.Frame()
	if !installed {
		t.Fatal("the install did not run")
	}
	if hasTextContaining(tt.Texts(), "内嵌 Core 已从") {
		t.Fatalf("the banner survived a reinstall: %q", tt.Texts())
	}
}

// The remote page shows the service-mode Config Server state inferred from
// the service's file log, separate from the process state.
func TestRemotePageServiceLogInference(t *testing.T) {
	a := newTestApp(t)
	a.serviceLogScan = func() (easytier.ServiceLogStatus, error) {
		return easytier.ServiceLogStatus{
			Found:     true,
			Known:     true,
			Connected: true,
			At:        time.Date(2026, 10, 23, 10, 30, 0, 0, time.Local),
			Line:      "2026-10-23T10:30:00+08:00 INFO Successfully connected to tunnel",
			Path:      "/var/log/easytier/easytier.log",
		}, nil
	}
	a.page = "remote"
	tt := ui.NewTester(a.view, 1200, 1800)
	flushAsync(t, a)
	tt.Frame()
	for _, text := range []string{"服务模式连接状态", "服务日志最近报告：已连接（推断）"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q: %q", text, tt.Texts())
		}
	}
}

// Without a service file log there is no channel, and the page says so
// instead of guessing.
func TestRemotePageServiceNoChannel(t *testing.T) {
	a := newTestApp(t)
	a.serviceLogScan = func() (easytier.ServiceLogStatus, error) {
		return easytier.ServiceLogStatus{}, nil
	}
	a.page = "remote"
	tt := ui.NewTester(a.view, 1200, 1800)
	flushAsync(t, a)
	tt.Frame()
	if !tt.HasText("服务模式连接状态") || !hasTextContaining(tt.Texts(), "服务模式无连接状态通道") {
		t.Fatalf("the no-channel explanation is missing: %q", tt.Texts())
	}
}

// The ordinary mode transport claim shows the Core line it rests on.
func TestRemotePageTransportEvidence(t *testing.T) {
	a := newTestApp(t)
	a.central.SetSnapshotForTest(easytier.CentralSnapshot{
		Phase:              easytier.PhaseRunning,
		TransportConnected: true,
		TransportAt:        time.Now(),
		TransportLine:      "Successfully connected to [Config Server]",
	})
	a.page = "remote"
	tt := ui.NewTester(a.view, 1200, 1800)
	if !tt.HasText("Core 报告传输已连接（不等于已验证服务端心跳）") {
		t.Fatalf("transport claim missing: %q", tt.Texts())
	}
	if !hasTextContaining(tt.Texts(), "最近传输状态依据") ||
		!hasTextContaining(tt.Texts(), "Successfully connected to [Config Server]") {
		t.Fatalf("the evidence line is missing: %q", tt.Texts())
	}
}
