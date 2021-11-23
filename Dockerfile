FROM --platform=$BUILDPLATFORM golang:1.16.6-alpine3.14 AS build
ARG TARGETPLATFORM

# can be passed with any prefix (like `v1.2.3@GITHASH`), e.g.: `docker build --build-arg "APP_VERSION=v1.2.3@GITHASH" .`
ARG APP_VERSION="undefined@docker"

WORKDIR /go/src/github.com/robbertkl/docker-ipv6nat
COPY . .
RUN [ "$TARGETPLATFORM" = "linux/amd64"  ] && echo GOOS=linux GOARCH=amd64 > .env || true
RUN [ "$TARGETPLATFORM" = "linux/arm64"  ] && echo GOOS=linux GOARCH=arm64 > .env || true
RUN [ "$TARGETPLATFORM" = "linux/arm/v6" ] && echo GOOS=linux GOARCH=arm GOARM=6 > .env || true
RUN [ "$TARGETPLATFORM" = "linux/arm/v7" ] && echo GOOS=linux GOARCH=arm GOARM=7 > .env || true
ENV CGO_ENABLED=0
ENV GO111MODULE=auto
RUN env $(cat .env | xargs) go build -o /docker-ipv6nat.$(echo "$TARGETPLATFORM" | sed -E 's/(^linux|\/)//g') ./cmd/docker-ipv6nat

FROM alpine:3.14 AS release
RUN apk add --no-cache ip6tables
COPY --from=build /docker-ipv6nat.* /docker-ipv6nat
COPY docker-ipv6nat-compat /

ENTRYPOINT ["/docker-ipv6nat-compat"]
CMD ["--retry"]

LABEL \
    # Docs: <https://github.com/opencontainers/image-spec/blob/master/annotations.md>
    org.opencontainers.image.title="ipv6nat" \
    org.opencontainers.image.description="Extend Docker with IPv6 NAT, similar to IPv4 " \
    org.opencontainers.image.url="https://github.com/robbertkl/docker-ipv6nat" \
    org.opencontainers.image.source="https://github.com/robbertkl/docker-ipv6nat" \
    org.opencontainers.image.vendor="robbertkl" \
    org.opencontainers.version="$APP_VERSION" \
    org.opencontainers.image.licenses="MIT"
    