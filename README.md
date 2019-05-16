# docker-ipv6nat

This project mimics the way Docker does NAT for IPv4 and applies it to IPv6. Jump to [Usage](#usage) to get started right away.

## Why would I need this?

Unfortunately, initially Docker was not created with IPv6 in mind.
It was added later and, while it has come a long way, is still not as usable as one would want.
Much discussion is still going on as to how IPv6 should be used in a containerized world; see the various GitHub issues linked below.

Currently, you can let Docker give each container an IPv6 address from your (public) pool, but this has disadvantages:

* Giving each container a publicly routable address means all ports (even unexposed / unpublished ports) are suddenly reachable by everyone, if no additional filtering is done ([docker/docker#21614](https://github.com/docker/docker/issues/21614))
* By default, each container gets a random IPv6, making it impossible to do properly do DNS; the alternative is to assign a specific IPv6 address to each container, still an administrative hassle ([docker/docker#13481](https://github.com/docker/docker/issues/13481))
* Published ports won't work on IPv6, unless you have the userland proxy enabled (which, for now, is enabled by default in Docker)
* The userland proxy, however, seems to be on its way out ([docker/docker#14856](https://github.com/docker/docker/issues/14856)) and has various issues, like:
  * It can use a lot of RAM ([docker/docker#11185](https://github.com/docker/docker/issues/11185))
  * Source IP addresses are rewritten, making it completely unusable for many purposes, e.g. mail servers ([docker/docker#17666](https://github.com/docker/docker/issues/17666), [docker/libnetwork#1099](https://github.com/docker/libnetwork/issues/1099))

Special mention of [@JonasT](https://github.com/JonasT) who submitted the majority of the above issues, pointing out some of the practical issues when using IPv6 with Docker.

So basically, IPv6 for Docker can (depending on your setup) be pretty unusable ([docker/docker#13481](https://github.com/docker/docker/issues/13481)) and completely inconsistent with the way how IPv4 works ([docker/docker#21951](https://github.com/docker/docker/issues/21951)).
Docker images are mostly designed with IPv4 NAT in mind, having NAT provide a layer of security allowing only published ports through, and letting container linking or user-defined networks provide inter-container communication.
This does not go hand in hand with the way Docker IPv6 works, requiring image maintainers to rethink/adapt their images with IPv6 in mind.

## Welcome IPv6 NAT

So what does this repo do? It attempts to resolve all of the above issues by managing `ip6tables` to setup IPv6 NAT for your containers, similar to how it's done by the Docker daemon for IPv4.

* A ULA range ([RFC 4193](https://tools.ietf.org/html/rfc4193)) is used for containers; this automatically means the containers will NOT be publicly routable
* Published ports are forwarded to the corresponding containers, similar to IPv4
* The original IPv6 source addresses are maintained, again, just like with IPv4
* Userland proxy can be turned off and IPv6 will still work

This makes a transition to IPv6 completely painless, without needing to make changes to your images.

Please note:

* The Docker network API is required, so at least Docker 1.9.0
* It triggers only on ULA ranges (so within `fc00::/7`), e.g. `fd00:dead:beef::/48`
* Only networks with driver `bridge` are supported; this includes Docker's default network ("bridge"), as well as user-defined bridge networks

## NAT on IPv6, are you insane?

First of all, thank you for questioning my sanity!
I'm aware NAT on IPv6 is almost always a no-go, since the huge number of available addresses removes the need for it.
However, considering all the above issues related to IPv6 are fixed with IPv6 NAT, I thought: why not?
The concepts of working with Docker images/containers rely heavily on IPv4 NAT, so if this makes IPv6 with Docker usable in the same way, be happy.
I'm in no way "pro IPv6 NAT" in the general case; I'm just "pro working shit".

Probably IPv6 NAT will never make it into Docker, just because it's not "the right way".
This is fine; when a better alternative is found, I'd be happy to use it and get rid of this repo.
However, since the IPv6 support just isn't there yet, and discussions are still ongoing, this repo can be used in the meantime.

Still think IPv6 NAT is a bad idea? That's fine, you're absolutely free to NOT use this repo.

## Usage

The recommended way is to run the Docker image:

```
docker run -d --restart=always -v /var/run/docker.sock:/var/run/docker.sock:ro --privileged --net=host robbertkl/ipv6nat
```

The flags `--privileged` and `--net=host` are necessary because docker-ipv6nat manages the hosts IPv6 firewall using ip6tables.

To limit runtime privileges as a security precaution, the `--privileged` flag can be replaced with `--cap-add=NET_ADMIN --cap-add=SYS_MODULE`.

If you're a security fan (it's not bad), you can drop all capabilities `--cap-drop=ALL` and leave only `--cap-add=NET_RAW --cap-add=NET_ADMIN --cap-add=SYS_MODULE`. About it you can read in a good [article](https://www.redhat.com/en/blog/secure-your-containers-one-weird-trick) from RedHat.

Alternatively, you can download the latest release from the [release page](https://github.com/robbertkl/docker-ipv6nat/releases) and run it directly on your host.
See `docker-ipv6nat --help` for usage flags.

## Docker IPv6 configuration

Instructions below show ways to enable IPv6 and are not specific to docker-ipv6nat.
Just make sure to use a ULA range in order for docker-ipv6nat to pick them up.

### Option A: default bridge network

To use IPv6, make sure your Docker daemon is started with `--ipv6` and specifies a ULA range with `--fixed-cidr-v6` (e.g. `--fixed-cidr-v6 fd00:dead:beef::/48`).

### Option B: user-defined network

To try it out without messing with your Docker daemon flags, or if you're already using user-defined networks, you can create a IPv6-enabled network with:

```
docker network create --ipv6 --subnet=fd00:dead:beef::/48 mynetwork
```

Then start all of your other containers with `--net=mynetwork`. Please note the `robbertkl/ipv6nat` container still needs to run with `--net=host` to access the host firewall.

Docker-ipv6nat respects all supported `com.docker.network.bridge.*` options (pass them with `-o`) and adds 1 additional option:

* `com.docker.network.bridge.host_binding_ipv6`: Default IPv6 address when binding container ports (do not include subnet/prefixlen; defaults to `::`, i.e. all IPv6 addresses)

Please note this option can only be set on user-defined networks, as the default bridge network is controlled by the Docker daemon.

## Troubleshooting

On some systems, IPv6 filter related kernel modules will not be loaded by default, and you'll see error messages in the log.
Luckily, ip6tables will automatically load all necessary kernel modules for us, and it will even do so from within the container, since we're a privileged container anyway!
To accommodate this, we need to mount the modules so ip6tables can load them: just add `-v /lib/modules:/lib/modules:ro` to the above docker run command.

Also, if you can see the added ip6tables rules, but it's still not working, it might be that forwarding is not enabled for IPv6.
This is usually the case if you're using router advertisements (e.g. having `net.ipv6.conf.eth0.accept_ra=1`).
Enabling forwarding in such a case will break router advertisements. To overcome this, use the following in your `/etc/sysctl.conf`:

```
net.ipv6.conf.eth0.accept_ra = 2
net.ipv6.conf.all.forwarding = 1
net.ipv6.conf.default.forwarding = 1
```

The special value of 2 will allow accepting router advertisements even if forwarding is enabled.

## Authors

* Robbert Klarenbeek, <robbertkl@renbeek.nl>

## License

This repo is published under the [MIT License](http://www.opensource.org/licenses/mit-license.php).
