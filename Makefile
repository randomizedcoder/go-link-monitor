GO      ?= go
PKGS    := ./pkg/... ./cmd/...
BINDIR  := bin
BIN     := $(BINDIR)/go-link-monitor

.PHONY: all build test race cover bench fuzz vet fmt tidy clean \
        nix-build nix-check nix-oci nix-shell

all: fmt vet test build

build:
	$(GO) build -o $(BIN) ./cmd/go-link-monitor

test:
	$(GO) test $(PKGS)

race:
	$(GO) test -race $(PKGS)

cover:
	$(GO) test -cover -coverprofile=coverage.out $(PKGS)
	$(GO) tool cover -func=coverage.out | tail -1

bench:
	$(GO) test -run=NONE -bench=. -benchmem ./pkg/linkmonitor/

# Usage: make fuzz FUZZ=FuzzParseBaselineNames [FUZZTIME=30s]
FUZZ     ?= FuzzParseBaselineNames
FUZZTIME ?= 30s
fuzz:
	$(GO) test -run=NONE -fuzz=$(FUZZ) -fuzztime=$(FUZZTIME) ./pkg/linkmonitor/

vet:
	$(GO) vet $(PKGS)

fmt:
	$(GO) fmt $(PKGS)

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BINDIR) coverage.out result result-*

# --- Nix ---

nix-build:
	nix build .#go-link-monitor

# Build the streamed OCI image and load it into docker.
nix-oci:
	nix build .#oci-go-link-monitor
	./result | docker load

nix-check:
	nix flake check -L

nix-shell:
	nix develop
