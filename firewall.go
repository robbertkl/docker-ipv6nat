package dockeripv6nat

import (
	"strings"

	"github.com/coreos/go-iptables/iptables"
)

type Table string

const (
	TableFilter = "filter"
	TableNat    = "nat"
)

type Chain string

const (
	ChainInput           = "INPUT"
	ChainOutput          = "OUTPUT"
	ChainPrerouting      = "PREROUTING"
	ChainPostrouting     = "POSTROUTING"
	ChainForward         = "FORWARD"
	ChainDocker          = "DOCKER"
	ChainDockerIsolation = "DOCKER-ISOLATION"
)

type TableChain struct {
	table Table
	chain Chain
}

type rule struct {
	tc      TableChain
	spec    []string
	prepend bool
}

func NewRule(table Table, chain Chain, spec ...string) *rule {
	return &rule{
		tc:      TableChain{table, chain},
		spec:    spec,
		prepend: false,
	}
}

func NewPrependRule(table Table, chain Chain, spec ...string) *rule {
	return &rule{
		tc:      TableChain{table, chain},
		spec:    spec,
		prepend: true,
	}
}

func (r *rule) hash() string {
	return strings.Join(r.spec, "#")
}

func (r1 *rule) Equal(r2 *rule) bool {
	if r1.tc != r2.tc {
		return false
	}

	if len(r1.spec) != len(r2.spec) {
		return false
	}

	for index := range r1.spec {
		if r1.spec[index] != r2.spec[index] {
			return false
		}
	}

	return true
}

type Ruleset []*rule

func (s *Ruleset) Contains(r *rule) bool {
	for _, sr := range *s {
		if r.Equal(sr) {
			return true
		}
	}

	return false
}

func (s1 *Ruleset) Diff(s2 *Ruleset) *Ruleset {
	if len(*s2) == 0 {
		return s1
	}

	s := make(Ruleset, 0, len(*s1))
	for _, r := range *s1 {
		if !s2.Contains(r) {
			s = append(s, r)
		}
	}

	return &s
}

type firewall struct {
	ipt         *iptables.IPTables
	activeRules map[TableChain]map[string]bool
}

func NewFirewall() (*firewall, error) {
	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		return nil, err
	}

	return &firewall{ipt: ipt, activeRules: make(map[TableChain]map[string]bool)}, nil
}

func (fw *firewall) activateRule(r *rule) {
	if _, exists := fw.activeRules[r.tc]; !exists {
		fw.activeRules[r.tc] = make(map[string]bool)
	}
	fw.activeRules[r.tc][r.hash()] = true
}

func (fw *firewall) deactivateRule(r *rule) {
	delete(fw.activeRules[r.tc], r.hash())
}

func (fw *firewall) EnsureTableChains(tableChains []TableChain) error {
	for _, tc := range tableChains {
		if err := fw.ipt.ClearChain(string(tc.table), string(tc.chain)); err != nil {
			return err
		}
		delete(fw.activeRules, tc)
	}

	return nil
}

func (fw *firewall) RemoveTableChains(tableChains []TableChain) error {
	for _, tc := range tableChains {
		fw.ipt.ClearChain(string(tc.table), string(tc.chain))
		fw.ipt.DeleteChain(string(tc.table), string(tc.chain))
		delete(fw.activeRules, tc)
	}

	return nil
}

func (fw *firewall) EnsureRules(rules *Ruleset) error {
	// A regular loop to append only the non-prepend rules
	for _, rule := range *rules {
		if rule.prepend {
			continue
		}

		exists, err := fw.ipt.Exists(string(rule.tc.table), string(rule.tc.chain), rule.spec...)
		if err != nil {
			return err
		}

		if !exists {
			if err := fw.ipt.Insert(string(rule.tc.table), string(rule.tc.chain), len(fw.activeRules[rule.tc])+1, rule.spec...); err != nil {
				return err
			}
		}
		fw.activateRule(rule)
	}

	// Loop in reverse to insert the prepend rules to the start of the chain
	for index := len(*rules) - 1; index >= 0; index-- {
		rule := (*rules)[index]
		if !rule.prepend {
			continue
		}

		exists, err := fw.ipt.Exists(string(rule.tc.table), string(rule.tc.chain), rule.spec...)
		if err != nil {
			return err
		}

		if !exists {
			if err := fw.ipt.Insert(string(rule.tc.table), string(rule.tc.chain), 1, rule.spec...); err != nil {
				return err
			}
		}
		fw.activateRule(rule)
	}

	return nil
}

func (fw *firewall) RemoveRules(rules *Ruleset) error {
	for _, rule := range *rules {

		exists, err := fw.ipt.Exists(string(rule.tc.table), string(rule.tc.chain), rule.spec...)
		if err != nil {
			return err
		}

		if exists {
			if err := fw.ipt.Delete(string(rule.tc.table), string(rule.tc.chain), rule.spec...); err != nil {
				return err
			}
		}
		fw.deactivateRule(rule)
	}

	return nil
}
