FROM alpine:3.9
MAINTAINER Robbert Klarenbeek <robbertkl@renbeek.nl>

RUN apk add --no-cache --repository=http://dl-cdn.alpinelinux.org/alpine/edge/main ip6tables

ENV DOCKER_IPV6NAT_VERSION v0.3.7
ADD https://github.com/robbertkl/docker-ipv6nat/releases/download/${DOCKER_IPV6NAT_VERSION}/docker-ipv6nat.amd64 /docker-ipv6nat
RUN chmod u+x /docker-ipv6nat

COPY docker-ipv6nat-compat /
ENTRYPOINT ["/docker-ipv6nat-compat"]
CMD ["--retry"]
