package easytier

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// ACL mirrors v2.6.4 acl.proto, including the numeric wire enums used in
// Core TOML. Keeping the model typed prevents arbitrary JSON reaching Core.
type ACL struct {
	V1 ACLV1 `json:"acl_v1" toml:"acl_v1"`
}
type ACLV1 struct {
	Chains []ACLChain `json:"chains" toml:"chains"`
	Group  ACLGroup   `json:"group" toml:"group"`
}
type ACLGroup struct {
	Declares []ACLGroupIdentity `json:"declares" toml:"declares"`
	Members  []string           `json:"members" toml:"members"`
}
type ACLGroupIdentity struct {
	Name   string `json:"group_name" toml:"group_name"`
	Secret string `json:"group_secret" toml:"group_secret"`
}
type ACLChain struct {
	Name          string    `json:"name" toml:"name"`
	Type          int       `json:"chain_type" toml:"chain_type"`
	Description   string    `json:"description" toml:"description"`
	Enabled       bool      `json:"enabled" toml:"enabled"`
	Rules         []ACLRule `json:"rules" toml:"rules"`
	DefaultAction int       `json:"default_action" toml:"default_action"`
}
type ACLRule struct {
	Name              string   `json:"name" toml:"name"`
	Description       string   `json:"description" toml:"description"`
	Priority          uint32   `json:"priority" toml:"priority"`
	Enabled           bool     `json:"enabled" toml:"enabled"`
	Protocol          int      `json:"protocol" toml:"protocol"`
	Ports             []string `json:"ports" toml:"ports"`
	SourceIPs         []string `json:"source_ips" toml:"source_ips"`
	DestinationIPs    []string `json:"destination_ips" toml:"destination_ips"`
	SourcePorts       []string `json:"source_ports" toml:"source_ports"`
	Action            int      `json:"action" toml:"action"`
	RateLimit         uint32   `json:"rate_limit" toml:"rate_limit"`
	BurstLimit        uint32   `json:"burst_limit" toml:"burst_limit"`
	Stateful          bool     `json:"stateful" toml:"stateful"`
	SourceGroups      []string `json:"source_groups" toml:"source_groups"`
	DestinationGroups []string `json:"destination_groups" toml:"destination_groups"`
}

func DefaultACL() *ACL {
	acl := &ACL{}
	for i, name := range []string{"Inbound", "Outbound", "Forward"} {
		acl.V1.Chains = append(acl.V1.Chains, ACLChain{Name: name, Type: i + 1, Enabled: true, DefaultAction: 1})
	}
	return acl
}

// NewChain builds an enabled Allow-default chain of the given type. Several
// chains may share a type, so the default name gains a sequence number when
// an existing chain already uses it.
func NewChain(chainType int, existing []ACLChain) ACLChain {
	name := fmt.Sprintf("Chain %d", chainType)
	switch chainType {
	case 1:
		name = "Inbound"
	case 2:
		name = "Outbound"
	case 3:
		name = "Forward"
	}
	taken := map[string]bool{}
	for _, chain := range existing {
		taken[chain.Name] = true
	}
	base := name
	for suffix := 2; taken[name]; suffix++ {
		name = fmt.Sprintf("%s %d", base, suffix)
	}
	return ACLChain{Name: name, Type: chainType, Enabled: true, DefaultAction: 1}
}

// SortChainRules orders rules by priority, highest first, matching the order
// Core matches them in. The sort is stable so equal priorities keep their
// relative order.
func SortChainRules(chain *ACLChain) {
	slices.SortStableFunc(chain.Rules, func(a, b ACLRule) int {
		return cmp.Compare(b.Priority, a.Priority)
	})
}

// Sorted returns a copy with every chain's rules in match order; the
// receiver is left as the editor arranged it.
func (acl *ACL) Sorted() *ACL {
	if acl == nil {
		return nil
	}
	sorted := *acl
	sorted.V1.Chains = make([]ACLChain, len(acl.V1.Chains))
	for i, chain := range acl.V1.Chains {
		sorted.V1.Chains[i] = chain
		sorted.V1.Chains[i].Rules = slices.Clone(chain.Rules)
		SortChainRules(&sorted.V1.Chains[i])
	}
	return &sorted
}

// RenameGroup renames a declared group and every rule reference to it across
// all chains, so a rename never leaves dangling group names behind.
func RenameGroup(acl *ACL, oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	found := false
	for _, group := range acl.V1.Group.Declares {
		if group.Name == oldName {
			found = true
		} else if group.Name == newName {
			return fmt.Errorf(locale.T("ACL 组名称为空或重复：%q"), newName)
		}
	}
	if newName == "" {
		return fmt.Errorf(locale.T("ACL 组名称为空或重复：%q"), newName)
	}
	if !found {
		return fmt.Errorf(locale.T("ACL 组 %q 不存在"), oldName)
	}
	for i := range acl.V1.Group.Declares {
		if acl.V1.Group.Declares[i].Name == oldName {
			acl.V1.Group.Declares[i].Name = newName
		}
	}
	for i := range acl.V1.Chains {
		for j := range acl.V1.Chains[i].Rules {
			rule := &acl.V1.Chains[i].Rules[j]
			rule.SourceGroups = renameGroupRefs(rule.SourceGroups, oldName, newName)
			rule.DestinationGroups = renameGroupRefs(rule.DestinationGroups, oldName, newName)
		}
	}
	return nil
}

// renameGroupRefs replaces exact matches only; "admin2" stays put when
// "admin" is renamed.
func renameGroupRefs(refs []string, oldName, newName string) []string {
	for i, ref := range refs {
		if ref == oldName {
			refs[i] = newName
		}
	}
	return refs
}

func (acl *ACL) Validate() error {
	if acl == nil {
		return nil
	}
	for _, chain := range acl.V1.Chains {
		if chain.Type < 1 || chain.Type > 3 {
			return fmt.Errorf(locale.T("ACL 链类型无效：%d"), chain.Type)
		}
		if chain.DefaultAction != 1 && chain.DefaultAction != 2 {
			return fmt.Errorf(locale.T("ACL 链 %s 的默认动作必须是 Allow 或 Drop"), chain.Name)
		}
		for _, rule := range chain.Rules {
			if rule.Priority > 65535 || rule.Protocol < 1 || rule.Protocol > 5 || (rule.Action != 1 && rule.Action != 2) {
				return fmt.Errorf(locale.T("ACL 规则 %s 的优先级、协议或动作无效"), rule.Name)
			}
			for _, ip := range append(append([]string{}, rule.SourceIPs...), rule.DestinationIPs...) {
				if _, err := netip.ParseAddr(ip); err != nil {
					if _, err := netip.ParsePrefix(ip); err != nil {
						return fmt.Errorf(locale.T("ACL IP %q 无效"), ip)
					}
				}
			}
			for _, port := range append(append([]string{}, rule.SourcePorts...), rule.Ports...) {
				parts := strings.Split(port, "-")
				if len(parts) > 2 {
					return fmt.Errorf(locale.T("ACL 端口范围 %q 无效"), port)
				}
				previous := 0
				for _, part := range parts {
					n, err := strconv.Atoi(part)
					if err != nil || n < 1 || n > 65535 || n < previous {
						return fmt.Errorf(locale.T("ACL 端口 %q 无效"), port)
					}
					previous = n
				}
			}
		}
	}
	groups := map[string]bool{}
	for _, group := range acl.V1.Group.Declares {
		if strings.TrimSpace(group.Name) == "" || groups[group.Name] {
			return fmt.Errorf(locale.T("ACL 组名称为空或重复：%q"), group.Name)
		}
		groups[group.Name] = true
	}
	return nil
}
