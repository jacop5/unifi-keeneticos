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
cp LICENSE NOTICE "$output/"
mkdir -p "$output/licenses"
cp LICENSES/Go-BSD-3-Clause.txt "$output/licenses/"
cp internal/models/NOTICE "$output/licenses/model-catalogue-MIT.txt"
# Ship rebuildable corresponding source with the binary, not just a URL or
# a promise of a future source offer. Exclude runtime/controller secrets.
mkdir -p "$output/source"
cp -R cmd internal tools packaging docs LICENSES "$output/source/"
cp go.mod go.sum LICENSE NOTICE README.md "$output/source/"
go mod vendor -o "$output/source/vendor"
printf 'Built %s (%s softfloat)\n' "$output" "$arch"
