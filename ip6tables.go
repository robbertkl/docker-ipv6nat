package dockeripv6nat

import (
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
	table   Table
	chain   Chain
	spec    []string
	prepend bool
}

func NewRule(table Table, chain Chain, spec ...string) *rule {
	return &rule{
		table: table,
		chain: chain,
		spec:  spec,
	}
}

func NewPrependRule(table Table, chain Chain, spec ...string) *rule {
	return &rule{
		table:   table,
		chain:   chain,
		spec:    spec,
		prepend: true,
	}
}

func (r1 *rule) Equal(r2 *rule) bool {
	if r1.table != r2.table {
		return false
	}

	if r1.chain != r2.chain {
		return false
	}

	if len(r1.spec) != len(r2.spec) {
		return false
	}

	for index, _ := range r1.spec {
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

type ip6tables struct {
	iptables.IPTables
}

func NewIP6Tables() (*ip6tables, error) {
	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		return nil, err
	}

	return &ip6tables{*ipt}, nil
}

func (ipt *ip6tables) SafeDelete(table, chain string, rulespec ...string) error {
	exists, err := ipt.Exists(table, chain, rulespec...)
	if err != nil {
		return err
	}

	if exists {
		return ipt.Delete(table, chain, rulespec...)
	}

	return nil
}

func (ipt *ip6tables) InsertUnique(table, chain string, pos int, rulespec ...string) error {
	exists, err := ipt.Exists(table, chain, rulespec...)
	if err != nil {
		return err
	}

	if !exists {
		return ipt.Insert(table, chain, pos, rulespec...)
	}

	return nil
}

func (ipt *ip6tables) Prepend(table, chain string, rulespec ...string) error {
	return ipt.Insert(table, chain, 1, rulespec...)
}

func (ipt *ip6tables) PrependUnique(table, chain string, rulespec ...string) error {
	return ipt.InsertUnique(table, chain, 1, rulespec...)
}

func (ipt *ip6tables) EnsureTableChains(tableChains []TableChain) error {
	for _, tc := range tableChains {
		if err := ipt.ClearChain(string(tc.table), string(tc.chain)); err != nil {
			return err
		}
	}

	return nil
}

func (ipt *ip6tables) RemoveTableChains(tableChains []TableChain) error {
	for _, tc := range tableChains {
		ipt.ClearChain(string(tc.table), string(tc.chain))
		ipt.DeleteChain(string(tc.table), string(tc.chain))
	}

	return nil
}

func (ipt *ip6tables) EnsureRules(rules *Ruleset) error {
	// A regular loop to append only the non-prepend rules
	for _, rule := range *rules {
		if rule.prepend {
			continue
		}
		if err := ipt.AppendUnique(string(rule.table), string(rule.chain), rule.spec...); err != nil {
			return err
		}
	}

	// Loop in reverse to insert the prepend rules to the start of the chain
	for index := len(*rules) - 1; index >= 0; index-- {
		rule := (*rules)[index]
		if !rule.prepend {
			continue
		}
		if err := ipt.PrependUnique(string(rule.table), string(rule.chain), rule.spec...); err != nil {
			return err
		}
	}

	return nil
}

func (ipt *ip6tables) RemoveRules(rules *Ruleset) error {
	for _, rule := range *rules {
		if err := ipt.SafeDelete(string(rule.table), string(rule.chain), rule.spec...); err != nil {
			return err
		}
	}

	return nil
}
