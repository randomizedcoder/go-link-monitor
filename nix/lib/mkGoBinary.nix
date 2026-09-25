# Reusable Go binary derivation. Every package/container build funnels
# through here so build flags stay consistent.
{
  pkgs,
  lib,
  src,
}:
let
  versions = import ../versions.nix { inherit pkgs; };
  buildGoModule = pkgs.buildGoModule.override { inherit (versions) go; };
in
{
  name ? "go-link-monitor",
  subPath ? "cmd/${name}",
  variant ? "default",
  vendorHash ? versions.goVendorHash,
  # Reproducible defaults; a downstream build can override commit/date.
  commit ? "nix",
  date ? "1970-01-01T00:00:00Z",
  version ? (if builtins.pathExists ../../VERSION then lib.fileContents ../../VERSION else "0.0.0"),
  extraLdflags ? [ ],
  doCheck ? false,
}:
let
  variantCfg = versions.buildVariants.${variant};
in
buildGoModule {
  pname = "${name}${variantCfg.tagSuffix}";
  inherit
    version
    src
    vendorHash
    doCheck
    ;

  subPackages = [ subPath ];

  env.CGO_ENABLED = if versions.cgoEnabled then "1" else "0";

  tags = versions.buildTags;

  ldflags =
    variantCfg.extraLdflags
    ++ [
      "-X main.commit=${commit}"
      "-X main.date=${date}"
      "-X main.version=${version}"
    ]
    ++ extraLdflags;

  preBuild = ''
    export GOFLAGS="-trimpath ''${GOFLAGS:-}"
  '';

  # Variant-controlled stripping. `dontStrip` governs nixpkgs' own strip;
  # the postFixup does a hard strip for the "stripped" variant.
  dontStrip = !variantCfg.doStrip;
  postFixup = lib.optionalString variantCfg.doStrip ''
    for bin in $out/bin/*; do
      ${pkgs.binutils-unwrapped}/bin/strip --strip-all "$bin"
    done
  '';

  meta = with lib; {
    description = "go-link-monitor ${name} (${variant}) — rtnetlink NIC-port monitor";
    homepage = "https://github.com/randomizedcoder/go-link-monitor";
    license = licenses.mit;
    platforms = platforms.linux;
    mainProgram = name;
  };
}
