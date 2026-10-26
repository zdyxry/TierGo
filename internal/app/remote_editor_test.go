package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestRemoteEditorDoesNotMutateLocalDraft(t *testing.T) {
	a := newTestApp(t)
	a.page = "remote"
	a.remoteConnected = true
	// Keep networking out of this UI test while rendering the actual modal.
	a.remoteChecked = time.Now()
	a.remoteEditOpen = true
	a.remoteEditAddress = "127.0.0.1:15888"
	a.remoteEditConfig = easytier.RemoteConfig{Network: easytier.RemoteNetwork{ID: "test"}}
	a.remoteEditText = `{"network_name":"remote"}`
	tt := ui.NewTester(a.view, 1200, 1400)
	if !tt.HasText("完整远端 NetworkConfig") {
		t.Fatal("missing full editor")
	}
	if err := tt.Click("取消远端编辑"); err != nil {
		t.Fatal(err)
	}
	if a.remoteEditOpen || a.remoteEditText != "" {
		t.Fatal("cancel retained secret text")
	}
	if a.draft().config.NetworkName != "easytier" {
		t.Fatal("remote edit changed local profile")
	}
}
