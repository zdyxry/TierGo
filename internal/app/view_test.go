package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	// Profiles are written to a throwaway directory, never the user's.
	t.Cleanup(easytier.SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	profile := easytier.NewProfile("测试网络")
	profile.Config = easytier.DefaultConfig()
	a := New(easytier.NewManager(), []easytier.Profile{profile}, easytier.AppSettings{}, "")
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceNotInstalled, nil
	}
	a.serviceDriftCheck = func(easytier.ServiceSettings) (string, string, bool) {
		return "", "", false
	}
	return a
}

func TestOverviewShowsDisconnected(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 700)

	if !tt.HasText("未连接") {
		t.Fatalf("overview does not show the disconnected state: %q", tt.Texts())
	}
	if !tt.HasText("概览") || !tt.HasText("网络配置") {
		t.Fatalf("navigation is missing: %q", tt.Texts())
	}
	if !tt.HasText("在线节点") {
		t.Fatalf("stats are missing: %q", tt.Texts())
	}
}

func TestNavigationSwitchesPages(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 900)

	for _, testCase := range []struct {
		nav   string
		title string
	}{
		{"节点", "「测试网络」中的其他设备"},
		{"路由", "「测试网络」的转发表"},
		{"网络配置", "「测试网络」的网络身份、连接方式与虚拟网卡"},
		{"日志", "「测试网络」的内核事件与本机操作记录"},
		{"关于", "非官方 EasyTier 客户端 · MyGo 原生 UI"},
	} {
		if err := tt.Click(testCase.nav); err != nil {
			t.Fatalf("click %q: %v", testCase.nav, err)
		}
		if !tt.HasText(testCase.title) && !hasTextContaining(tt.Texts(), testCase.title) {
			t.Fatalf("page %q does not show %q: %q", testCase.nav, testCase.title, tt.Texts())
		}
	}
}

// The settings offer how much the core writes to disk, and say so.
func TestSettingsOffersTheLogLevel(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 1600)
	if err := tt.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("日志级别") {
		t.Fatalf("the log level is not offered: %q", tt.Texts())
	}
	if !tt.HasText("仅错误") {
		t.Errorf("the default log level is not shown: %q", tt.Texts())
	}
}

func TestSettingsEditAndSave(t *testing.T) {
	a := newTestApp(t)
	// Tall enough that the whole settings page is laid out and clickable.
	tt := ui.NewTester(a.view, 1040, 1800)

	if err := tt.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	a.draft().config.NetworkName = ""
	if err := tt.Click("网络名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("office")

	if a.draft().config.NetworkName != "office" {
		t.Fatalf("network name = %q, want %q", a.draft().config.NetworkName, "office")
	}

	if err := tt.Click("对端节点"); err != nil {
		t.Fatal(err)
	}
	tt.Type("tcp://public.top:11010\nudp://1.2.3.4:11010")
	if got := easytier.SplitLines(a.draft().peersText); len(got) != 2 || got[0] != "tcp://public.top:11010" {
		t.Fatalf("peers parsed to %v", got)
	}

	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles = %v", profiles)
	}
	saved := profiles[0].Config
	if saved.NetworkName != "office" {
		t.Fatalf("saved network name = %q, want office", saved.NetworkName)
	}
	if len(saved.Peers) != 2 {
		t.Fatalf("saved peers = %v", saved.Peers)
	}
}

func TestConnectWithoutNetworkNameGoesToSettings(t *testing.T) {
	a := newTestApp(t)
	a.draft().config.NetworkName = ""
	tt := ui.NewTester(a.view, 1040, 700)

	if err := tt.Click("连接"); err != nil {
		t.Fatal(err)
	}
	if a.page != "settings" {
		t.Fatalf("page = %q, want settings", a.page)
	}
	if a.ctl().Running() {
		t.Fatal("controller started without a network name")
	}
}

func hasTextContaining(texts []string, want string) bool {
	for _, text := range texts {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

// ---- 多网络 ----

func TestSidebarListsNetworksAndSwitches(t *testing.T) {
	a := newTestApp(t)
	other := easytier.NewProfile("家庭")
	a.profiles = append(a.profiles, other)

	tt := ui.NewTester(a.view, 1040, 700)
	if !tt.HasText("测试网络") || !tt.HasText("家庭") {
		t.Fatalf("the sidebar does not list both networks: %q", tt.Texts())
	}
	if err := tt.Click("家庭"); err != nil {
		t.Fatal(err)
	}
	if a.selected != other.ID {
		t.Fatal("clicking a network did not select it")
	}
	// The pages now belong to the other network.
	if !tt.HasText("家庭") {
		t.Fatalf("the heading did not follow the selection: %q", tt.Texts())
	}
}

func TestAddNetworkThroughTheModal(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 700)

	if err := tt.Click("新建网络"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("网络名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("公司")
	if err := tt.Click("创建"); err != nil {
		t.Fatal(err)
	}

	if len(a.profiles) != 2 {
		t.Fatalf("profiles = %v", a.profiles)
	}
	added := a.profiles[1]
	if added.Name != "公司" || a.selected != added.ID {
		t.Fatalf("added profile = %+v, selected = %q", added, a.selected)
	}
	// It was saved, so the next run sees it too.
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Name != "公司" {
		t.Fatalf("saved profiles = %v", profiles)
	}
	// A new network opens on its settings page.
	if a.page != "settings" {
		t.Fatalf("page = %q", a.page)
	}
}

func TestDeleteNetworkAsksAndRemoves(t *testing.T) {
	a := newTestApp(t)
	first := a.profiles[0]
	if err := easytier.SaveProfile(first); err != nil {
		t.Fatal(err)
	}
	other := easytier.NewProfile("临时")
	if err := easytier.SaveProfile(other); err != nil {
		t.Fatal(err)
	}
	a.profiles = append(a.profiles, other)
	a.selected = other.ID

	tt := ui.NewTester(a.view, 1040, 1800)
	if err := tt.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if !a.confirmDelete {
		t.Fatal("the confirmation did not open")
	}
	if err := tt.Click("确认删除"); err != nil {
		t.Fatal(err)
	}

	if len(a.profiles) != 1 || a.profiles[0].ID != first.ID {
		t.Fatalf("profiles = %v", a.profiles)
	}
	if a.selected != first.ID {
		t.Fatal("the selection did not fall back to the remaining network")
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("the deleted profile survived: %v", profiles)
	}
}

// The last network cannot be deleted: an empty list would leave nothing to
// connect.
func TestLastNetworkCannotBeDeleted(t *testing.T) {
	a := newTestApp(t)
	a.deleteProfile(a.profiles[0].ID)
	if len(a.profiles) != 1 {
		t.Fatalf("the last network was deleted: %v", a.profiles)
	}
}

// ---- 后台与服务 ----

func TestServicePageShowsTheServiceState(t *testing.T) {
	a := newTestApp(t)
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceStopped, nil
	}
	tt := ui.NewTester(a.view, 1040, 900)
	if err := tt.Click("系统服务"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	tt.Frame()
	if !tt.HasText("系统服务") || !tt.HasText("已停止") {
		t.Fatalf("the service state is not shown: %q", tt.Texts())
	}
	if tt.HasText("Language / 语言") || tt.HasText("登录时启动客户端") {
		t.Fatal("application preferences leaked into the system service page")
	}
}

// A running service owns the networks; connecting locally too would fight
// it, so the window refuses and points at the service page.
func TestConnectRefusesWhileTheServiceRuns(t *testing.T) {
	a := newTestApp(t)
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceRunning, nil
	}
	a.draft().config.EnableTUN = false // 不然先被权限检查拦下
	tt := ui.NewTester(a.view, 1040, 700)
	if err := tt.Click("连接"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	if a.page != "service" {
		t.Fatalf("page = %q, want service", a.page)
	}
	if a.ctl().Running() {
		t.Fatal("a local core started although the service runs")
	}
}

// ---- 导入导出 ----

// The TOML editor opens with the current draft, and applying a rewritten
// text replaces it; a broken text is named, not applied.
func TestTOMLEditorAppliesToTheDraft(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 1600)
	if err := tt.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("编辑 TOML"); err != nil {
		t.Fatal(err)
	}
	if !a.tomlOpen || !strings.Contains(a.tomlText, `network_name = "easytier"`) {
		t.Fatalf("the editor did not open with the draft: %q", a.tomlText)
	}

	a.tomlText = "[network_identity]\nnetwork_name = \"imported\"\nnetwork_secret = \"s\"\n"
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if a.tomlOpen {
		t.Fatal("a valid text did not close the editor")
	}
	if got := a.draft().config.NetworkName; got != "imported" {
		t.Fatalf("network name = %q", got)
	}

	// A broken text stays in the editor with its reason. (The toast from
	// the apply must fade first: it covers the buttons.)
	tt.Advance(6 * time.Second)
	if err := tt.Click("编辑 TOML"); err != nil {
		t.Fatal(err)
	}
	a.tomlText = "not = [valid"
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if !a.tomlOpen || a.tomlErr == "" {
		t.Fatalf("a broken text was applied: open=%v err=%q", a.tomlOpen, a.tomlErr)
	}
	if got := a.draft().config.NetworkName; got != "imported" {
		t.Fatalf("the broken text overwrote the draft: %q", got)
	}
}
