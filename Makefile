BINARY := wktree
CMD := ./cmd/wktree
DIST_DIR := dist
BUILD_PATH := $(DIST_DIR)/$(BINARY)
INSTALL_DIR ?= $(HOME)/.local/bin
GO ?= go
GOFMT ?= gofmt

.PHONY: all build install uninstall lint test race check clean help

all: build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	mkdir -p "$(DIST_DIR)"
	$(GO) build -trimpath -ldflags="-X github.com/alienxp03/wktree/internal/cli.Version=$(VERSION)" -o "$(BUILD_PATH)" "$(CMD)"

install: build
	mkdir -p "$(INSTALL_DIR)"
	install -m 0755 "$(BUILD_PATH)" "$(INSTALL_DIR)/$(BINARY)"
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

uninstall:
	rm -f "$(INSTALL_DIR)/$(BINARY)"
	@echo "Removed $(INSTALL_DIR)/$(BINARY)"

reset: install
	rm .wktree.yaml
	wktree init

test:
	$(GO) test ./...

lint:
	@unformatted="$$( $(GOFMT) -l $$(git ls-files '*.go') )"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

race:
	$(GO) test -race ./...

check: lint test race build

clean:
	rm -rf "$(DIST_DIR)"
	rm -f "./$(BINARY)"

help:
	@echo "Targets:"
	@echo "  make build                         Build $(BUILD_PATH)"
	@echo "  make install                       Install to $(INSTALL_DIR)/$(BINARY)"
	@echo "  make install INSTALL_DIR=/path/bin Install to a custom user bin path"
	@echo "  make uninstall                     Remove $(INSTALL_DIR)/$(BINARY)"
	@echo "  make lint                          Check gofmt formatting"
	@echo "  make test                          Run go test ./..."
	@echo "  make race                          Run go test -race ./..."
	@echo "  make check                         Run lint, test, race, and build"
	@echo "  make clean                         Remove build output"
