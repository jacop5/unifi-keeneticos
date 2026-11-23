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
cp packaging/keenetic/install.sh packaging/keenetic/space-check.sh packaging/keenetic/S99unifi-keenetic packaging/keenetic/unifi-keenetic.conf "$output/"
cp LICENSE NOTICE "$output/"
mkdir -p "$output/licenses"
cp LICENSES/Go-BSD-3-Clause.txt "$output/licenses/"
cp internal/models/NOTICE "$output/licenses/model-catalogue-MIT.txt"
# Ship rebuildable corresponding source with the binary, not just a URL or
# a promise of a future source offer. Exclude runtime/controller secrets.
# Rebuild source in a fresh directory: merging into an earlier bundle can
# retain files deleted from the current tree, including obsolete dependencies.
source_stage=$(mktemp -d "$output/.source.XXXXXX")
trap 'rm -rf "$source_stage"' 0
trap 'exit 1' HUP INT TERM
cp -R cmd internal tools packaging docs LICENSES "$source_stage/"
cp go.mod go.sum LICENSE NOTICE README.md "$source_stage/"
go mod vendor -o "$source_stage/vendor"
rm -rf "$output/source"
mv "$source_stage" "$output/source"
# Runtime package integrity (not a signature or source-tree manifest).
(cd "$output" && sha256sum ukn install.sh space-check.sh S99unifi-keenetic unifi-keenetic.conf LICENSE NOTICE licenses/Go-BSD-3-Clause.txt licenses/model-catalogue-MIT.txt > SHA256SUMS)
printf 'Built %s (%s softfloat)\n' "$output" "$arch"
