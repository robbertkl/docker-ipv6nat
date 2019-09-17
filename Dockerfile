FROM --platform=$BUILDPLATFORM golang:1.12.7-alpine3.10 AS build
ARG TARGETPLATFORM
ARG VERSION
WORKDIR /go/src/github.com/robbertkl/docker-ipv6nat
COPY . .
RUN [ "$TARGETPLATFORM" = "linux/amd64"  ] && echo GOOS=linux GOARCH=amd64 > .env || true
RUN [ "$TARGETPLATFORM" = "linux/arm64"  ] && echo GOOS=linux GOARCH=arm64 > .env || true
RUN [ "$TARGETPLATFORM" = "linux/arm/v6" ] && echo GOOS=linux GOARCH=arm GOARM=6 > .env || true
RUN [ "$TARGETPLATFORM" = "linux/arm/v7" ] && echo GOOS=linux GOARCH=arm GOARM=7 > .env || true
ENV CGO_ENABLED=0
RUN env $(cat .env | xargs) go build -o /docker-ipv6nat -ldflags "-X main.buildVersion=$VERSION" ./cmd/docker-ipv6nat

FROM alpine:3.10 AS release
RUN apk add --no-cache --repository=http://dl-cdn.alpinelinux.org/alpine/edge/main ip6tables
COPY --from=build /docker-ipv6nat /
COPY docker-ipv6nat-compat /
ENTRYPOINT ["/docker-ipv6nat-compat"]
CMD ["--retry"]
