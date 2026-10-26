package easytier

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceSettingsValidate(t *testing.T) {
	valid := []ServiceSettings{
		{},
		{ConfigDir: t.TempDir()},
		{LogDir: t.TempDir()},
		{FileLogLevel: "info"},
		{RPCAddress: "127.0.0.1:15999"},
		{RPCAddress: "tcp://0.0.0.0:22020"},
		{RPCAddress: "[::1]:15999"},
	}
	for _, settings := range valid {
		if err := settings.Validate(); err != nil {
			t.Errorf("%+v rejected: %v", settings, err)
		}
	}
	invalid := []ServiceSettings{
		{ConfigDir: "relative/dir"},
		{LogDir: "relative/dir"},
		{FileLogLevel: "verbose"},
		{RPCAddress: "15999"},
		{RPCAddress: ":15999"},
		{RPCAddress: "127.0.0.1:"},
		{RPCAddress: "127.0.0.1:0"},
		{RPCAddress: "127.0.0.1:65536"},
		{RPCAddress: "127.0.0.1:abc"},
		{RPCAddress: "udp://127.0.0.1:15999"},
	}
	for _, settings := range invalid {
		if err := settings.Validate(); err == nil {
			t.Errorf("%+v accepted", settings)
		}
	}
}

func TestServiceSettingsRoundTrip(t *testing.T) {
	defer SetConfigFile(filepath.Join(t.TempDir(), "config.json"))()

	settings := AppSettings{
		Service: ServiceSettings{
			ConfigDir:    "/srv/easytier/configs",
			LogDir:       "/var/log/easytier",
			FileLogLevel: "info",
			RPCAddress:   "127.0.0.1:15999",
		},
	}
	if err := settings.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Service != settings.Service {
		t.Fatalf("service settings = %+v, want %+v", loaded.Service, settings.Service)
	}
}

// With every option empty the install command is exactly the automatic one:
// default config directory, a picked free RPC port, no file logging.
func TestServiceInstallDefaultsUnchanged(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)

	if err := ServiceInstall(context.Background(), []Profile{NewProfile("公司")}, ServiceSettings{}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((*calls)[0], " ")
	root, err := InstanceRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joined, "--config-dir "+filepath.Join(root, "service-configs")) {
		t.Errorf("default config dir not used: %s", joined)
	}
	for _, absent := range []string{"--file-log-level", "--file-log-dir"} {
		if strings.Contains(joined, absent) {
			t.Errorf("empty options must not pass %s: %s", absent, joined)
		}
	}
	// The RPC address still comes from a picked free port and is recorded.
	address, err := ServiceRPCAddress()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(address, "127.0.0.1:") {
		t.Fatalf("service RPC address = %q", address)
	}
	// The install records the embedded core version for drift detection.
	marker, err := os.ReadFile(filepath.Join(root, "service-configs", coreVersionMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(marker)) == "" {
		t.Fatal("the core version marker is empty")
	}
}

// Every option on: the install command carries them all.
func TestServiceInstallWithOptions(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)
	configDir := t.TempDir()
	logDir := t.TempDir()
	settings := ServiceSettings{
		ConfigDir:    configDir,
		LogDir:       logDir,
		FileLogLevel: "info",
		RPCAddress:   "127.0.0.1:15999",
	}

	profile := NewProfile("公司")
	profile.Config = testConfig()
	if err := ServiceInstall(context.Background(), []Profile{profile}, settings); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((*calls)[0], " ")
	for _, want := range []string{
		"--config-dir " + configDir,
		"--rpc-portal 127.0.0.1:15999",
		"--file-log-level info",
		"--file-log-dir " + logDir,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("install args miss %q: %s", want, joined)
		}
	}
	// The profiles land in the configured directory, with the marker.
	if _, err := os.Stat(filepath.Join(configDir, profile.ID+".toml")); err != nil {
		t.Errorf("profile config not in the configured directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, coreVersionMarkerName)); err != nil {
		t.Errorf("core version marker not in the configured directory: %v", err)
	}
	// The configured RPC address is recorded for the remote page.
	address, err := ServiceRPCAddress()
	if err != nil {
		t.Fatal(err)
	}
	if address != "127.0.0.1:15999" {
		t.Fatalf("service RPC address = %q", address)
	}
}

func TestServiceInstallRejectsInvalidOptions(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)
	if err := ServiceInstall(context.Background(), nil, ServiceSettings{RPCAddress: "nope"}); err == nil {
		t.Fatal("an invalid RPC address was installed")
	}
	if len(*calls) != 0 {
		t.Fatalf("the install ran despite invalid options: %v", *calls)
	}
}

func TestServiceInstallCentralWithOptions(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)
	configDir := t.TempDir()
	settings := ServiceSettings{ConfigDir: configDir, FileLogLevel: "warn"}
	config := CentralConfig{Endpoint: "udp://127.0.0.1:22020/test", MachineID: NewMachineID()}

	if err := ServiceInstallCentral(context.Background(), config, settings); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join((*calls)[0], " ")
	for _, want := range []string{
		"--config-server " + config.Endpoint,
		"--config-dir " + configDir,
		"--file-log-level warn",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("central install args miss %q: %s", want, joined)
		}
	}
	// Without a log directory override, none is passed.
	if strings.Contains(joined, "--file-log-dir") {
		t.Errorf("empty log dir must not be passed: %s", joined)
	}
	// The marker goes into the Config Server mode config directory.
	if _, err := os.Stat(filepath.Join(configDir, coreVersionMarkerName)); err != nil {
		t.Errorf("central mode marker missing: %v", err)
	}
}

func TestCoreVersionDrift(t *testing.T) {
	dir := t.TempDir()

	// No marker yet: nothing to compare.
	if _, drift, err := coreVersionDrift(dir, "v2.6.4"); err != nil || drift {
		t.Fatalf("missing marker: drift=%v err=%v", drift, err)
	}
	if err := writeCoreVersionMarker(dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, coreVersionMarkerName)); !os.IsNotExist(err) {
		t.Fatal("an external core must not write a marker")
	}
	if err := writeCoreVersionMarker(dir, "v2.6.3"); err != nil {
		t.Fatal(err)
	}
	installed, drift, err := coreVersionDrift(dir, "v2.6.4")
	if err != nil || !drift || installed != "v2.6.3" {
		t.Fatalf("drift = %v installed = %q err = %v", drift, installed, err)
	}
	if _, drift, _ := coreVersionDrift(dir, "v2.6.3"); drift {
		t.Fatal("same version reported as drift")
	}
	// An unknown current version (external core) never reports drift.
	if _, drift, _ := coreVersionDrift(dir, ""); drift {
		t.Fatal("external core reported as drift")
	}
}

// ServiceCoreDrift compares the marker from the last install with the
// embedded core of this build.
func TestServiceCoreDrift(t *testing.T) {
	stubBinaries(t)
	root, err := InstanceRoot()
	if err != nil {
		t.Fatal(err)
	}
	_, current, ok := ServiceCoreDrift(ServiceSettings{})
	if ok {
		t.Fatal("no marker yet, drift reported")
	}
	if current == "" {
		t.Skip("this build carries no embedded core")
	}
	dir := filepath.Join(root, "service-configs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeCoreVersionMarker(dir, "v0.0.0-old"); err != nil {
		t.Fatal(err)
	}
	installed, got, ok := ServiceCoreDrift(ServiceSettings{})
	if !ok || installed != "v0.0.0-old" || got != current {
		t.Fatalf("drift = %v installed = %q current = %q", ok, installed, got)
	}
	// Reinstalling rewrites the marker and clears the drift.
	if err := writeCoreVersionMarker(dir, current); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := ServiceCoreDrift(ServiceSettings{}); ok {
		t.Fatal("drift survived a matching marker")
	}
}
