package easytier

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The writeConfigFlags contract rests on the stable core dropping flags it
// does not know. This test proves it against the real embedded v2.6.4: a
// control instance and one carrying the main-only keys (disable_relay_data,
// prefer_peer_relay, socket_mark, ipv6_public_addr_provider) must both reach
// a steady running state; an early exit means the key was rejected.
func TestE2EStableCoreToleratesMainFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	core, _ := requireBinaries(t)

	mark := uint32(42)
	withMainFlags := testConfig()
	withMainFlags.DisableRelayData = true
	withMainFlags.PreferPeerRelay = true
	withMainFlags.IPv6PublicProvider = true
	withMainFlags.SocketMark = &mark

	for _, test := range []struct {
		name   string
		config Config
	}{
		{"control", testConfig()},
		{"main-flags", withMainFlags},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := test.config
			config.IPv4 = "10.144.150.1/24"
			port, err := freePort("127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			text, err := buildInstanceTOML(config, port, "flag-tolerance-"+test.name)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(core, "-c", path)
			output := filepath.Join(t.TempDir(), "core.log")
			log, err := os.Create(output)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			cmd.Stdout, cmd.Stderr = log, log
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				select {
				case <-exited:
				case <-time.After(5 * time.Second):
				}
			})
			select {
			case err := <-exited:
				data, _ := os.ReadFile(output)
				t.Fatalf("core exited early with the %s config: %v\n%s", test.name, err, data)
			case <-time.After(4 * time.Second):
			}
		})
	}
}
