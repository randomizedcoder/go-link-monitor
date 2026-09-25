{
  pkgs,
  versions,
  vendoredSource,
}:
pkgs.runCommand "check-gosec"
  {
    nativeBuildInputs = [
      versions.go
      versions.gosec
    ];
  }
  ''
    set -euo pipefail
    cp -r ${vendoredSource} src && chmod -R +w src && cd src
    export HOME=$(mktemp -d)
    export CGO_ENABLED=0 GOFLAGS=-mod=vendor GOPROXY=off
    export GOCACHE=$HOME/cache GOPATH=$HOME/go
    # G304: variable file path is the whole point of BaselineStore.
    # G115: int->int32 conversions are kernel ifindexes (always small).
    if ! gosec -exclude=G115,G304 -fmt=text ./... > "$out" 2>&1; then
      cat "$out"
      exit 1
    fi
  ''
