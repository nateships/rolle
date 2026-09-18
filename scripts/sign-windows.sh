#!/bin/sh
# goreleaser post-build hook. Signs a Windows binary with the Certum SimplySign
# certificate through ssign. Other targets pass through. A run without the
# Certum secrets ships the binary unsigned and says so.
#
# Usage: sign-windows.sh <target> <path>
#   target  the goreleaser build target, such as windows_amd64
#   path    the binary goreleaser built
set -eu

target="$1"
path="$2"

case "$target" in
  windows_*) ;;
  *) exit 0 ;;
esac

if [ -z "${CERTUM_OTP:-}" ]; then
  echo "CERTUM_OTP is not set; $path ships unsigned."
  exit 0
fi

ssign "$path"
