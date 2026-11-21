#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
set -eu
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
arch=${1:-mipsle}
case "$arch" in
 mipsle) output=dist/keenetic ;;
 mips) output=dist/keenetic-mips ;;
 *) printf 'Usage: %s [mipsle|mips]\n' "$0" >&2; exit 1 ;;
esac
mkdir -p "$output"
GOOS=linux GOARCH="$arch" GOMIPS=softfloat go build -trimpath -ldflags='-s -w' -o "$output/ukn" ./cmd/ukn
cp packaging/keenetic/install.sh packaging/keenetic/S99unifi-keenetic packaging/keenetic/unifi-keenetic.conf "$output/"
printf 'Built %s (%s softfloat)\n' "$output" "$arch"
