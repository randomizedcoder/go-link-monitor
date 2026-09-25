# Per-system aggregator: versions -> binaries -> vendored source ->
# containers / checks / tests / devshell, assembled into the flake outputs.
{
  pkgs,
  lib,
  src,
}:
let
  versions = import ./versions.nix { inherit pkgs; };

  goMods = import ./lib/goModules.nix { inherit pkgs src; };
  inherit (goMods) vendoredSource;

  mkGoBinary = import ./lib/mkGoBinary.nix { inherit pkgs lib src; };

  binaries = {
    default = mkGoBinary { variant = "default"; };
    debug = mkGoBinary { variant = "debug"; };
    stripped = mkGoBinary { variant = "stripped"; };
  };

  containers = import ./containers/default.nix { inherit pkgs lib binaries; };

  checks = import ./checks/default.nix {
    inherit
      pkgs
      versions
      src
      vendoredSource
      ;
    binary = binaries.default;
  };

  tests = import ./tests/default.nix { inherit pkgs versions vendoredSource; };

  devshell = import ./devshell.nix { inherit pkgs versions; };

  benchApp = import ./tests/go-bench.nix { inherit pkgs versions; };

  # govulncheck needs the online vuln DB, so it is an app rather than a
  # (hermetic) flake check.
  govulncheckApp = pkgs.writeShellApplication {
    name = "govulncheck-run";
    runtimeInputs = [
      versions.go
      versions.govulncheck
    ];
    text = ''
      export CGO_ENABLED=0
      exec govulncheck ./...
    '';
  };

  # Both `nix fmt` targets in one script: Go then Nix.
  fmtApp = pkgs.writeShellApplication {
    name = "fmt";
    runtimeInputs = [
      versions.go
      versions.nixfmt
      pkgs.findutils
    ];
    text = ''
      gofmt -w .
      find . -type f -name '*.nix' -not -path './vendor/*' -exec nixfmt {} +
    '';
  };
in
{
  packages = {
    inherit (binaries) default;
    go-link-monitor = binaries.default;
    go-link-monitor-debug = binaries.debug;
    go-link-monitor-stripped = binaries.stripped;
    bench = benchApp;
  }
  // (lib.filterAttrs (n: _v: lib.hasPrefix "oci-" n) containers)
  // tests;

  devShells.default = devshell;

  # Static analysis + the behavioural test runners.
  checks = checks // tests;

  apps = {
    bench = {
      type = "app";
      program = "${benchApp}/bin/bench";
      meta.description = "Run go-link-monitor benchmarks (go test -bench).";
    };
    govulncheck = {
      type = "app";
      program = "${govulncheckApp}/bin/govulncheck-run";
      meta.description = "Run govulncheck against the module (needs network).";
    };
  };

  formatter = fmtApp;
}
