package easytier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run against the official easytier-web binary; opt-in when it is not on
// PATH, never substitute a mock for the promised server recovery test.
func TestE2EConfigServerPushAndReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("real Config Server integration")
	}
	binary := os.Getenv("EASYTIER_WEB")
	if binary == "" {
		binary, _ = exec.LookPath("easytier-web")
	}
	if binary == "" {
		t.Skip("set EASYTIER_WEB to the official v2.6.4 easytier-web")
	}
	requireBinaries(t)
	root := t.TempDir()
	t.Cleanup(SetConfigFile(filepath.Join(root, "config.json")))
	serverPort, err := freePort("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	apiPort, err := freePort("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "server.db")
	var server *exec.Cmd
	startServer := func() {
		server = exec.Command(binary, "--db", db, "--config-server-protocol", "tcp", "--config-server-port", fmt.Sprint(serverPort), "--api-server-port", fmt.Sprint(apiPort), "--api-server-addr", "127.0.0.1", "--allow-auto-create-user", "--internal-auth-token", "test-internal", "--console-log-level", "error", "--file-log-level", "off")
		if err := server.Start(); err != nil {
			t.Fatal(err)
		}
	}
	stopServer := func() {
		if server != nil {
			_ = server.Process.Kill()
			_ = server.Wait()
			server = nil
		}
	}
	t.Cleanup(stopServer)
	startServer()
	controller := NewCentralController()
	t.Cleanup(func() { controller.StopAndWait(5 * time.Second) })
	config := CentralConfig{Endpoint: fmt.Sprintf("tcp://127.0.0.1:%d/integration-token", serverPort), MachineID: NewMachineID()}
	if err := controller.Start(config); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	api := fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	request := func(method, path string, body any) ([]byte, error) {
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, api+path, bytes.NewReader(data))
		req.Header.Set("X-Internal-Auth", "test-internal")
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, out)
		}
		return out, nil
	}
	wait := func(label string, fn func() bool) {
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if fn() {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("timeout %s: %+v", label, controller.Snapshot())
	}
	var user int
	wait("authenticated server heartbeat", func() bool {
		data, err := request("GET", "/api/internal/sessions", nil)
		if err != nil {
			return false
		}
		var sessions []struct {
			MachineID string `json:"machine_id"`
			UserID    int    `json:"user_id"`
		}
		if json.Unmarshal(data, &sessions) != nil {
			return false
		}
		for _, s := range sessions {
			if s.MachineID == config.MachineID {
				user = s.UserID
				return true
			}
		}
		return false
	})
	id := NewMachineID()
	path := fmt.Sprintf("/api/internal/users/%d/machines/%s/networks", user, config.MachineID)
	network := map[string]any{"instance_id": id, "network_name": "pushed-network", "network_secret": "test-secret", "hostname": "server-managed", "networking_method": 2, "no_tun": true, "listener_urls": []string{}, "dhcp": false}
	if _, err := request("POST", path, map[string]any{"config": network, "save": true}); err != nil {
		t.Fatal(err)
	}
	wait("pushed network", func() bool {
		snap := controller.Snapshot()
		if snap.Portal == "" {
			return false
		}
		networks, err := ListRemoteNetworks(context.Background(), snap.Portal)
		if err != nil {
			return false
		}
		for _, n := range networks {
			if n.ID == id {
				return true
			}
		}
		return false
	})
	stopServer()
	// Existing server-managed data plane must survive a management outage.
	wait("network survives outage", func() bool {
		networks, err := ListRemoteNetworks(context.Background(), controller.Snapshot().Portal)
		return err == nil && len(networks) > 0
	})
	startServer()
	wait("reconnected stable machine", func() bool {
		data, err := request("GET", "/api/internal/sessions", nil)
		return err == nil && strings.Contains(string(data), config.MachineID)
	})
	// Restart Core as well: same MachineID plus server persistence recovers config.
	controller.StopAndWait(5 * time.Second)
	if err := controller.Start(config); err != nil {
		t.Fatal(err)
	}
	wait("recovered configuration after Core restart", func() bool {
		snap := controller.Snapshot()
		if snap.Phase != PhaseRunning {
			return false
		}
		networks, err := ListRemoteNetworks(context.Background(), snap.Portal)
		if err != nil {
			return false
		}
		for _, n := range networks {
			if n.ID == id {
				return true
			}
		}
		return false
	})
}

func TestCentralLifecycleLogsRedactToken(t *testing.T) {
	c := NewCentralController()
	config := CentralConfig{Endpoint: "tcp://localhost:1234/secret-token"}
	c.observeLine("Successfully connected to tcp://localhost:1234/secret-token", config)
	if !c.Snapshot().TransportConnected || strings.Contains(strings.Join(c.Snapshot().Logs, ""), "secret-token") {
		t.Fatal("bad transport state or token leak")
	}
	c.observeLine("heartbeat failed: server unavailable", config)
	if c.Snapshot().TransportConnected {
		t.Fatal("failed heartbeat shown as connected")
	}
}
