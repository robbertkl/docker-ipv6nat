FROM alpine:latest
MAINTAINER Robbert Klarenbeek <robbertkl@renbeek.nl>

RUN apk add --no-cache \
        ip6tables

ENV DOCKER_IPV6NAT_VERSION v0.1.1
ADD https://github.com/robbertkl/docker-ipv6nat/releases/download/${DOCKER_IPV6NAT_VERSION}/docker-ipv6nat /docker-ipv6nat
RUN chmod u+x /docker-ipv6nat

ENTRYPOINT ["/docker-ipv6nat"]
CMD ["--retry"]
