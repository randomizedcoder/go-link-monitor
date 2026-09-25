# Produces the vendored dependency tree plus a writable source copy with
# vendor/ populated, reused by every hermetic check so no network is needed
# in the sandbox.
{
  pkgs,
  src,
}:
let
  versions = import ../versions.nix { inherit pkgs; };
  vendorHash = versions.goVendorHash;

  parent = (pkgs.buildGoModule.override { inherit (versions) go; }) {
    pname = "go-link-monitor";
    version = "vendored";
    inherit src vendorHash;
    env.CGO_ENABLED = "0";
    doCheck = false;
  };
in
{
  inherit (parent) goModules;

  vendoredSource = pkgs.runCommand "go-link-monitor-vendored-source" { } ''
    cp -r ${src}/. $out
    chmod -R +w $out
    cp -r ${parent.goModules} $out/vendor
  '';
}
