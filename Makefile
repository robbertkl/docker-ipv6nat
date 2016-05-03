NAME := docker-ipv6nat
TAG := `git describe --tags`
LDFLAGS := -X main.buildVersion=$(TAG)

.SILENT:

all:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" "./cmd/$(NAME)"
