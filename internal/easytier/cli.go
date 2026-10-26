package easytier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/corebin"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// The client drives a native easytier-core process of its own: one instance,
// its own configuration directory, its own ports. It never touches another
// EasyTier installation on the machine.
//
// The core owns the TUN device, routes, subnet proxies and DNS, exactly as
// the official command does, so the app has no platform networking code.

// Binary names. Override with these environment variables to run a build
// from a source tree:
//
//	EASYTIER_CORE=/path/to/easytier-core
//	EASYTIER_CLI=/path/to/easytier-cli
const (
	coreBinaryEnv = "EASYTIER_CORE"
	cliBinaryEnv  = "EASYTIER_CLI"
	coreName      = "easytier-core"
	cliName       = "easytier-cli"
)

// LocateBinaries finds the two programs the client runs. A build that
// carries the core writes it out first and uses that; otherwise the machine's
// own installation is used.
func LocateBinaries(root string) (core, cli, embeddedVersion string, err error) {
	var extra []string
	if binary, err := corebin.Ensure(filepath.Join(root, "bin")); err == nil {
		extra = []string{binary.Dir}
		embeddedVersion = binary.Version
	}
	core, err = findBinary(coreBinaryEnv, coreName, extra...)
	if err != nil {
		return "", "", embeddedVersion, err
	}
	cli, err = findBinary(cliBinaryEnv, cliName, extra...)
	if err != nil {
		return "", "", embeddedVersion, err
	}
	return core, cli, embeddedVersion, nil
}

// findBinary looks for an EasyTier tool: the environment variable first, then
// the directories given (the embedded core, when this build carries one),
// then the directory of this executable, then a bin directory beside the
// user's configuration, then PATH.
func findBinary(env, name string, extraDirs ...string) (string, error) {
	if path := os.Getenv(env); path != "" {
		if err := executable(path); err != nil {
			return "", fmt.Errorf("%s=%s: %w", env, path, err)
		}
		return path, nil
	}
	var tried []string
	for _, dir := range append(extraDirs, searchDirs()...) {
		path := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			path += ".exe"
		}
		tried = append(tried, path)
		if err := executable(path); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf(locale.T("找不到 %s，请把它放在 %s，或用 %s 指定路径"),
		name, strings.Join(tried[:min(2, len(tried))], "、"), env)
}

func executable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New(locale.T("是一个目录"))
	}
	return nil
}

func searchDirs() []string {
	dirs := []string{}
	if dir, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(dir))
	}
	if dir, err := InstanceRoot(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "bin"))
	}
	return dirs
}

// ---- easytier-cli output ----
//
// The shapes below mirror `easytier-cli -o json` of EasyTier 2.6. Numbers the
// CLI humanizes ("0.11", "2.69 kB", "0.0%", "-") stay strings and are parsed
// where they are shown.

type cliNode struct {
	PeerID     uint32      `json:"peer_id"`
	IPv4Addr   string      `json:"ipv4_addr"`
	ProxyCIDRs []string    `json:"proxy_cidrs"`
	Hostname   string      `json:"hostname"`
	StunInfo   *cliStun    `json:"stun_info"`
	InstID     string      `json:"inst_id"`
	Listeners  []string    `json:"listeners"`
	Version    string      `json:"version"`
	Config     string      `json:"config"`
	Feature    *cliFeature `json:"feature_flag"`
	IPList     *struct {
		IPv4s []struct {
			Addr uint32 `json:"addr"`
		} `json:"interface_ipv4s"`
		IPv6s      []cliIPv6 `json:"interface_ipv6s"`
		PublicIPv6 *cliIPv6  `json:"public_ipv6"`
	} `json:"ip_list"`
}

type cliStun struct {
	UDPNATType int      `json:"udp_nat_type"`
	TCPNATType int      `json:"tcp_nat_type"`
	PublicIP   []string `json:"public_ip"`
	MinPort    uint32   `json:"min_port"`
	MaxPort    uint32   `json:"max_port"`
}

type cliFeature struct {
	IsPublicServer bool `json:"is_public_server"`
	AvoidRelayData bool `json:"avoid_relay_data"`
	KCPInput       bool `json:"kcp_input"`
	QuicInput      bool `json:"quic_input"`
	NeedP2P        bool `json:"need_p2p"`
}

type cliPeer struct {
	CIDR        string `json:"cidr"`
	IPv4        string `json:"ipv4"`
	Hostname    string `json:"hostname"`
	Cost        string `json:"cost"`
	LatencyMS   string `json:"lat_ms"`
	LossRate    string `json:"loss_rate"`
	RxBytes     string `json:"rx_bytes"`
	TxBytes     string `json:"tx_bytes"`
	TunnelProto string `json:"tunnel_proto"`
	NATType     string `json:"nat_type"`
	ID          string `json:"id"`
	Version     string `json:"version"`
}

type cliRoute struct {
	IPv4         string  `json:"ipv4"`
	Hostname     string  `json:"hostname"`
	ProxyCIDRs   string  `json:"proxy_cidrs"`
	NextHopIPv4  string  `json:"next_hop_ipv4"`
	NextHopName  string  `json:"next_hop_hostname"`
	NextHopLat   float64 `json:"next_hop_lat"`
	PathLen      int     `json:"path_len"`
	PathLatency  int     `json:"path_latency"`
	PathLatFirst int     `json:"path_latency_lat_first"`
	Version      string  `json:"version"`
}

// cliQuery runs one `easytier-cli -o json` command against a core.
func cliQuery(ctx context.Context, cli, portal, command string, out any) error {
	return cliQueryArgs(ctx, cli, portal, out, command)
}

// cliQueryArgs also supports subcommands and -v for unformatted counters.
func cliQueryArgs(ctx context.Context, cli, portal string, out any, args ...string) error {
	command := strings.Join(args, " ")
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(queryCtx, cli, append([]string{"-p", portal, "-o", "json"}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("easytier-cli %s: %s", command, message)
	}
	body := strings.TrimSpace(stdout.String())
	if body == "" {
		return fmt.Errorf(locale.T("easytier-cli %s 没有输出"), command)
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		return fmt.Errorf(locale.T("解析 easytier-cli %s 输出失败：%w"), command, err)
	}
	return nil
}
