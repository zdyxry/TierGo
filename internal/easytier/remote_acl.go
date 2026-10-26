package easytier

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// RemoteConfigACL maps the acl section of the official NetworkConfig to the
// typed ACL model, so the ACL editor works on remote instances too. The
// section is decoded through protojson with enum numbers, matching the
// model's numeric fields. Fields inside the acl section that the model does
// not know are not retained across a WithACL round trip; unknown fields
// outside it and every other section stay untouched.

func aclConfigField() protoreflect.FieldDescriptor {
	return networkDescriptor.Fields().ByName("acl")
}

// ACL reads the remote config's acl section, nil when it has none.
func (config RemoteConfig) ACL() (*ACL, error) {
	msg := dynamicpb.NewMessage(networkDescriptor)
	if err := proto.Unmarshal(config.Wire, msg); err != nil {
		return nil, err
	}
	field := aclConfigField()
	if field == nil || !msg.Has(field) {
		return nil, nil
	}
	data, err := (protojson.MarshalOptions{UseProtoNames: true, UseEnumNumbers: true}).Marshal(msg.Get(field).Message().Interface())
	if err != nil {
		return nil, err
	}
	var acl ACL
	if err := json.Unmarshal(data, &acl); err != nil {
		return nil, fmt.Errorf("ACL JSON: %w", err)
	}
	return &acl, nil
}

// WithACL returns the config with only the acl section replaced; nil clears
// it. The ACL is validated before anything is serialized, and each chain's
// rules are written in priority order, which is Core's match order.
func (config RemoteConfig) WithACL(acl *ACL) (RemoteConfig, error) {
	if err := acl.Validate(); err != nil {
		return config, err
	}
	field := aclConfigField()
	if field == nil {
		return config, fmt.Errorf("%s", "embedded schema has no acl field")
	}
	msg := dynamicpb.NewMessage(networkDescriptor)
	if err := proto.Unmarshal(config.Wire, msg); err != nil {
		return config, err
	}
	if acl == nil {
		msg.Clear(field)
	} else {
		data, err := json.Marshal(acl.Sorted())
		if err != nil {
			return config, err
		}
		section := dynamicpb.NewMessage(field.Message())
		if err := protojson.Unmarshal(data, section); err != nil {
			return config, fmt.Errorf("ACL JSON: %w", err)
		}
		msg.Set(field, protoreflect.ValueOfMessage(section))
	}
	wire, err := proto.Marshal(msg)
	if err != nil {
		return config, err
	}
	config.Wire = wire
	return config, nil
}
