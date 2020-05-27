package dockeripv6nat

import (
	"log"
	"strings"

	"github.com/coreos/go-iptables/iptables"
)

// Table describes an ip(6)tables table
type Table string

// All ip(6)tables tables we use
const (
	TableFilter = "filter"
	TableNat    = "nat"
)

// Chain describes an ip(6)tables chain
type Chain string

// All ip(6)tables chains we use
const (
	ChainInput            = "INPUT"
	ChainOutput           = "OUTPUT"
	ChainPrerouting       = "PREROUTING"
	ChainPostrouting      = "POSTROUTING"
	ChainForward          = "FORWARD"
	ChainDockerUser       = "DOCKER-USER"
	ChainDocker           = "DOCKER"
	ChainDockerIsolation1 = "DOCKER-ISOLATION-STAGE-1"
	ChainDockerIsolation2 = "DOCKER-ISOLATION-STAGE-2"
)

// TableChain references a combination of an ip(6)tables table and chain
type TableChain struct {
	table Table
	chain Chain
}

// Rule represents a unique firewall rule
type Rule struct {
	tc      TableChain
	spec    []string
	prepend bool
}

// NewRule constructs a new (non prepended) Rule
func NewRule(table Table, chain Chain, spec ...string) *Rule {
	return &Rule{
		tc:      TableChain{table, chain},
		spec:    spec,
		prepend: false,
	}
}

// NewPrependRule constructs a new Rule with prepend set to true
func NewPrependRule(table Table, chain Chain, spec ...string) *Rule {
	return &Rule{
		tc:      TableChain{table, chain},
		spec:    spec,
		prepend: true,
	}
}

func (r *Rule) hash() string {
	return strings.Join(r.spec, "#")
}

// Equal compares 2 Rules
func (r *Rule) Equal(other *Rule) bool {
	if r.tc != other.tc {
		return false
	}

	if len(r.spec) != len(other.spec) {
		return false
	}

	for index := range r.spec {
		if r.spec[index] != other.spec[index] {
			return false
		}
	}

	return true
}

// Ruleset contains a list of unique rules
type Ruleset []*Rule

// Contains checks if a Rule is part of the Ruleset
func (s *Ruleset) Contains(r *Rule) bool {
	for _, sr := range *s {
		if r.Equal(sr) {
			return true
		}
	}

	return false
}

// Diff returns a new Ruleset with only the rules that are not part of other
func (s *Ruleset) Diff(other *Ruleset) *Ruleset {
	if len(*other) == 0 {
		return s
	}

	diffed := make(Ruleset, 0, len(*s))
	for _, r := range *s {
		if !other.Contains(r) {
			diffed = append(diffed, r)
		}
	}

	return &diffed
}

// Firewall keeps track of the active rules, in order to perform proper appends/prepends
type Firewall struct {
	ipt               *iptables.IPTables
	activeRules       map[TableChain]map[string]bool
	debug             bool
	userChainJumpRule *Rule
}

// NewFirewall constructs a new Firewall
func NewFirewall(debug bool) (*Firewall, error) {
	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		return nil, err
	}

	return &Firewall{
		ipt:               ipt,
		activeRules:       make(map[TableChain]map[string]bool),
		debug:             debug,
		userChainJumpRule: NewRule(TableFilter, ChainForward, "-j", ChainDockerUser),
	}, nil
}

func (fw *Firewall) activateRule(r *Rule) {
	if _, exists := fw.activeRules[r.tc]; !exists {
		fw.activeRules[r.tc] = make(map[string]bool)
	}
	fw.activeRules[r.tc][r.hash()] = true
}

func (fw *Firewall) deactivateRule(r *Rule) {
	delete(fw.activeRules[r.tc], r.hash())
}

// EnsureTableChains creates (and clears!) the given TableChains
func (fw *Firewall) EnsureTableChains(tableChains []TableChain) error {
	for _, tc := range tableChains {
		if err := fw.ipt.ClearChain(string(tc.table), string(tc.chain)); err != nil {
			return err
		}
		delete(fw.activeRules, tc)
	}

	return nil
}

// RemoveTableChains deletes the given TableChains
func (fw *Firewall) RemoveTableChains(tableChains []TableChain) error {
	for _, tc := range tableChains {
		fw.ipt.ClearChain(string(tc.table), string(tc.chain))
		fw.ipt.DeleteChain(string(tc.table), string(tc.chain))
		delete(fw.activeRules, tc)
	}

	return nil
}

// EnsureRules makes sure the Rules in the given Ruleset exist or it creates them
func (fw *Firewall) EnsureRules(rules *Ruleset) error {
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
			if fw.debug {
				log.Println("rule added: -t", string(rule.tc.table), "-I", string(rule.tc.chain), len(fw.activeRules[rule.tc])+1, strings.Join(rule.spec, " "))
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
			if fw.debug {
				log.Println("rule added: -t", string(rule.tc.table), "-I", string(rule.tc.chain), 1, strings.Join(rule.spec, " "))
			}
		}
		fw.activateRule(rule)
	}

	return nil
}

// RemoveRules makes sure the Rules in the given Ruleset don't exist or removes them
func (fw *Firewall) RemoveRules(rules *Ruleset) error {
	for _, rule := range *rules {
		if rule.Equal(fw.userChainJumpRule) {
			continue
		}

		exists, err := fw.ipt.Exists(string(rule.tc.table), string(rule.tc.chain), rule.spec...)
		if err != nil {
			return err
		}

		if exists {
			if err := fw.ipt.Delete(string(rule.tc.table), string(rule.tc.chain), rule.spec...); err != nil {
				return err
			}
			if fw.debug {
				log.Println("rule removed: -t", string(rule.tc.table), "-D", string(rule.tc.chain), strings.Join(rule.spec, " "))
			}
		}
		fw.deactivateRule(rule)
	}

	return nil
}

// EnsureUserFilterChain makes sure the DOCKER-USER chain exists, without clearing it
func (fw *Firewall) EnsureUserFilterChain() error {
	chains, err := fw.ipt.ListChains(TableFilter)
	if err != nil {
		return err
	}

	exists := false
	for _, chain := range chains {
		if chain == ChainDockerUser {
			exists = true
		}
	}

	if !exists {
		if err = fw.ipt.NewChain(TableFilter, ChainDockerUser); err != nil {
			return err
		}
	}

	if err = fw.ipt.AppendUnique(TableFilter, ChainDockerUser, "-j", "RETURN"); err != nil {
		return err
	}

	exists, err = fw.ipt.Exists(TableFilter, ChainForward, "-j", ChainDockerUser)
	if err != nil {
		return err
	}

	if exists {
		err = fw.ipt.Delete(TableFilter, ChainForward, "-j", ChainDockerUser)
	}
	return err
}
