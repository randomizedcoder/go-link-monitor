{
  pkgs,
  versions,
  vendoredSource,
}:
pkgs.runCommand "check-gofmt"
  {
    nativeBuildInputs = [ versions.go ];
  }
  ''
    set -euo pipefail
    cp -r ${vendoredSource} src && chmod -R +w src && cd src
    bad=$(gofmt -l . | grep -v -E '^vendor/' || true)
    if [ -n "$bad" ]; then
      echo "gofmt: the following files are not formatted:"
      echo "$bad"
      exit 1
    fi
    touch "$out"
  ''
