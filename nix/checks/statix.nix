{
  pkgs,
  versions,
  src,
}:
pkgs.runCommand "check-statix"
  {
    nativeBuildInputs = [ versions.statix ];
  }
  ''
    set -euo pipefail
    cp -r ${src} s && chmod -R +w s && cd s
    statix check -i vendor -i .git -o errfmt .
    touch "$out"
  ''
