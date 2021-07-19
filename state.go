package dockeripv6nat

import (
	"log"
	"net"
	"strconv"

	"github.com/fsouza/go-dockerclient"
)

// State keeps track of the current Docker containers and networks to apply relative updates to the manager
type State struct {
	manager    *Manager
	networks   map[string]*managedNetwork
	containers map[string]*managedContainer
	mapIpv4    map[*net.IPNet]net.IP
	debug      bool
}

// fc00::/7, Unique Local IPv6 Unicast Addresses, see RFC 4193
var ulaCIDR = net.IPNet{
	IP:   net.ParseIP("fc00::"),
	Mask: net.CIDRMask(7, 128),
}

// NewState constructs a new state
func NewState(debug bool, mapIpv4 map[*net.IPNet]net.IP) (*State, error) {
	manager, err := NewManager(debug)
	if err != nil {
		return nil, err
	}

	return &State{
		manager:    manager,
		networks:   make(map[string]*managedNetwork),
		containers: make(map[string]*managedContainer),
		mapIpv4:    mapIpv4,
	}, nil
}

// Cleanup resets the state
func (s *State) Cleanup() error {
	s.RemoveMissingContainers([]string{})
	s.RemoveMissingNetworks([]string{})

	if err := s.manager.Cleanup(); err != nil {
		return err
	}

	return nil
}

// RemoveMissingNetworks removes any of the given networks, if they don't exist
func (s *State) RemoveMissingNetworks(networkIDs []string) error {
	for id := range s.networks {
		if !contains(networkIDs, id) {
			if err := s.UpdateNetwork(id, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveMissingContainers removes any of the given containers if they don't exist
func (s *State) RemoveMissingContainers(containerIDs []string) error {
	for id := range s.containers {
		if !contains(containerIDs, id) {
			if err := s.UpdateContainer(id, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// UpdateNetwork applies a network, which can add, remove or update it
func (s *State) UpdateNetwork(id string, network *docker.Network) error {
	oldNetwork := s.networks[id]
	newNetwork := s.parseNetwork(network)

	if oldNetwork != nil || newNetwork != nil {
		if err := s.manager.ReplaceNetwork(oldNetwork, newNetwork); err != nil {
			return err
		}
	}

	if newNetwork == nil {
		delete(s.networks, id)
	} else {
		s.networks[id] = newNetwork
	}

	return nil
}

// UpdateContainer applies a container, which can add, remove or update it
func (s *State) UpdateContainer(id string, container *docker.Container) error {
	oldContainer := s.containers[id]
	newContainer := s.parseContainer(container)

	if oldContainer != nil || newContainer != nil {
		if err := s.manager.ReplaceContainer(oldContainer, newContainer); err != nil {
			return err
		}
	}

	if newContainer == nil {
		delete(s.containers, id)
	} else {
		s.containers[id] = newContainer
	}

	return nil
}

func (s *State) parseNetwork(network *docker.Network) *managedNetwork {
	if network == nil {
		return nil
	}

	// Don't check network.EnableIPv6, since this will be false before Docker 1.11.0, even if we have IPv6 subnets.

	if network.Driver != "bridge" {
		return nil
	}

	n := managedNetwork{
		id:         network.ID,
		bridge:     "br-" + network.ID[:12],
		icc:        true,
		masquerade: true,
		internal:   network.Internal,
		binding:    net.ParseIP("::"),
	}

	for _, config := range network.IPAM.Config {
		_, subnet, err := net.ParseCIDR(config.Subnet)
		if err != nil {
			continue
		}
		if ulaCIDR.Contains(subnet.IP) {
			n.subnet = *subnet
			break
		}
	}

	if n.subnet.IP == nil {
		return nil
	}

	for key, value := range network.Options {
		switch key {
		case "com.docker.network.bridge.name":
			n.bridge = value
		case "com.docker.network.bridge.enable_icc":
			b, err := strconv.ParseBool(value)
			if err != nil {
				log.Printf("invalid value for com.docker.network.bridge.enable_icc (network %s)", network.ID)
				break
			}
			n.icc = b
		case "com.docker.network.bridge.enable_ip_masquerade":
			b, err := strconv.ParseBool(value)
			if err != nil {
				log.Printf("invalid value for com.docker.network.bridge.enable_ip_masquerade (network %s)", network.ID)
				break
			}
			n.masquerade = b
		case "com.docker.network.bridge.host_binding_ipv6":
			ip := net.ParseIP(value)
			if ip == nil || ip.To4() != nil {
				log.Printf("invalid value for com.docker.network.bridge.host_binding_ipv6 (network %s)", network.ID)
				break
			}
			n.binding = ip
		}
	}

	return &n
}

func (s *State) findFirstKnownNetwork(networks map[string]docker.ContainerNetwork) (*managedNetwork, net.IP) {
	for _, network := range networks {
		ip := net.ParseIP(network.GlobalIPv6Address)
		if !ulaCIDR.Contains(ip) {
			continue
		}

		n, found := s.networks[network.NetworkID]
		if !found || n.internal {
			continue
		}

		return n, ip
	}

	return nil, nil
}

func (s *State) getKnownNetworks() []*managedNetwork {
	networks := make([]*managedNetwork, len(s.networks))
	index := 0
	for _, network := range s.networks {
		networks[index] = network
		index++
	}

	return networks
}

func (s *State) parseContainer(container *docker.Container) *managedContainer {
	if container == nil {
		return nil
	}

	network, containerAddress := s.findFirstKnownNetwork(container.NetworkSettings.Networks)
	if network == nil {
		return nil
	}

	if network.internal {
		return nil
	}

	ports := make([]managedPort, 0)
	for port, bindings := range container.HostConfig.PortBindings {
		proto := port.Proto()
		containerPort, err := parsePort(port.Port())
		if err != nil {
			log.Printf("invalid port %s for container %s", port.Port(), container.ID)
			continue
		}

		for _, binding := range bindings {
			hostAddress := network.binding

			if binding.HostIP != "" && binding.HostIP != "0.0.0.0" {
				ip := net.ParseIP(binding.HostIP)
				if ip != nil && ip.To4() != nil {
					// Try map Ipv4 to IPv6
					for ip4net, ip6 := range s.mapIpv4 {
						if ip4net.Contains(ip) {
							log.Printf("Converting listen IP %v to %v (matching %v)", ip, ip6, ip4net)
							ip = ip6
							break
						}
					}
				}
				if ip == nil || ip.To4() != nil {
					// Skip bindings to IPv4.
					continue
				}

				hostAddress = ip
			}

			hostPort, err := parsePort(binding.HostPort)
			if err != nil {
				log.Printf("invalid port %s for container %s", binding.HostPort, container.ID)
				continue
			}

			ports = append(ports, managedPort{
				port:        containerPort,
				proto:       proto,
				hostAddress: hostAddress,
				hostPort:    hostPort,
			})
		}
	}

	if len(ports) == 0 {
		return nil
	}

	return &managedContainer{
		id:      container.ID,
		address: containerAddress,
		bridge:  network.bridge,
		ports:   ports,
	}
}

func parsePort(rawPort string) (uint16, error) {
	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(port), nil
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}

	return false
}
