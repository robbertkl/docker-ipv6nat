package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"net"

	"github.com/fsouza/go-dockerclient"
	"github.com/robbertkl/docker-ipv6nat"
)

const buildVersion = "0.4.2"

var (
	cleanup       bool
	retry         bool
	userlandProxy bool
	version       bool
	debug         bool
	mapIpv4       string
)

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: docker-ipv6 [options]

Automatically configure IPv6 NAT for running docker containers

Options:`)
	flag.PrintDefaults()

	fmt.Fprintln(os.Stderr, `
Environment Variables:
  DOCKER_HOST - default value for -endpoint
  DOCKER_CERT_PATH - directory path containing key.pem, cert.pem and ca.pem
  DOCKER_TLS_VERIFY - enable client TLS verification
`)

	fmt.Fprintln(os.Stderr, `For more information, see https://github.com/robbertkl/docker-ipv6nat`)
}

func initFlags() {
	flag.BoolVar(&cleanup, "cleanup", false, "remove rules when shutting down")
	flag.BoolVar(&retry, "retry", false, "keep retrying to reconnect after a disconnect")
	flag.BoolVar(&version, "version", false, "show version")
	flag.BoolVar(&debug, "debug", false, "log ruleset changes to stdout")
	flag.StringVar(&mapIpv4, "map-ipv4", "", "IPv4 listen address mapping (IPV4/CIDR=IPV4,...)")

	flag.Usage = usage
	flag.Parse()
}

func main() {
	initFlags()

	if version {
		fmt.Println(buildVersion)
		return
	}

	if flag.NArg() > 0 {
		usage()
		os.Exit(1)
	}

	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func parseIpMapping(mapIpv4 string) (map[*net.IPNet]net.IP, error) {
	res := map[*net.IPNet]net.IP{}
	for _, map46 := range strings.Split(mapIpv4, ",") {
		s := strings.Split(map46, "=")
		if len(s)==2 && s[0] != "" && s[1] != "" {
			_, ip4, err := net.ParseCIDR(s[0])
			if err != nil {
				return nil, fmt.Errorf("Cannot parse %+v IPv4, %e", map46, err)
			}
			ip6 := net.ParseIP(s[1])
			if ip6 == nil {
				return nil, fmt.Errorf("Cannot parse %+v IPv6, %e", map46)
			}
			res[ip4] = ip6
		}
	}
	return res, nil
}

func run() error {
	if debug {
		log.Println("docker-ipv6nat is running in debug mode")
	}

	client, err := docker.NewClientFromEnv()
	if err != nil {
		return err
	}

	ipMap, err := parseIpMapping(mapIpv4)
	if err != nil {
		return err
	}

	state, err := dockeripv6nat.NewState(debug, ipMap)
	if err != nil {
		return err
	}

	if cleanup {
		defer func() {
			if err := state.Cleanup(); err != nil {
				log.Printf("%v", err)
			}
		}()
	}

	watcher := dockeripv6nat.NewWatcher(client, state, retry)
	if err := watcher.Watch(); err != nil {
		return err
	}

	return nil
}
