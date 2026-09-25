{
  pkgs,
  versions,
  src,
}:
pkgs.runCommand "check-deadnix"
  {
    nativeBuildInputs = [ versions.deadnix ];
  }
  ''
    set -euo pipefail
    cd ${src}
    mapfile -t files < <(find . -type f -name '*.nix' -not -path './vendor/*' -not -path './.git/*')
    # Mark intentionally-unused bindings/args with a leading underscore.
    deadnix --fail "''${files[@]}"
    touch "$out"
  ''
