# Smoke test: the built binary must print usage and exit cleanly for -help
# and -version, without needing netlink privileges.
{ pkgs, binary }:
pkgs.runCommand "check-cli-help-smoke" { } ''
  set -uo pipefail

  help_txt=$(${binary}/bin/go-link-monitor -help 2>&1)
  help_code=$?
  echo "$help_txt"
  if [ -z "$help_txt" ]; then
    echo "no -help output"; exit 1
  fi
  # flag prints usage and exits 2 for -help; anything higher is a crash.
  if [ "$help_code" -gt 2 ]; then
    echo "-help exited $help_code"; exit 1
  fi

  ver_txt=$(${binary}/bin/go-link-monitor -version 2>&1)
  ver_code=$?
  echo "$ver_txt"
  if [ "$ver_code" -ne 0 ]; then
    echo "-version exited $ver_code"; exit 1
  fi
  case "$ver_txt" in
    go-link-monitor*) ;;
    *) echo "unexpected -version output"; exit 1 ;;
  esac

  touch "$out"
''
