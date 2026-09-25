{
  pkgs,
  versions,
  vendoredSource,
}:
pkgs.runCommand "check-golangci-lint"
  {
    nativeBuildInputs = [
      versions.go
      versions.golangci-lint
    ];
  }
  ''
    set -euo pipefail
    cp -r ${vendoredSource} src && chmod -R +w src && cd src
    export HOME=$(mktemp -d)
    export CGO_ENABLED=0 GOFLAGS=-mod=vendor GOPROXY=off
    export GOCACHE=$HOME/cache GOPATH=$HOME/go GOMODCACHE=$HOME/gomod
    export GOLANGCI_LINT_CACHE=$HOME/golangci
    # Timeout lives in .golangci.yml (run.timeout); a CLI flag would override it.
    if ! golangci-lint run --config .golangci.yml ./... > "$out" 2>&1; then
      cat "$out"
      exit 1
    fi
  ''
