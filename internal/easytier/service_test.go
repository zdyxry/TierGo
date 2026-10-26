package easytier

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubCLI records the arguments of every service call and answers with
// scripted output.
func stubCLI(t *testing.T, output string, err error) *[][]string {
	t.Helper()
	calls := &[][]string{}
	previous := execCLI
	execCLI = func(_ context.Context, _ string, args ...string) (string, error) {
		*calls = append(*calls, args)
		return output, err
	}
	t.Cleanup(func() { execCLI = previous })
	return calls
}

// stubBinaries points the client at standing-in binaries.
func stubBinaries(t *testing.T) {
	t.Helper()
	t.Setenv(coreBinaryEnv, "/bin/true")
	t.Setenv(cliBinaryEnv, "/bin/true")
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
}

func TestParseServiceStatus(t *testing.T) {
	for output, want := range map[string]ServiceState{
		"Service is running\n":       ServiceRunning,
		"Service is stopped\n":       ServiceStopped,
		"Service is not installed\n": ServiceNotInstalled,
		"something else":             ServiceUnknown,
	} {
		if got := parseServiceStatus(output); got != want {
			t.Errorf("parseServiceStatus(%q) = %v, want %v", output, got, want)
		}
	}
}

func TestServiceStatusMapsTheAnswer(t *testing.T) {
	stubBinaries(t)
	stubCLI(t, "Service is running\n", nil)
	state, err := ServiceStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state != ServiceRunning {
		t.Fatalf("state = %v", state)
	}
}

func TestServiceStatusReportsFailures(t *testing.T) {
	stubBinaries(t)
	stubCLI(t, "", errors.New("systemd not available"))
	if _, err := ServiceStatus(context.Background()); err == nil {
		t.Fatal("a failing status call must surface")
	}
}

// Installing the service hands it exactly the current networks: one config
// per profile, stale ones removed, autostart and restart-on-failure left to
// the system defaults (on).
func TestServiceInstallWritesEveryProfile(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)

	office := NewProfile("公司")
	office.Config = testConfig()
	home := NewProfile("家庭")
	home.Config = testConfig()
	home.Config.NetworkName = "home-net"
	home.Config.Listeners = []string{"tcp://0.0.0.0:22333"}

	if err := ServiceInstall(context.Background(), []Profile{office, home}, ServiceSettings{}); err != nil {
		t.Fatal(err)
	}

	if len(*calls) != 1 {
		t.Fatalf("calls = %v", *calls)
	}
	args := (*calls)[0]
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"service --name easytier-mygo install",
		"--core-path /bin/true",
		"--config-dir",
		"--rpc-portal 127.0.0.1:",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("install args miss %q: %s", want, joined)
		}
	}

	dir, err := serviceConfigsDir(ServiceSettings{})
	if err != nil {
		t.Fatal(err)
	}
	// Beside the TOMLs the install drops a core-version marker for drift
	// detection; only the TOMLs count as networks.
	tomlCount := func() []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".toml") {
				names = append(names, entry.Name())
			}
		}
		return names
	}
	if names := tomlCount(); len(names) != 2 {
		t.Fatalf("service configs = %v", names)
	}

	// Each config is its own instance; the fixed listener survives.
	homeTOML, err := os.ReadFile(filepath.Join(dir, home.ID+".toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(homeTOML), `instance_name = "`+home.InstanceName()+`"`) {
		t.Errorf("home config lacks its instance name:\n%s", homeTOML)
	}
	if !strings.Contains(string(homeTOML), `tcp://0.0.0.0:22333`) {
		t.Errorf("home config lost its listener:\n%s", homeTOML)
	}

	// A profile removed from the list leaves the service on the next sync.
	if err := ServiceInstall(context.Background(), []Profile{office}, ServiceSettings{}); err != nil {
		t.Fatal(err)
	}
	if names := tomlCount(); len(names) != 1 || names[0] != office.ID+".toml" {
		t.Fatalf("stale configs survived: %v", names)
	}
}

func TestServiceLifecycleCommands(t *testing.T) {
	stubBinaries(t)
	calls := stubCLI(t, "", nil)

	if err := ServiceStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ServiceStop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ServiceUninstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"service", "--name", "easytier-mygo", "start"},
		{"service", "--name", "easytier-mygo", "stop"},
		{"service", "--name", "easytier-mygo", "uninstall"},
	}
	if len(*calls) != len(want) {
		t.Fatalf("calls = %v", *calls)
	}
	for i, call := range *calls {
		if strings.Join(call, " ") != strings.Join(want[i], " ") {
			t.Errorf("call %d = %v, want %v", i, call, want[i])
		}
	}
}
