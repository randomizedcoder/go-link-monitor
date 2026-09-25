{
  pkgs,
  versions,
  src,
}:
pkgs.runCommand "check-nixfmt"
  {
    nativeBuildInputs = [ versions.nixfmt ];
  }
  ''
    set -euo pipefail
    cd ${src}
    fail=0
    while IFS= read -r f; do
      if ! nixfmt --check "$f" 2>/dev/null; then
        echo "nixfmt: needs formatting: $f"
        fail=1
      fi
    done < <(find . -type f -name '*.nix' -not -path './vendor/*' -not -path './.git/*')
    [ "$fail" -eq 0 ] || exit 1
    touch "$out"
  ''
