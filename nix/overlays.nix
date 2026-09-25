# overlays.default exposes the binary and OCI image to downstream flakes.
{ self }:
final: _prev: {
  go-link-monitor = self.packages.${final.system}.go-link-monitor or null;
  go-link-monitor-oci = self.packages.${final.system}.oci-go-link-monitor or null;
}
