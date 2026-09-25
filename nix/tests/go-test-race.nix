# Race detector needs cgo, so this build pulls in a C toolchain.
{
  pkgs,
  versions,
  vendoredSource,
}:
pkgs.runCommand "test-go-race"
  {
    nativeBuildInputs = [
      versions.go
      pkgs.gcc
    ];
  }
  ''
    set -euo pipefail
    cp -r ${vendoredSource} src && chmod -R +w src && cd src
    export HOME=$(mktemp -d)
    export CGO_ENABLED=1 GOFLAGS=-mod=vendor GOPROXY=off
    export GOCACHE=$HOME/cache GOPATH=$HOME/go
    if ! go test -race -count=1 -timeout 5m ./... > "$out" 2>&1; then
      cat "$out"
      exit 1
    fi
  ''
