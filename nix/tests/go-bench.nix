# Benchmarks as an on-demand app (impure: runs against the working tree).
{ pkgs, versions }:
pkgs.writeShellApplication {
  name = "bench";
  runtimeInputs = [ versions.go ];
  text = ''
    export CGO_ENABLED=0
    exec go test -run=NONE -bench=. -benchmem ./pkg/linkmonitor/
  '';
}
