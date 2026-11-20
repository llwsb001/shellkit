VERSION ?= dev
BIN     := bin
# go.mod asks for 1.27.4; point make at that toolchain so a stray system Go
# does not silently build something older.
# Prefer the local 1.27.4 toolchain when it exists, fall back to PATH so the
# Makefile works on any machine. `make GO=go` forces the PATH one.
GO      ?= $(shell if [ -x /home/user/tools/go1.27.4/bin/go ]; then echo /home/user/tools/go1.27.4/bin/go; else echo go; fi)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all cli web tui nogui desktop sizes test fmt vet demo shot clean

all: sizes

# The smallest shell: no page — what the container runs.
cli:
	$(GO) build -tags nogui -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/shellkit-cli .

nogui: cli

# The default shell: command tree + gateway + the embedded page.
web:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/shellkit .

# Terminal panel on top of the same core.
tui:
	$(GO) build -tags tui -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/shellkit-tui .

# Native window. Needs GTK3 + WebKitGTK 4.1 headers; when this host has none,
# the same command runs in CI (.github/workflows/desktop.yml).
desktop:
	$(GO) build -tags desktop -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/shellkit-desktop .

sizes: web tui cli
	@echo "shell                       bytes"
	@echo "--------------------------------"
	@for f in $(BIN)/shellkit $(BIN)/shellkit-tui $(BIN)/shellkit-cli; do \
	  printf "%-26s %s\n" $$(basename $$f) $$(wc -c < $$f); done
	@echo
	@echo "same core; the difference is build tags, not forks."

test:
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	$(GO) vet ./...
	$(GO) test ./...
	$(GO) test -race -count=1 ./...
	$(GO) test -tags nogui ./internal/ui
	$(GO) build -tags tui ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

demo: web
	./demo.sh

# Screenshot of the page, with whatever playwright is on the box.
shot: web
	python3 scripts/shot.py

clean:
	rm -rf $(BIN)
