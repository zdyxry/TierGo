package easytier

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func testACL() *ACL {
	acl := DefaultACL()
	acl.V1.Group = ACLGroup{Declares: []ACLGroupIdentity{{Name: "admin", Secret: "group-secret"}}, Members: []string{"admin"}}
	acl.V1.Chains[0].Rules = []ACLRule{{Name: "ssh", Description: "administrators", Priority: 1000, Enabled: true,
		Protocol: 1, Ports: []string{"22", "8000-9000"}, SourceIPs: []string{"10.0.0.0/24"}, DestinationIPs: []string{"10.0.0.1"},
		SourcePorts: []string{"1024-65535"}, SourceGroups: []string{"admin"}, DestinationGroups: []string{"server"},
		Action: 1, RateLimit: 100, BurstLimit: 200, Stateful: true}}
	return acl
}
func TestACLTOMLRoundTrip(t *testing.T) {
	config := DefaultConfig()
	config.ACL = testACL()
	text, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	assertACLMatches(t, config.ACL, back.ACL)
}

// TOML encodes nil lists as empty arrays; they have identical ACL semantics.
func assertACLMatches(t *testing.T, want, got *ACL) {
	t.Helper()
	if got == nil {
		t.Fatal("missing ACL")
	}
	if len(got.V1.Chains) != len(want.V1.Chains) || !reflect.DeepEqual(want.V1.Group, got.V1.Group) {
		t.Fatal("ACL chains/groups differ")
	}
	for i := range want.V1.Chains {
		w, g := want.V1.Chains[i], got.V1.Chains[i]
		if len(w.Rules) == 0 && len(g.Rules) == 0 {
			w.Rules, g.Rules = nil, nil
		}
		if !reflect.DeepEqual(w, g) {
			t.Fatalf("chain mismatch: %#v != %#v", w, g)
		}
	}
}

func TestACLValidation(t *testing.T) {
	for _, mutate := range []func(*ACL){
		func(a *ACL) { a.V1.Chains[0].Type = 9 },
		func(a *ACL) { a.V1.Chains[0].DefaultAction = 0 },
		func(a *ACL) { a.V1.Chains[0].Rules[0].Protocol = 9 },
		func(a *ACL) { a.V1.Chains[0].Rules[0].Ports = []string{"9000-8000"} },
		func(a *ACL) { a.V1.Chains[0].Rules[0].SourceIPs = []string{"garbage"} },
		func(a *ACL) { a.V1.Chains[0].Rules[0].Priority = 65536 },
		func(a *ACL) { a.V1.Group.Declares = append(a.V1.Group.Declares, a.V1.Group.Declares[0]) },
	} {
		a := testACL()
		mutate(a)
		if a.Validate() == nil {
			t.Fatal("invalid ACL accepted")
		}
	}
}

// Several chains of one type are legal; only out-of-range types are not.
func TestACLValidateAllowsSameTypeChains(t *testing.T) {
	acl := testACL()
	acl.V1.Chains = append(acl.V1.Chains, NewChain(1, acl.V1.Chains), NewChain(2, acl.V1.Chains))
	if err := acl.Validate(); err != nil {
		t.Fatalf("same-type chains rejected: %v", err)
	}
	acl.V1.Chains[3].Type = 7
	if err := acl.Validate(); err == nil {
		t.Fatal("out-of-range chain type accepted")
	}
}

func TestNewChain(t *testing.T) {
	chains := DefaultACL().V1.Chains
	extra := NewChain(1, chains)
	if extra.Name != "Inbound 2" || extra.Type != 1 || !extra.Enabled || extra.DefaultAction != 1 {
		t.Fatalf("unexpected chain: %+v", extra)
	}
	again := NewChain(1, append(append([]ACLChain{}, chains...), extra))
	if again.Name != "Inbound 3" {
		t.Fatalf("name not deduplicated: %q", again.Name)
	}
	if outbound := NewChain(2, chains); outbound.Name != "Outbound 2" {
		t.Fatalf("name not deduplicated per base: %q", outbound.Name)
	}
	if unknown := NewChain(9, nil); unknown.Name != "Chain 9" || unknown.Type != 9 {
		t.Fatalf("unknown type: %+v", unknown)
	}
}

// A rename updates the declaration and every rule reference across chains,
// and leaves names that merely contain the old one untouched.
func TestRenameGroup(t *testing.T) {
	acl := DefaultACL()
	acl.V1.Group.Declares = []ACLGroupIdentity{{Name: "admin", Secret: "s"}, {Name: "admin2"}}
	acl.V1.Chains[0].Rules = []ACLRule{{Name: "a", SourceGroups: []string{"admin", "admin2"}, DestinationGroups: []string{"admin"}}}
	acl.V1.Chains[2].Rules = []ACLRule{{Name: "b", DestinationGroups: []string{"admin"}}}
	if err := RenameGroup(acl, "admin", " ops "); err != nil {
		t.Fatal(err)
	}
	if acl.V1.Group.Declares[0].Name != "ops" || acl.V1.Group.Declares[1].Name != "admin2" {
		t.Fatalf("declares: %+v", acl.V1.Group.Declares)
	}
	if got := acl.V1.Chains[0].Rules[0].SourceGroups; !reflect.DeepEqual(got, []string{"ops", "admin2"}) {
		t.Fatalf("source groups: %v", got)
	}
	if got := acl.V1.Chains[0].Rules[0].DestinationGroups[0]; got != "ops" {
		t.Fatalf("destination group: %q", got)
	}
	if got := acl.V1.Chains[2].Rules[0].DestinationGroups[0]; got != "ops" {
		t.Fatalf("other chain not updated: %q", got)
	}
	if err := RenameGroup(acl, "ops", "admin2"); err == nil {
		t.Fatal("rename onto an existing group accepted")
	}
	if err := RenameGroup(acl, "ops", "  "); err == nil {
		t.Fatal("empty rename accepted")
	}
	if err := RenameGroup(acl, "ghost", "x"); err == nil {
		t.Fatal("rename of a missing group accepted")
	}
}

func TestSortChainRulesAndSorted(t *testing.T) {
	chain := &ACLChain{Rules: []ACLRule{{Name: "low", Priority: 10}, {Name: "high-a", Priority: 100}, {Name: "high-b", Priority: 100}}}
	SortChainRules(chain)
	names := []string{chain.Rules[0].Name, chain.Rules[1].Name, chain.Rules[2].Name}
	if !reflect.DeepEqual(names, []string{"high-a", "high-b", "low"}) {
		t.Fatalf("not stably sorted descending: %v", names)
	}

	acl := DefaultACL()
	acl.V1.Chains[0].Rules = []ACLRule{{Name: "low", Priority: 1}, {Name: "high", Priority: 100}}
	sorted := acl.Sorted()
	if sorted.V1.Chains[0].Rules[0].Name != "high" {
		t.Fatal("Sorted did not order by priority")
	}
	if acl.V1.Chains[0].Rules[0].Name != "low" {
		t.Fatal("Sorted mutated the receiver")
	}
	if (*ACL)(nil).Sorted() != nil {
		t.Fatal("nil ACL did not sort to nil")
	}
}

// The exported TOML carries rules in match order whatever order the editor
// kept them in.
func TestACLExportSortsRules(t *testing.T) {
	config := DefaultConfig()
	config.ACL = testACL()
	config.ACL.V1.Chains[0].Rules = []ACLRule{
		{Name: "low", Priority: 1, Enabled: true, Protocol: 5, Action: 1},
		{Name: "high", Priority: 100, Enabled: true, Protocol: 5, Action: 1},
	}
	text, err := ExportConfigTOML(config)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseConfigTOML(text)
	if err != nil {
		t.Fatal(err)
	}
	rules := back.ACL.V1.Chains[0].Rules
	if len(rules) != 2 || rules[0].Name != "high" || rules[1].Name != "low" {
		t.Fatalf("exported rules not in priority order: %+v", rules)
	}
}
func TestE2ECoreLoadsACL(t *testing.T) {
	if testing.Short() {
		t.Skip("real Core integration")
	}
	_, cli := requireBinaries(t)
	t.Cleanup(SetConfigFile(filepath.Join(t.TempDir(), "config.json")))
	config := testConfig()
	config.ACL = testACL()
	controller := NewController("acl")
	controller.Start(config)
	t.Cleanup(func() { controller.StopAndWait(10 * time.Second) })
	snap := waitForPhase(t, controller, PhaseRunning, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var node cliNode
	if err := cliQuery(ctx, cli, snap.Portal, "node", &node); err != nil {
		t.Fatal(err)
	}
	var parsed tomlDocument
	if err := toml.Unmarshal([]byte(node.Config), &parsed); err != nil {
		t.Fatal(err)
	}
	assertACLMatches(t, config.ACL, parsed.ACL)
}
