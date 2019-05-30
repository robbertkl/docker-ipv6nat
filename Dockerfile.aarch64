FROM arm64v8/alpine:3.9
MAINTAINER Robbert Klarenbeek <robbertkl@renbeek.nl>

ENV QEMU_EXECVE 1
COPY qemu-aarch64-static /usr/bin/

RUN ["qemu-aarch64-static", "-0", "/bin/sh", "/bin/sh", "-c", "apk add --no-cache --repository=http://dl-cdn.alpinelinux.org/alpine/edge/main ip6tables"]

ENV DOCKER_IPV6NAT_VERSION v0.3.7
ADD https://github.com/robbertkl/docker-ipv6nat/releases/download/${DOCKER_IPV6NAT_VERSION}/docker-ipv6nat.aarch64 /docker-ipv6nat
RUN ["qemu-aarch64-static", "-0", "/bin/sh", "/bin/sh", "-c", "chmod u+x /docker-ipv6nat"]

COPY docker-ipv6nat-compat /
ENTRYPOINT ["/docker-ipv6nat-compat"]
CMD ["--retry"]
