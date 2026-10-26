package app

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"time"
)

func TestCentralOrdinaryModeControlsAndShutdown(t *testing.T) {
	a := newTestApp(t)
	a.page = "remote"
	tt := ui.NewTester(a.view, 1200, 1800)
	for _, text := range []string{"普通模式连接 Config Server", "停止普通模式集中管理", "查看普通模式下发网络"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %s", text)
		}
	}
	a.Shutdown(time.Second)
}
