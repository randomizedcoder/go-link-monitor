{
  pkgs,
  versions,
  vendoredSource,
}:
pkgs.runCommand "check-go-vet"
  {
    nativeBuildInputs = [ versions.go ];
  }
  ''
    set -euo pipefail
    cp -r ${vendoredSource} src && chmod -R +w src && cd src
    export HOME=$(mktemp -d)
    export CGO_ENABLED=0 GOFLAGS=-mod=vendor GOPROXY=off
    export GOCACHE=$HOME/cache GOPATH=$HOME/go
    if ! go vet ./... > "$out" 2>&1; then
      cat "$out"
      exit 1
    fi
  ''
