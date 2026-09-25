# Package lists for the dev shell.
{ pkgs, versions }:
rec {
  nativeBuildInputs = [
    versions.go
    pkgs.git
    pkgs.cacert
  ];

  buildInputs = [ ]; # pure Go, no C deps

  devTools = [
    # Go development
    versions.go
    pkgs.gopls
    pkgs.gotools # goimports, etc.
    pkgs.delve # dlv debugger
    pkgs.go-tools # staticcheck

    # Static analysis
    versions.golangci-lint
    versions.gosec
    versions.govulncheck

    # Nix hygiene
    versions.nixfmt
    versions.deadnix
    versions.statix

    # Containers + runtime poking
    pkgs.skopeo # inspect/copy OCI images
    pkgs.iproute2 # `ip link` to exercise the monitor
  ];

  allDevPackages = nativeBuildInputs ++ buildInputs ++ devTools;
}
