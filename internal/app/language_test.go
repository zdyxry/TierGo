package app

import (
	"context"
	"testing"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func TestEnglishPagesAndSwitchPreserveConfiguration(t *testing.T) {
	old := locale.Language()
	defer locale.Set(old)
	a := newTestApp(t)
	a.profiles[0].Name = "Office"
	a.draft().name = "Office"
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) { return easytier.ServiceStopped, nil }
	locale.Set("en")
	a.advancedOpen = true
	a.draft().config.ACL = easytier.DefaultACL()
	for _, page := range []string{"overview", "peers", "routes", "settings", "logs", "acl", "monitor", "remote", "portal", "service", "preferences", "about"} {
		t.Run(page, func(t *testing.T) {
			a.page = page
			tt := ui.NewTester(a.view, 1400, 8000)
			for _, text := range tt.Texts() {
				if text == "Language / 语言" || text == "简体中文" {
					continue
				}
				for _, r := range text {
					if unicode.Is(unicode.Han, r) {
						t.Fatalf("Chinese text in English %s page: %q", page, text)
					}
				}
			}
		})
	}
	// Switch through the actual UI, persist, then verify network data untouched.
	a.page = "preferences"
	tt := ui.NewTester(a.view, 1400, 1800)
	if err := tt.Click("简体中文"); err != nil {
		t.Fatal(err)
	}
	if locale.Language() != "zh-Hans" {
		t.Fatal("language did not change")
	}
	tt.Frame()
	if !tt.HasText("网络配置") {
		t.Fatal("navigation did not switch live")
	}
	saved, err := easytier.LoadAppSettings()
	if err != nil || saved.Language != "zh-Hans" {
		t.Fatalf("language not persisted: %+v %v", saved, err)
	}
	if a.draft().config.NetworkName != "easytier" || a.profiles[0].Name != "Office" {
		t.Fatal("language changed network data")
	}
	a.Shutdown(time.Second)
}
