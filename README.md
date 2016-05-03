# docker-ipv6nat

...

## Usage

The easiest way is to just run the Docker image:

```
docker run -d --restart=always -v /var/run/docker.sock:/var/run/docker.sock:ro --privileged --net=host robbertkl/ipv6nat
```

The flags `--privileged` and `--net=host` are necessary because docker-ipv6nat manages the hosts IPv6 firewall using ip6tables.

Alternatively, you can download the latest release from the [release page](https://github.com/robbertkl/docker-ipv6nat/releases) and run it on your host.
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

Then start your containers with `--net=mynetwork`.

Docker-ipv6nat respects all supported `com.docker.network.bridge.` options (pass them with `-o`) and adds 1 additional option:

* `com.docker.network.bridge.host_binding_ipv6`: Default IPv6 address when binding container ports (defaults to `::/0`)

## Authors

* Robbert Klarenbeek, <robbertkl@renbeek.nl>

## License

This repo is published under the [MIT License](http://www.opensource.org/licenses/mit-license.php).
