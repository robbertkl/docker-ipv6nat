package dockeripv6nat

import (
	"errors"
	"net"
	"strconv"

	"github.com/coreos/go-iptables/iptables"
)

type managedNetwork struct {
	id         string
	bridge     string
	subnet     net.IPNet
	icc        bool
	masquerade bool
	internal   bool
	binding    net.IP
}

type managedContainer struct {
	id      string
	bridge  string
	address net.IP
	ports   []managedPort
}

type managedPort struct {
	port        uint16
	proto       string
	hostAddress net.IP
	hostPort    uint16
}

// Manager controls the firewall by managing rules for Docker networks and containers
type Manager struct {
	fw          *Firewall
	hairpinMode bool
}

// NewManager constructs a new Manager
func NewManager(debug bool) (*Manager, error) {
	hairpinMode, err := detectHairpinMode()
	if err != nil {
		return nil, err
	}

	fw, err := NewFirewall(debug)
	if err != nil {
		return nil, err
	}

	if err := fw.EnsureUserFilterChain(); err != nil {
		return nil, err
	}

	if err := fw.EnsureTableChains(getCustomTableChains()); err != nil {
		return nil, err
	}

	if err := fw.EnsureRules(getBaseRules(hairpinMode)); err != nil {
		return nil, err
	}

	return &Manager{
		fw:          fw,
		hairpinMode: hairpinMode,
	}, nil
}

func detectHairpinMode() (bool, error) {
	// Use the IPv4 firewall to detect if the docker daemon is started with --userland-proxy=false.

	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err != nil {
		return false, err
	}

	hairpinModeOffRulespec := []string{
		"!", "-d", "127.0.0.0/8",
		"-m", "addrtype",
		"--dst-type", "LOCAL",
		"-j", "DOCKER",
	}

	hairpinModeOnRulespec := hairpinModeOffRulespec[3:]

	hairpinModeOn, err := ipt.Exists(TableNat, ChainOutput, hairpinModeOnRulespec...)
	if err != nil {
		return false, err
	} else if hairpinModeOn {
		return true, nil
	}

	hairpinModeOff, err := ipt.Exists(TableNat, ChainOutput, hairpinModeOffRulespec...)
	if err != nil {
		return false, err
	} else if hairpinModeOff {
		return false, nil
	}

	// Old iptables misinterprets prefix matches in new iptables
	hairpinModeOffRulespec[2] = "127.0.0.0/32"

	hairpinModeOff, err = ipt.Exists(TableNat, ChainOutput, hairpinModeOffRulespec...)
	if err != nil {
		return false, err
	} else if hairpinModeOff {
		return false, nil
	}

	return false, errors.New("unable to detect hairpin mode (is the docker daemon running?)")
}

// Cleanup removes the base rules and table-chains (per-network / per-container rules should already be removed)
func (m *Manager) Cleanup() error {
	if err := m.fw.RemoveRules(getBaseRules(m.hairpinMode)); err != nil {
		return err
	}

	if err := m.fw.RemoveTableChains(getCustomTableChains()); err != nil {
		return err
	}

	return nil
}

// ReplaceNetwork applies relative rule changes for a network
func (m *Manager) ReplaceNetwork(oldNetwork, newNetwork *managedNetwork) error {
	return m.applyRules(getRulesForNetwork(oldNetwork, m.hairpinMode), getRulesForNetwork(newNetwork, m.hairpinMode))
}

// ReplaceContainer applies relative rule changes for a container
func (m *Manager) ReplaceContainer(oldContainer, newContainer *managedContainer) error {
	return m.applyRules(getRulesForContainer(oldContainer, m.hairpinMode), getRulesForContainer(newContainer, m.hairpinMode))
}

func (m *Manager) applyRules(oldRules, newRules *Ruleset) error {
	oldRules = oldRules.Diff(newRules)

	if err := m.fw.EnsureRules(newRules); err != nil {
		return err
	}

	if err := m.fw.RemoveRules(oldRules); err != nil {
		return err
	}

	return nil
}

func getCustomTableChains() []TableChain {
	return []TableChain{
		{TableFilter, ChainDocker},
		{TableFilter, ChainDockerIsolation1},
		{TableFilter, ChainDockerIsolation2},
		{TableNat, ChainDocker},
	}
}

func getBaseRules(hairpinMode bool) *Ruleset {
	outputRule := NewRule(TableNat, ChainOutput,
		"-m", "addrtype",
		"--dst-type", "LOCAL",
		"-j", ChainDocker)

	if !hairpinMode {
		outputRule.spec = append(outputRule.spec, "!", "-d", "::1")
	}

	return &Ruleset{
		NewPrependRule(TableFilter, ChainForward,
			"-j", ChainDockerUser),
		NewPrependRule(TableFilter, ChainForward,
			"-j", ChainDockerIsolation1),
		NewRule(TableFilter, ChainDockerIsolation1,
			"-j", "RETURN"),
		NewRule(TableFilter, ChainDockerIsolation2,
			"-j", "RETURN"),
		NewRule(TableNat, ChainPrerouting,
			"-m", "addrtype",
			"--dst-type", "LOCAL",
			"-j", ChainDocker),
		outputRule,
	}
}

func getRulesForNetwork(network *managedNetwork, hairpinMode bool) *Ruleset {
	if network == nil {
		return &Ruleset{}
	}

	iccAction := "ACCEPT"
	if !network.icc {
		iccAction = "DROP"
	}

	if network.internal {
		return &Ruleset{
			// internal: drop traffic to docker network from foreign subnet
			// notice: rule is different from IPv4 counterpart because NDP should not be blocked
			NewPrependRule(TableFilter, ChainDockerIsolation1,
				"!", "-i", network.bridge,
				"-o", network.bridge,
				"-j", "DROP"),
			// internal: drop traffic from docker network to foreign subnet
			// notice: rule is different from IPv4 counterpart because NDP should not be blocked
			NewPrependRule(TableFilter, ChainDockerIsolation1,
				"!", "-o", network.bridge,
				"-i", network.bridge,
				"-j", "DROP"),
			// ICC
			NewRule(TableFilter, ChainForward,
				"-i", network.bridge,
				"-o", network.bridge,
				"-j", iccAction),
		}
	}

	rs := Ruleset{
		// not internal: catch if packet wants to leave docker network (stage 1)
		NewPrependRule(TableFilter, ChainDockerIsolation1,
			"-i", network.bridge,
			"!", "-o", network.bridge,
			"-j", ChainDockerIsolation2),
		// not internal: if packet wants to enter another docker network, drop it (stage 2)
		NewPrependRule(TableFilter, ChainDockerIsolation2,
			"-o", network.bridge,
			"-j", "DROP"),
		// not internal: check ingoing traffic to docker network for new connections in additional chain
		NewRule(TableFilter, ChainForward,
			"-o", network.bridge,
			"-j", ChainDocker),
		// not internal: allow ingoing traffic to docker network for established connections
		NewRule(TableFilter, ChainForward,
			"-o", network.bridge,
			"-m", "conntrack",
			"--ctstate", "RELATED,ESTABLISHED",
			"-j", "ACCEPT"),
		// not internal: allow outgoing traffic from docker network
		NewRule(TableFilter, ChainForward,
			"-i", network.bridge,
			"!", "-o", network.bridge,
			"-j", "ACCEPT"),
		// ICC
		NewRule(TableFilter, ChainForward,
			"-i", network.bridge,
			"-o", network.bridge,
			"-j", iccAction),
		// masquerade packets if they enter the docker network
		NewPrependRule(TableNat, ChainPostrouting,
			"-o", network.bridge,
			"-m", "addrtype",
			"--dst-type", "LOCAL",
			"-j", "MASQUERADE"),
	}

	if network.masquerade {
		rs = append(rs,
			// masquerade packets if they leave the docker network
			NewPrependRule(TableNat, ChainPostrouting,
				"-s", network.subnet.String(),
				"!", "-o", network.bridge,
				"-j", "MASQUERADE"),
		)
	}

	if !hairpinMode {
		rs = append(rs, NewPrependRule(TableNat, ChainDocker,
			"-i", network.bridge,
			"-j", "RETURN"))
	}

	return &rs
}

func getRulesForContainer(container *managedContainer, hairpinMode bool) *Ruleset {
	if container == nil {
		return &Ruleset{}
	}

	rs := make(Ruleset, 0, len(container.ports)*3)
	for _, port := range container.ports {
		rs = append(rs, *getRulesForPort(&port, container, hairpinMode)...)
	}

	return &rs
}

func getRulesForPort(port *managedPort, container *managedContainer, hairpinMode bool) *Ruleset {
	containerPortString := strconv.Itoa(int(port.port))
	hostPortString := strconv.Itoa(int(port.hostPort))
	hostAddressString := "0/0"
	if !port.hostAddress.IsUnspecified() {
		hostAddressString = port.hostAddress.String()
	}

	dnatRule := NewRule(TableNat, ChainDocker,
		"-d", hostAddressString,
		"-p", port.proto,
		"-m", port.proto,
		"--dport", hostPortString,
		"-j", "DNAT",
		"--to-destination", net.JoinHostPort(container.address.String(), containerPortString))

	if !hairpinMode {
		dnatRule.spec = append(dnatRule.spec, "!", "-i", container.bridge)
	}

	return &Ruleset{
		NewRule(TableFilter, ChainDocker,
			"-d", container.address.String(),
			"!", "-i", container.bridge,
			"-o", container.bridge,
			"-p", port.proto,
			"-m", port.proto,
			"--dport", containerPortString,
			"-j", "ACCEPT"),
		NewRule(TableNat, ChainPostrouting,
			"-s", container.address.String(),
			"-d", container.address.String(),
			"-p", port.proto,
			"-m", port.proto,
			"--dport", containerPortString,
			"-j", "MASQUERADE"),
		dnatRule,
	}
}
