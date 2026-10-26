package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestACLAddEditValidateSave(t *testing.T) {
	a := newTestApp(t)
	a.page = "acl"
	tt := ui.NewTester(a.view, 1200, 2400)
	if err := tt.Click("配置 ACL"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("添加 Inbound 规则"); err != nil {
		t.Fatal(err)
	}
	a.aclRule.value.Name = "ssh"
	a.aclRule.ports = "22, 8000-9000"
	a.aclRule.sourceIPs = "10.0.0.0/24"
	if err := tt.Click("应用规则"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存 ACL"); err != nil {
		t.Fatal(err)
	}
	profiles, err := easytier.LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Config.ACL == nil || len(profiles[0].Config.ACL.V1.Chains[0].Rules) != 1 {
		t.Fatalf("ACL not saved: %+v", profiles)
	}
	a.editACLRule(0, 0)
	a.aclRule.ports = "99999"
	a.applyACLRule()
	if !a.aclRuleOpen || a.aclErr == "" {
		t.Fatal("invalid port accepted")
	}
	if a.draft().config.ACL.V1.Chains[0].Rules[0].Ports[0] != "22" {
		t.Fatal("invalid edit modified saved rule")
	}
}

func TestACLDraftDoesNotMutateProfile(t *testing.T) {
	a := newTestApp(t)
	a.profiles[0].Config.ACL = easytier.DefaultACL()
	a.draft().config.ACL.V1.Chains[0].DefaultAction = 2
	if a.profiles[0].Config.ACL.V1.Chains[0].DefaultAction != 1 {
		t.Fatal("unsaved draft modified profile")
	}
}

func TestACLChainAddAndDelete(t *testing.T) {
	a := newTestApp(t)
	a.page = "acl"
	tt := ui.NewTester(a.view, 1200, 2400)
	if err := tt.Click("配置 ACL"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("添加 Inbound 链"); err != nil {
		t.Fatal(err)
	}
	chains := a.draft().config.ACL.V1.Chains
	if len(chains) != 4 || chains[3].Type != 1 || chains[3].Name != "Inbound 2" {
		t.Fatalf("chain not added: %+v", chains)
	}
	if !tt.HasText("Inbound 2") {
		t.Fatal("new chain card not shown")
	}
	if err := a.draft().config.ACL.Validate(); err != nil {
		t.Fatalf("same-type chains rejected: %v", err)
	}
	if err := tt.Click("删除链 #4"); err != nil {
		t.Fatal(err)
	}
	if chains := a.draft().config.ACL.V1.Chains; len(chains) != 3 {
		t.Fatalf("chain not removed: %+v", chains)
	}
}

// The rename button commits the draft name and rewrites every rule reference
// in one step.
func TestACLRenameGroupUpdatesRules(t *testing.T) {
	a := newTestApp(t)
	a.page = "acl"
	tt := ui.NewTester(a.view, 1200, 2400)
	if err := tt.Click("配置 ACL"); err != nil {
		t.Fatal(err)
	}
	acl := a.draft().config.ACL
	acl.V1.Group.Declares = []easytier.ACLGroupIdentity{{Name: "admin"}, {Name: "ops"}}
	acl.V1.Chains[0].Rules = []easytier.ACLRule{{Name: "ssh", Priority: 100, Enabled: true, Protocol: 5, Action: 1, SourceGroups: []string{"admin"}}}
	tt.Frame()
	a.aclGroupNames[0] = "ops"
	if err := tt.Click("重命名组 1"); err != nil {
		t.Fatal(err)
	}
	if acl.V1.Group.Declares[0].Name != "admin" || acl.V1.Chains[0].Rules[0].SourceGroups[0] != "admin" {
		t.Fatal("duplicate rename changed state")
	}
	a.aclGroupNames[0] = "admins"
	if err := tt.Click("重命名组 1"); err != nil {
		t.Fatal(err)
	}
	if acl.V1.Group.Declares[0].Name != "admins" {
		t.Fatalf("group not renamed: %+v", acl.V1.Group.Declares)
	}
	if acl.V1.Chains[0].Rules[0].SourceGroups[0] != "admins" {
		t.Fatalf("rule reference not updated: %+v", acl.V1.Chains[0].Rules[0])
	}
}

// After a rule is saved with a hand-set priority the chain is in match order.
func TestACLRuleApplySortsByPriority(t *testing.T) {
	a := newTestApp(t)
	a.page = "acl"
	tt := ui.NewTester(a.view, 1200, 2400)
	if err := tt.Click("配置 ACL"); err != nil {
		t.Fatal(err)
	}
	a.editACLRule(0, -1)
	a.aclRule.value.Name = "low"
	a.aclRule.value.Priority = 10
	a.applyACLRule()
	a.editACLRule(0, -1)
	a.aclRule.value.Name = "high"
	a.aclRule.value.Priority = 500
	a.applyACLRule()
	rules := a.draft().config.ACL.V1.Chains[0].Rules
	if len(rules) != 2 || rules[0].Name != "high" || rules[1].Name != "low" {
		t.Fatalf("rules not sorted after add: %+v", rules)
	}
	// Editing the lower rule's priority above the other moves it up.
	a.editACLRule(0, 1)
	a.aclRule.value.Priority = 900
	a.applyACLRule()
	rules = a.draft().config.ACL.V1.Chains[0].Rules
	if rules[0].Name != "low" || rules[0].Priority != 900 {
		t.Fatalf("rules not resorted after edit: %+v", rules)
	}
}
