BINARY := voice-memo-capture
PKG := ./cmd/voice-memo-capture
BIN_DIR := $(HOME)/.local/bin

.PHONY: build test vet fmt install uninstall

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

install: build
	./install.sh

uninstall:
	./uninstall.sh
