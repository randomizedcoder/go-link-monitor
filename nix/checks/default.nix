# The `nix flake check` static-analysis surface.
{
  pkgs,
  versions,
  src,
  vendoredSource,
  binary,
}:
{
  # Go static analysis (hermetic, against the vendored tree).
  go-vet = import ./go-vet.nix { inherit pkgs versions vendoredSource; };
  gofmt = import ./gofmt.nix { inherit pkgs versions vendoredSource; };
  golangci-lint = import ./golangci-lint.nix { inherit pkgs versions vendoredSource; };
  gosec = import ./go-sec.nix { inherit pkgs versions vendoredSource; };

  # Nix hygiene.
  nix-fmt = import ./nix-fmt.nix { inherit pkgs versions src; };
  deadnix = import ./deadnix.nix { inherit pkgs versions src; };
  statix = import ./statix.nix { inherit pkgs versions src; };

  # Behavioural smoke of the built binary.
  cli-help-smoke = import ./cli-help-smoke.nix { inherit pkgs binary; };
}
