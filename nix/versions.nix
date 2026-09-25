# Single source of truth for tool versions and build axes.
{ pkgs }:
rec {
  # Go toolchain pinned to 1.27.x (nixpkgs default `go` lags a release).
  # Must satisfy the `go` directive in go.mod.
  go = pkgs.go_1_27;

  # Static-analysis / lint tooling and Nix hygiene tools.
  inherit (pkgs)
    golangci-lint
    gosec
    govulncheck
    deadnix
    statix
    ;

  # nixfmt's package name differs from the attr we expose.
  nixfmt = pkgs.nixfmt-rfc-style;

  # Build configuration. Pure Go, so CGO is off; netgo/osusergo keep the
  # binary fully static.
  cgoEnabled = false;
  buildTags = [
    "netgo"
    "osusergo"
  ];

  # go module vendor hash. Bootstrap/refresh with:
  #   nix build .#go-link-monitor 2>&1 | grep 'got:.*sha256-' | head -1
  # then paste the value here.
  goVendorHash = "sha256-52P+e+Rg3ZfKqOMp7u4rL+wSnLrAvNy5D+4grtaSboo=";

  # Build variants: unstripped debug, default (-s -w), and hard-stripped.
  buildVariants = {
    debug = {
      extraLdflags = [ ];
      doStrip = false;
      tagSuffix = "-debug";
    };
    default = {
      extraLdflags = [
        "-s"
        "-w"
      ];
      doStrip = false;
      tagSuffix = "";
    };
    stripped = {
      extraLdflags = [
        "-s"
        "-w"
      ];
      doStrip = true;
      tagSuffix = "-stripped";
    };
  };
}
