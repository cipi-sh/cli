BINARY_NAME=cipi-cli
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-s -w -X github.com/cipi-sh/cli/cmd.Version=$(VERSION) -X github.com/cipi-sh/cli/cmd.BuildTime=$(BUILD_TIME)"

.PHONY: build install clean test release

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

install: build
	mv $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

test:
	go test ./...

release: clean
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	cd dist && shasum -a 256 * > checksums.txt
