BINARY_NAME=confluence-cli
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

.PHONY: build install clean test release-build

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

install:
	go install .

install-local: build
	mkdir -p ~/bin
	cp $(BINARY_NAME) ~/bin/
	@echo "Make sure ~/bin is in your PATH"

clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

test:
	go test ./...

# Cross-compile for multiple platforms
release-build: clean
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-arm64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe .
