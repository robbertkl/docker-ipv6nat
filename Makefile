NAME := docker-ipv6nat
PKG := github.com/robbertkl/$(NAME)
DIR := /go/src/$(PKG)
GO := 1.7.5-alpine3.5
TAG := `git describe --tags`
LDFLAGS := -X main.buildVersion=$(TAG)

.SILENT:

all:
	docker run --rm \
		-v "$(PWD)":"$(DIR)" \
		-w "$(DIR)" \
		-e GOOS=linux \
		-e GOARCH=amd64 \
		-e CGO_ENABLED=0 \
		golang:"$(GO)" \
		go build -ldflags "$(LDFLAGS)" "./cmd/$(NAME)"
