package easytier

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

// aclWireFixture is a NetworkConfig with an acl section, another section and
// an unknown wire field, built through the embedded official schema.
func aclWireFixture(t *testing.T) []byte {
	t.Helper()
	msg := dynamicpb.NewMessage(networkDescriptor)
	document := `{
		"network_name": "fixture",
		"mtu": 1400,
		"acl": {"acl_v1": {
			"chains": [{
				"name": "Inbound", "chain_type": 1, "description": "d", "enabled": true, "default_action": 2,
				"rules": [{
					"name": "ssh", "priority": 100, "enabled": true, "protocol": 1,
					"ports": ["22", "8000-9000"], "source_ips": ["10.0.0.0/24"],
					"action": 1, "rate_limit": 50, "burst_limit": 100, "stateful": true,
					"source_groups": ["ops"]
				}]
			}],
			"group": {"declares": [{"group_name": "ops", "group_secret": "s3cr3t"}], "members": ["ops"]}
		}}
	}`
	if err := protojson.Unmarshal([]byte(document), msg); err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	// An unknown field must survive an ACL round trip.
	return wireInt(wire, 999, 42)
}

func TestRemoteACLRoundTrip(t *testing.T) {
	config := RemoteConfig{Network: RemoteNetwork{ID: NewMachineID()}, Wire: aclWireFixture(t)}
	acl, err := config.ACL()
	if err != nil {
		t.Fatal(err)
	}
	if acl == nil || len(acl.V1.Chains) != 1 {
		t.Fatalf("acl = %+v", acl)
	}
	chain := acl.V1.Chains[0]
	if chain.Name != "Inbound" || chain.Type != 1 || chain.DefaultAction != 2 || !chain.Enabled {
		t.Fatalf("chain = %+v", chain)
	}
	rule := chain.Rules[0]
	if rule.Name != "ssh" || rule.Protocol != 1 || rule.Action != 1 || rule.Priority != 100 ||
		rule.RateLimit != 50 || rule.BurstLimit != 100 || !rule.Stateful ||
		len(rule.Ports) != 2 || rule.SourceIPs[0] != "10.0.0.0/24" || rule.SourceGroups[0] != "ops" {
		t.Fatalf("rule = %+v", rule)
	}
	if acl.V1.Group.Declares[0].Name != "ops" || acl.V1.Group.Declares[0].Secret != "s3cr3t" ||
		acl.V1.Group.Members[0] != "ops" {
		t.Fatalf("group = %+v", acl.V1.Group)
	}

	// Edit and write back: only the acl section changes.
	acl.V1.Chains[0].DefaultAction = 1
	acl.V1.Chains[0].Rules[0].Ports = []string{"443"}
	updated, err := config.WithACL(acl)
	if err != nil {
		t.Fatal(err)
	}
	back, err := updated.ACL()
	if err != nil {
		t.Fatal(err)
	}
	if back.V1.Chains[0].DefaultAction != 1 || back.V1.Chains[0].Rules[0].Ports[0] != "443" {
		t.Fatalf("edited acl = %+v", back.V1.Chains[0])
	}
	msg := dynamicpb.NewMessage(networkDescriptor)
	if err := proto.Unmarshal(updated.Wire, msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.GetUnknown()) == 0 {
		t.Fatal("unknown wire field lost")
	}
	name := networkDescriptor.Fields().ByName("network_name")
	if msg.Get(name).String() != "fixture" {
		t.Fatal("other sections changed")
	}
	mtu := networkDescriptor.Fields().ByName("mtu")
	if msg.Get(mtu).Int() != 1400 {
		t.Fatal("mtu changed")
	}

	// Clearing removes the section; reading it gives nil.
	cleared, err := updated.WithACL(nil)
	if err != nil {
		t.Fatal(err)
	}
	if acl, err := cleared.ACL(); err != nil || acl != nil {
		t.Fatalf("cleared acl = %+v, %v", acl, err)
	}

	// A config without the section reads as nil.
	bare := RemoteConfig{Network: RemoteNetwork{ID: NewMachineID()}}
	if acl, err := bare.ACL(); err != nil || acl != nil {
		t.Fatalf("bare acl = %+v, %v", acl, err)
	}

	// An invalid ACL is rejected before any serialization.
	invalid := DefaultACL()
	invalid.V1.Chains[0].Rules = []ACLRule{{Name: "bad", Priority: 1, Protocol: 1, Action: 1, Ports: []string{"99999"}}}
	if _, err := config.WithACL(invalid); err == nil {
		t.Fatal("invalid ACL accepted")
	}
}
