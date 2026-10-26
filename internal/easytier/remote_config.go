package easytier

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zdyxry/easytier-mygo/internal/locale"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

//go:embed schema/v2.6.4.pb
var remoteSchema []byte

var networkDescriptor = func() protoreflect.MessageDescriptor {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(remoteSchema, &set); err != nil {
		panic(err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		panic(err)
	}
	desc, err := files.FindDescriptorByName("api.manage.NetworkConfig")
	if err != nil {
		panic(err)
	}
	return desc.(protoreflect.MessageDescriptor)
}()

// RemoteConfig retains the entire official protobuf, including unknown wire
// fields and source ownership. JSON is only its editor view, not a local Config
// projection. Never include these bytes in logs: they contain credentials.
type RemoteConfig struct {
	Network RemoteNetwork `json:"network"`
	Wire    []byte        `json:"wire"`
	Source  uint64        `json:"source"`
}

func (config RemoteConfig) JSON() (string, error) {
	msg := dynamicpb.NewMessage(networkDescriptor)
	if err := proto.Unmarshal(config.Wire, msg); err != nil {
		return "", err
	}
	data, err := (protojson.MarshalOptions{Indent: "  ", UseProtoNames: true}).Marshal(msg)
	return string(data), err
}
func (config RemoteConfig) WithJSON(text string) (RemoteConfig, error) {
	original := dynamicpb.NewMessage(networkDescriptor)
	if err := proto.Unmarshal(config.Wire, original); err != nil {
		return config, err
	}
	edited := dynamicpb.NewMessage(networkDescriptor)
	if err := protojson.Unmarshal([]byte(text), edited); err != nil {
		return config, fmt.Errorf("NetworkConfig JSON: %w", err)
	}
	// instance_id is immutable, even if the editor changes it.
	field := networkDescriptor.Fields().ByName("instance_id")
	edited.Set(field, protoreflect.ValueOfString(config.Network.ID))
	edited.SetUnknown(original.GetUnknown())
	wire, err := proto.Marshal(edited)
	if err != nil {
		return config, err
	}
	config.Wire = wire
	name := networkDescriptor.Fields().ByName("network_name")
	config.Network.Name = edited.Get(name).String()
	return config, nil
}
func GetRemoteConfig(ctx context.Context, address string, network RemoteNetwork) (RemoteConfig, error) {
	if network.Permissions&1 != 0 {
		return RemoteConfig{}, fmt.Errorf("%s", locale.T("远端网络配置只读"))
	}
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return RemoteConfig{}, err
	}
	if network.Disabled {
		return loadDisabledRemote(address, network.ID)
	}
	uuid, err := encodeUUID(network.ID)
	if err != nil {
		return RemoteConfig{}, err
	}
	data, err := managementRPC(ctx, address, "WebClientService", 7, wireBytes(nil, 1, uuid))
	if err != nil {
		return RemoteConfig{}, err
	}
	fields, err := wireFields(data)
	if err != nil {
		return RemoteConfig{}, err
	}
	config := RemoteConfig{Network: network}
	for _, field := range fields {
		switch field.number {
		case 1:
			config.Wire = field.bytes
		case 2:
			config.Source = field.value
		}
	}
	if len(config.Wire) == 0 {
		return config, fmt.Errorf("%s", locale.T("远端没有返回配置"))
	}
	return config, nil
}
func SaveRemoteConfig(ctx context.Context, address string, config RemoteConfig) error {
	if config.Network.Permissions&1 != 0 {
		return fmt.Errorf("%s", locale.T("远端网络配置只读"))
	}
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return err
	}
	// Validate on the actual Core before replacing a running instance.
	if _, err := managementRPC(ctx, address, "WebClientService", 1, wireBytes(nil, 1, config.Wire)); err != nil {
		return err
	}
	if config.Network.Disabled {
		return saveDisabledRemote(address, config)
	}
	return runRemoteConfig(ctx, address, config)
}
func runRemoteConfig(ctx context.Context, address string, config RemoteConfig) error {
	uuid, err := encodeUUID(config.Network.ID)
	if err != nil {
		return err
	}
	request := wireBytes(nil, 1, uuid)
	request = wireBytes(request, 2, config.Wire)
	request = wireInt(request, 3, 1)
	request = wireInt(request, 4, config.Source)
	_, err = managementRPC(ctx, address, "WebClientService", 2, request)
	return err
}
func disabledRemotePath(address, id string) (string, error) {
	if _, err := encodeUUID(id); err != nil {
		return "", err
	}
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return "", err
	}
	root, err := InstanceRoot()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(address))
	return filepath.Join(root, "remote-disabled", hex.EncodeToString(hash[:]), id+".json"), nil
}
func saveDisabledRemote(address string, config RemoteConfig) error {
	path, err := disabledRemotePath(address, config.Network.ID)
	if err != nil {
		return err
	}
	config.Network.Disabled = true
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return writeFileDurable(path, data, 0o600)
}
func loadDisabledRemote(address, id string) (RemoteConfig, error) {
	path, err := disabledRemotePath(address, id)
	if err != nil {
		return RemoteConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RemoteConfig{}, err
	}
	var config RemoteConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if config.Network.ID != id {
		return config, fmt.Errorf("%s", locale.T("停用配置 ID 不匹配"))
	}
	return config, nil
}
func listDisabledRemote(address string) ([]RemoteNetwork, error) {
	path, err := disabledRemotePath(address, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var networks []RemoteNetwork
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		config, err := loadDisabledRemote(address, entry.Name()[:len(entry.Name())-5])
		if err != nil {
			return nil, err
		}
		networks = append(networks, config.Network)
	}
	return networks, nil
}
func removeDisabledRemote(address, id string) error {
	path, err := disabledRemotePath(address, id)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Official GUI semantics: persist complete config before DeleteNetworkInstance,
// then re-run the saved config to enable. An ambiguous delete retains the backup;
// a successful enable removes it. No Retain(list) races with other clients.
func SetRemoteDisabled(ctx context.Context, address string, network RemoteNetwork, disabled bool) error {
	address, err := NormalizeRPCAddress(address)
	if err != nil {
		return err
	}
	if network.Disabled == disabled {
		return nil
	}
	if disabled {
		if network.Permissions&3 != 0 {
			return fmt.Errorf("%s", locale.T("远端网络禁止停用"))
		}
		config, err := GetRemoteConfig(ctx, address, network)
		if err != nil {
			return err
		}
		if err := saveDisabledRemote(address, config); err != nil {
			return err
		}
		if err := DeleteRemoteNetwork(ctx, address, network); err != nil {
			// A transport timeout is ambiguous: Core may already have
			// deleted the instance. Retain the durable backup on any error.
			return err
		}
		return nil
	}
	config, err := loadDisabledRemote(address, network.ID)
	if err != nil {
		return err
	}
	config.Network.Disabled = false
	if err := runRemoteConfig(ctx, address, config); err != nil {
		return err
	}
	return removeDisabledRemote(address, network.ID)
}
