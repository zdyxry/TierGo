package easytier

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// The client can hand its networks to a system service: one easytier-core
// process the operating system starts at boot and restarts when it crashes,
// loading every profile from a config directory. The service is managed
// through `easytier-cli service`, which knows systemd, launchd and the
// Windows service manager, so there is no platform service code here.

// ServiceName is the system service this client installs.
const ServiceName = "easytier-mygo"

// ServiceState is what the operating system reports about the service.
type ServiceState int

const (
	ServiceUnknown ServiceState = iota
	ServiceNotInstalled
	ServiceStopped
	ServiceRunning
)

func (state ServiceState) String() string {
	switch state {
	case ServiceRunning:
		return locale.T("运行中")
	case ServiceStopped:
		return locale.T("已停止")
	case ServiceNotInstalled:
		return locale.T("未安装")
	default:
		return locale.T("未知")
	}
}

// parseServiceStatus reads `easytier-cli service status`, which answers in
// prose ("Service is running") rather than JSON.
func parseServiceStatus(output string) ServiceState {
	switch {
	case strings.Contains(output, "is running"):
		return ServiceRunning
	case strings.Contains(output, "is stopped"):
		return ServiceStopped
	case strings.Contains(output, "not installed"):
		return ServiceNotInstalled
	default:
		return ServiceUnknown
	}
}

// execCLI runs easytier-cli and returns its standard output. Tests replace
// it to script the answers.
var execCLI = func(ctx context.Context, cli string, args ...string) (string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(queryCtx, cli, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("easytier-cli %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

// serviceCLIName appends the service name every operation needs.
func serviceArgs(args ...string) []string {
	return append([]string{"service", "--name", ServiceName}, args...)
}

// ServiceStatus asks the operating system about the service.
func ServiceStatus(ctx context.Context) (ServiceState, error) {
	_, cli, _, err := serviceBinaries()
	if err != nil {
		return ServiceUnknown, err
	}
	output, err := execCLI(ctx, cli, serviceArgs("status")...)
	if err != nil {
		return ServiceUnknown, err
	}
	return parseServiceStatus(output), nil
}

// serviceBinaries finds the tools service operations need, plus the version
// of the embedded core they come from (empty when the core is external).
func serviceBinaries() (core, cli, version string, err error) {
	root, err := InstanceRoot()
	if err != nil {
		return "", "", "", err
	}
	core, cli, version, err = LocateBinaries(root)
	return core, cli, version, err
}

// syncServiceConfigs writes one TOML per profile into the service's config
// directory and removes whatever no longer has a profile, so the service
// runs exactly the current networks.
func syncServiceConfigs(profiles []Profile, settings ServiceSettings) (string, error) {
	dir, err := serviceConfigsDir(settings)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf(locale.T("创建服务配置目录失败：%w"), err)
	}
	stale, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf(locale.T("读取服务配置目录失败：%w"), err)
	}
	live := map[string]bool{}
	for _, profile := range profiles {
		listenPort, _, err := pickPorts(profile.Config.Listeners)
		if err != nil {
			return "", fmt.Errorf(locale.T("为「%s」选择端口失败：%w"), profile.Name, err)
		}
		document, err := buildInstanceTOML(profile.Config, listenPort, profile.InstanceName())
		if err != nil {
			return "", fmt.Errorf(locale.T("生成「%s」的配置失败：%w"), profile.Name, err)
		}
		name := profile.ID + ".toml"
		live[name] = true
		if err := writeFileDurable(filepath.Join(dir, name), []byte(document), 0o600); err != nil {
			return "", err
		}
	}
	for _, entry := range stale {
		if entry.IsDir() || live[entry.Name()] || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		// A leftover config would make the service run a deleted network.
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return "", fmt.Errorf(locale.T("移除旧的服务配置失败：%w"), err)
		}
	}
	return dir, nil
}

// ServiceInstall writes the current networks into the service's config
// directory and (re)installs the service: it starts at boot and the system
// restarts it when it crashes. The settings override the install-time
// defaults (config directory, RPC portal, file logging).
func ServiceInstall(ctx context.Context, profiles []Profile, settings ServiceSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	dir, err := syncServiceConfigs(profiles, settings)
	if err != nil {
		return err
	}
	return installService(ctx, []string{"--config-dir", dir}, dir, dir, settings)
}

// installService runs `easytier-cli service install`. workDir is the
// service's working directory; configDir is where the Core reads networks
// and where the core-version marker goes — they differ in Config Server
// mode, whose networks live in a subdirectory.
//
// The install itself is platform-neutral: easytier-cli knows systemd,
// launchd and the Windows service manager, so the same arguments apply on
// Windows, macOS and Linux.
func installService(ctx context.Context, coreArgs []string, workDir, configDir string, settings ServiceSettings) error {
	core, cli, version, err := serviceBinaries()
	if err != nil {
		return err
	}
	// The service shares the machine, so it must not squat on EasyTier's
	// default management port.
	address := ""
	if settings.RPCAddress != "" {
		address, err = normalizeServiceRPCAddress(settings.RPCAddress)
		if err != nil {
			return err
		}
	} else {
		portal, err := freePort("127.0.0.1")
		if err != nil {
			return err
		}
		address = fmt.Sprintf("127.0.0.1:%d", portal)
	}
	args := serviceArgs("install",
		"--core-path", core,
		"--service-work-dir", workDir,
		"--description", "EasyTier networks configured by the MyGo desktop client",
		"--display-name", "EasyTier (MyGo)",
		"--",
	)
	args = append(args, coreArgs...)
	args = append(args, "--rpc-portal", address)
	if settings.FileLogLevel != "" {
		args = append(args, "--file-log-level", settings.FileLogLevel)
	}
	if settings.LogDir != "" {
		args = append(args, "--file-log-dir", settings.LogDir)
	}
	_, err = execCLI(ctx, cli, args...)
	if err != nil {
		return err
	}
	root, e := InstanceRoot()
	if e != nil {
		return e
	}
	if e := writeFileDurable(filepath.Join(root, "service-rpc"), []byte(address), 0o600); e != nil {
		return e
	}
	// Remember which embedded core the service was installed with, so the
	// window can notice when an upgrade replaced it underneath.
	return writeCoreVersionMarker(configDir, version)
}

// ServiceStart starts the installed service.
func ServiceStart(ctx context.Context) error {
	_, cli, _, err := serviceBinaries()
	if err != nil {
		return err
	}
	_, err = execCLI(ctx, cli, serviceArgs("start")...)
	return err
}

// ServiceStop stops the service without uninstalling it.
func ServiceStop(ctx context.Context) error {
	_, cli, _, err := serviceBinaries()
	if err != nil {
		return err
	}
	_, err = execCLI(ctx, cli, serviceArgs("stop")...)
	return err
}

// ServiceUninstall removes the service; the config directory stays, so
// installing again brings the same networks back.
func ServiceUninstall(ctx context.Context) error {
	_, cli, _, err := serviceBinaries()
	if err != nil {
		return err
	}
	_, err = execCLI(ctx, cli, serviceArgs("uninstall")...)
	return err
}
