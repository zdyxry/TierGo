package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPreferencesContainClientOnlySettings(t *testing.T) {
	a := newTestApp(t)
	a.page = "preferences"
	tt := ui.NewTester(a.view, 1040, 700)
	for _, text := range []string{"应用设置", "Language / 语言", "登录时启动客户端", "开机自动启动"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q: %q", text, tt.Texts())
		}
	}
	for _, text := range []string{"安装或更新", "卸载", "服务选项"} {
		if tt.HasText(text) {
			t.Fatalf("system-service control %q leaked into application settings", text)
		}
	}
}
