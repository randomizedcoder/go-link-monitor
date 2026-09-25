# Minimal OCI images for the daemon, one per build variant.
{
  pkgs,
  lib,
  binaries,
}:
let
  mkOciImage = import ../lib/mkOciImage.nix { inherit pkgs lib; };
in
{
  oci-go-link-monitor = mkOciImage {
    name = "go-link-monitor";
    tag = "latest";
    binary = binaries.default;
    exposedPorts = [ 9101 ];
    cmd = [
      "-metrics-addr"
      ":9101"
    ];
  };

  oci-go-link-monitor-stripped = mkOciImage {
    name = "go-link-monitor";
    tag = "stripped";
    binary = binaries.stripped;
    exposedPorts = [ 9101 ];
    cmd = [
      "-metrics-addr"
      ":9101"
    ];
  };
}
