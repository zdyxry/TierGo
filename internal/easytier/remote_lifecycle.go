package easytier

import (
	"context"
	"fmt"
	"net/netip"
	"reflect"

	"github.com/zdyxry/easytier-mygo/internal/locale"
	"google.golang.org/protobuf/encoding/protowire"
)

// Remote configuration uses the official NetworkConfig protobuf projection.
// Only the basic fields below are supported by this first lifecycle slice;
// reject advanced settings rather than silently drop them on a remote host.
func basicRemoteConfig(c Config, id string) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	// Compare all non-basic fields to the defaults so newly added model
	// fields cannot accidentally disappear during remote creation.
	basic := DefaultConfig()
	basic.NetworkName, basic.NetworkSecret, basic.Hostname, basic.IPv4 = c.NetworkName, c.NetworkSecret, c.Hostname, c.IPv4
	basic.Peers, basic.Listeners, basic.ProxyNetworks = c.Peers, c.Listeners, c.ProxyNetworks
	basic.EnableTUN, basic.Encryption, basic.LatencyFirst = c.EnableTUN, c.Encryption, c.LatencyFirst
	basic.MultiThread, basic.BindDevice, basic.EnableIPv6 = c.MultiThread, c.BindDevice, c.EnableIPv6
	basic.LogLevel = c.LogLevel
	if !reflect.DeepEqual(c, basic) {
		return nil, fmt.Errorf("%s", locale.T("远端创建暂仅支持基础网络字段；高级设置请在远端配置，不会静默丢弃"))
	}
	var out []byte
	putString := func(n protowire.Number, v string) { out = wireBytes(out, n, []byte(v)) }
	putBool := func(n protowire.Number, v bool) {
		var x uint64
		if v {
			x = 1
		}
		out = wireInt(out, n, x)
	}
	putString(1, id)
	putString(5, c.Hostname)
	putString(6, c.NetworkName)
	putString(7, c.NetworkSecret)
	out = wireInt(out, 8, 1) // Manual peers, not official public-server default
	putBool(2, c.EnableTUN && c.IPv4 == "")
	if c.IPv4 != "" {
		prefix, err := netip.ParsePrefix(c.IPv4)
		if err != nil {
			return nil, err
		}
		putString(3, prefix.Addr().String())
		out = wireInt(out, 4, uint64(prefix.Bits()))
	}
	for _, peer := range c.Peers {
		putString(10, peer)
	}
	for _, cidr := range c.ProxyNetworks {
		putString(11, cidr)
	}
	for _, listener := range c.Listeners {
		putString(17, listener)
	}
	putBool(19, c.LatencyFirst)
	putBool(25, c.BindDevice)
	putBool(26, !c.EnableTUN)
	putBool(29, c.MultiThread)
	putBool(36, !c.Encryption)
	putBool(47, !c.EnableIPv6)
	return out, nil
}

func CreateRemoteNetwork(ctx context.Context, address string, config Config) (string, error) {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return "", err
	}
	id := newProfileID()
	configBytes, err := basicRemoteConfig(config, id)
	if err != nil {
		return "", err
	}
	uuid, err := encodeUUID(id)
	if err != nil {
		return "", err
	}
	request := wireBytes(nil, 1, uuid)
	request = wireBytes(request, 2, configBytes)
	request = wireInt(request, 3, 0)
	if _, err := managementRPC(ctx, address, "WebClientService", 2, request); err != nil {
		return "", err
	}
	return id, nil
}

// Delete is explicit and separate from disconnecting. Core checks read-only
// and no-delete file permissions. No caller uses Retain(all except ID), which
// could accidentally stop instances created concurrently by another client.
func DeleteRemoteNetwork(ctx context.Context, address string, network RemoteNetwork) error {
	if network.Permissions&2 != 0 {
		return fmt.Errorf("%s", locale.T("远端网络禁止删除"))
	}
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return err
	}
	if network.Disabled {
		return removeDisabledRemote(address, network.ID)
	}
	id, err := encodeUUID(network.ID)
	if err != nil {
		return err
	}
	_, err = managementRPC(ctx, address, "WebClientService", 6, wireBytes(nil, 1, id))
	return err
}
