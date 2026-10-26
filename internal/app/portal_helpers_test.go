package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestMergeDeviceGroups(t *testing.T) {
	declared := []string{"admin", "ops", "guest"}
	for _, test := range []struct {
		name    string
		checked map[string]bool
		manual  string
		want    string
	}{
		{"checked keep declaration order", map[string]bool{"guest": true, "admin": true}, "", "admin,guest"},
		{"manual extras append", map[string]bool{"ops": true}, "temp, partner", "ops,temp,partner"},
		{"manual duplicate of checked collapses", map[string]bool{"admin": true}, "admin", "admin"},
		{"unchecked declared are skipped", map[string]bool{"admin": false}, "ops", "ops"},
		{"empty everything", map[string]bool{}, "", ""},
		{"spaces and empties in manual", nil, " , admin ,, ", "admin"},
	} {
		got := mergeDeviceGroups(declared, test.checked, test.manual)
		if strings.Join(got, ",") != test.want {
			t.Errorf("%s: got %v, want %q", test.name, got, test.want)
		}
	}
}

// devicePortalFixture loads a running local device-mode portal with one
// client, as if 读取客户端配置 had just succeeded.
func devicePortalFixture(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	a.page = "portal"
	a.draft().config.VPNPortal = &easytier.VPNPortal{
		DeviceMode: true, ClientCIDR: "10.14.14.0/24", Listen: "127.0.0.1:51820",
	}
	a.portalOwner = a.portalContextKey()
	a.portalInfo = easytier.PortalInfo{
		Listener: "0.0.0.0:51820",
		Clients: []easytier.PortalDeviceInfo{{
			PortalDevice: easytier.PortalDevice{Name: "laptop", VirtualIP: "10.14.14.2/24", Groups: []string{"admin"}},
			State:        3, ClientConfig: "[Interface]\nPrivateKey = secret\n",
		}},
	}
	a.ctl().SetSnapshotForTest(easytier.Snapshot{Phase: easytier.PhaseRunning})
	return a
}

func TestPortalDeviceIPAndEndpointSuggestions(t *testing.T) {
	a := devicePortalFixture(t)
	a.draft().config.Listeners = []string{"tcp://0.0.0.0:11010", "udp://192.168.1.8:11010"}
	a.draft().config.MappedListeners = []string{"tcp://203.0.113.5:11010"}
	tt := ui.NewTester(a.view, 1200, 2400)
	// 10.14.14.2 is taken by laptop; .1 is the first free host address.
	if !tt.HasText("使用建议地址") {
		t.Fatal("IP suggestion button missing")
	}
	if err := tt.Click("使用建议地址"); err != nil {
		t.Fatal(err)
	}
	if a.portalDeviceIP != "10.14.14.1/24" {
		t.Fatalf("suggested IP = %q", a.portalDeviceIP)
	}
	// Wildcard listeners are filtered; the LAN listener and the mapped
	// (public) listener each make a candidate on the portal port.
	for _, endpoint := range []string{"使用建议 192.168.1.8:51820", "使用建议 203.0.113.5:51820"} {
		if !tt.HasText(endpoint) {
			t.Fatalf("missing %s", endpoint)
		}
	}
	if err := tt.Click("使用建议 203.0.113.5:51820"); err != nil {
		t.Fatal(err)
	}
	if a.portalEndpoint != "203.0.113.5:51820" {
		t.Fatalf("endpoint = %q", a.portalEndpoint)
	}
}

func TestPortalDeviceGroupCheckboxes(t *testing.T) {
	a := devicePortalFixture(t)
	a.draft().config.ACL = easytier.DefaultACL()
	a.draft().config.ACL.V1.Group.Declares = []easytier.ACLGroupIdentity{{Name: "admin"}, {Name: "ops"}}
	tt := ui.NewTester(a.view, 1200, 2400)
	if !tt.HasText("设备 ACL 组（勾选已声明的组）") || !tt.HasText("其他组（逗号分隔，可选）") {
		t.Fatal("declared groups did not turn into checkboxes")
	}
	if err := tt.Click("ops"); err != nil {
		t.Fatal(err)
	}
	if !a.portalDeviceChecked["ops"] || a.portalDeviceChecked["admin"] {
		t.Fatalf("checkbox state: %v", a.portalDeviceChecked)
	}
}

func TestPortalDeviceAutoRefresh(t *testing.T) {
	a := devicePortalFixture(t)
	gate := make(chan struct{})
	a.portalPoll = func(ctx context.Context) (easytier.PortalInfo, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return easytier.PortalInfo{Listener: "0.0.0.0:51820", Clients: []easytier.PortalDeviceInfo{
			{PortalDevice: easytier.PortalDevice{Name: "laptop", VirtualIP: "10.14.14.2/24"}, State: 3},
			{PortalDevice: easytier.PortalDevice{Name: "phone", VirtualIP: "10.14.14.3/24"}, State: 1},
		}}, nil
	}
	tt := ui.NewTester(a.view, 1200, 2400)
	if a.portalPollPending == nil {
		t.Fatal("auto refresh was not scheduled on an open device portal")
	}
	pending := a.portalPollPending
	close(gate)
	deadline := time.Now().Add(2 * time.Second)
	for len(pending) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	tt.Frame()
	if len(a.portalInfo.Clients) != 2 || a.portalInfo.Clients[1].Name != "phone" {
		t.Fatalf("auto refresh did not update the device list: %+v", a.portalInfo.Clients)
	}
	// The added device persisted into the local profile.
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	clients := profiles[0].Config.VPNPortal.Clients
	if len(clients) != 2 || clients[1].Name != "phone" {
		t.Fatalf("device change not persisted: %+v", clients)
	}
}

func TestPortalCredentialPanel(t *testing.T) {
	a := devicePortalFixture(t)
	var generated easytier.CredentialRequest
	var revoked string
	a.credentialGenerateFn = func(ctx context.Context, request easytier.CredentialRequest) (easytier.GeneratedCredential, error) {
		generated = request
		return easytier.GeneratedCredential{ID: "cred-9", Secret: "secret-base64"}, nil
	}
	a.credentialListFn = func(ctx context.Context) ([]easytier.Credential, error) {
		return []easytier.Credential{{ID: "cred-9", Groups: []string{"admin"}, ExpiryUnix: time.Now().Add(time.Hour).Unix(), Reusable: true}}, nil
	}
	a.credentialRevokeFn = func(ctx context.Context, id string) error {
		revoked = id
		return nil
	}
	tt := ui.NewTester(a.view, 1200, 2600)
	for _, text := range []string{"临时凭据", "有效期（秒）", "生成凭据", "刷新凭据列表"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %s", text)
		}
	}
	if err := tt.Click("凭据 ACL 组（逗号分隔）"); err != nil {
		t.Fatal(err)
	}
	tt.Type("admin")
	a.credentialReusable = true
	if err := tt.Click("生成凭据"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	tt.Frame()
	if generated.TTLSeconds != 86400 || len(generated.Groups) != 1 || generated.Groups[0] != "admin" || !generated.Reusable {
		t.Fatalf("generate request: %+v", generated)
	}
	if !tt.HasText("凭据密钥只显示一次，请立即复制保存。") || !tt.HasText("cred-9") {
		t.Fatal("generated credential not shown")
	}
	// The list refreshed after generation; revoke goes through confirmation.
	if err := tt.Click("吊销凭据"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("确认吊销") {
		t.Fatal("revoke lacks confirmation")
	}
	if err := tt.Click("确认吊销"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	if revoked != "cred-9" {
		t.Fatalf("revoked %q", revoked)
	}
}

func TestPortalCredentialErrorSurfaces(t *testing.T) {
	a := devicePortalFixture(t)
	a.credentialListFn = func(ctx context.Context) ([]easytier.Credential, error) {
		return nil, context.DeadlineExceeded
	}
	tt := ui.NewTester(a.view, 1200, 2600)
	if err := tt.Click("刷新凭据列表"); err != nil {
		t.Fatal(err)
	}
	flushAsync(t, a)
	tt.Frame()
	if a.credentialErr == "" || !tt.HasText(a.credentialErr) {
		t.Fatal("RPC failure was not shown")
	}
}

func TestCredentialGenerationDoesNotBlockUI(t *testing.T) {
	a := devicePortalFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	a.credentialGenerateFn = func(ctx context.Context, request easytier.CredentialRequest) (easytier.GeneratedCredential, error) {
		close(started)
		<-release
		return easytier.GeneratedCredential{ID: "cred-blocked", Secret: "secret"}, nil
	}
	a.credentialListFn = func(ctx context.Context) ([]easytier.Credential, error) {
		return nil, nil
	}
	tt := ui.NewTester(a.view, 1200, 2600)
	before := time.Now()
	if err := tt.Click("生成凭据"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("credential generation blocked clicking for %v", elapsed)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("credential generation did not start")
	}
	if !a.asyncBusy("credentials:" + a.portalContextKey()) {
		t.Fatal("credential generation is not pending")
	}
	before = time.Now()
	tt.Frame()
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("rendering blocked for %v", elapsed)
	}
	close(release)
	flushAsync(t, a)
	tt.Frame()
	if !tt.HasText("cred-blocked") {
		t.Fatal("generated credential was not shown after the RPC completed")
	}
}
