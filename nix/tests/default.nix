# Behavioural test runners. The `test-` prefixed derivations are folded into
# both `packages` and `nix flake check` by the aggregator.
{
  pkgs,
  versions,
  vendoredSource,
}:
{
  test-go-unit = import ./go-unit.nix { inherit pkgs versions vendoredSource; };
  test-go-race = import ./go-test-race.nix { inherit pkgs versions vendoredSource; };
}
