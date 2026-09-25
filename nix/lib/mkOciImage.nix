# Reusable minimal OCI image builder. Uses streamLayeredImage, so
#   nix build .#oci-go-link-monitor && ./result | docker load
# produces the image without a large intermediate tarball. Base is scratch
# plus CA certificates only.
{ pkgs, lib }:
{
  name,
  tag ? "latest",
  binary,
  entrypoint ? "/bin/go-link-monitor",
  exposedPorts ? [ ],
  cmd ? [ ],
  env ? [ ],
}:
let
  exposedPortsAttr = lib.listToAttrs (
    map (p: {
      name = "${toString p}/tcp";
      value = { };
    }) exposedPorts
  );
in
pkgs.dockerTools.streamLayeredImage {
  inherit name tag;

  contents = [
    binary
    pkgs.dockerTools.caCertificates # roots for Go crypto/x509 (HTTPS)
  ];

  config = {
    Entrypoint = [ entrypoint ];
    ExposedPorts = exposedPortsAttr;
    Env = [ "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt" ] ++ env;
  }
  // lib.optionalAttrs (cmd != [ ]) { Cmd = cmd; };
}
