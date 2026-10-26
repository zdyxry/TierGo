package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRemotePageMakesOwnershipAndSecurityExplicit(t *testing.T) {
	a := newTestApp(t)
	a.page = "remote"
	tt := ui.NewTester(a.view, 1200, 900)
	for _, text := range []string{"远程 RPC 地址", "不启动或停止远端进程", "不提供身份认证", "断开远程查看"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q", text)
		}
	}
	if err := tt.Click("断开远程查看"); err != nil {
		t.Fatal(err)
	}
	if a.mgr.AnyRunning() {
		t.Fatal("remote inspector owns a local process")
	}
}
