package easytier

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// ServiceSettings are the install-time options of the system service. Every
// field empty keeps the automatic behavior: the default config directory
// under the instance root, a picked free RPC port, and no file logging.
// The official GUI (easytier-gui ModeSwitcher) offers the same four options
// and always passes --config-dir/--rpc-portal/--file-log-level/--file-log-dir
// to the service install; here they stay optional.
type ServiceSettings struct {
	// ConfigDir is the directory the service loads network TOMLs from.
	ConfigDir string `json:"config_dir,omitempty"`
	// LogDir is where the service Core writes its file log (easytier.log).
	LogDir string `json:"log_dir,omitempty"`
	// FileLogLevel enables the service's file log; empty leaves it off, as
	// the Core defaults to off.
	FileLogLevel string `json:"file_log_level,omitempty"`
	// RPCAddress is the service's --rpc-portal as host:port; empty picks a
	// free loopback port at install time.
	RPCAddress string `json:"rpc_address,omitempty"`
}

// Validate rejects options the service install could not honor.
func (s ServiceSettings) Validate() error {
	if s.ConfigDir != "" && !filepath.IsAbs(s.ConfigDir) {
		return fmt.Errorf("%s", locale.T("服务配置目录必须是绝对路径"))
	}
	if s.LogDir != "" && !filepath.IsAbs(s.LogDir) {
		return fmt.Errorf("%s", locale.T("服务日志目录必须是绝对路径"))
	}
	if s.FileLogLevel != "" && !validLogLevel(s.FileLogLevel) {
		return fmt.Errorf(locale.T("服务文件日志级别无效：%s"), s.FileLogLevel)
	}
	if s.RPCAddress != "" {
		if _, err := normalizeServiceRPCAddress(s.RPCAddress); err != nil {
			return err
		}
	}
	return nil
}

// normalizeServiceRPCAddress accepts host:port or tcp://host:port and returns
// the bare host:port the Core's --rpc-portal expects.
func normalizeServiceRPCAddress(address string) (string, error) {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return "", err
	}
	_, port, _ := net.SplitHostPort(address)
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf(locale.T("服务 RPC 端口无效：%s"), port)
	}
	return address, nil
}

// serviceConfigsDir is the directory the service loads its networks from:
// the configured one, or <instance root>/service-configs.
func serviceConfigsDir(settings ServiceSettings) (string, error) {
	if settings.ConfigDir != "" {
		return settings.ConfigDir, nil
	}
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "service-configs"), nil
}

// centralConfigsDir is where a Config Server mode service keeps the networks
// the server pushes: the configured directory, or <root>/central-configs.
func centralConfigsDir(settings ServiceSettings) (string, error) {
	if settings.ConfigDir != "" {
		return settings.ConfigDir, nil
	}
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "central-configs"), nil
}
