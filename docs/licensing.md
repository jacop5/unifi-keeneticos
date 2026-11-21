# License and distribution review

Review date: 2026-11-21. Scope: the Keenetic fork's source changes, Go
dependencies and generated Keenetic installation bundles. This records
repository checks and distribution requirements, not a legal certification.

## Inherited license and attribution

The starting point is [antoniocosta/unifi-openwrt at a211831](https://github.com/antoniocosta/unifi-openwrt/commit/a211831).
Its notices identify the project as **GPL-2.0-or-later**. The fork retains
that license; the root LICENSE is the upstream GPLv2 text, unmodified.
The original NOTICE is retained verbatim after the fork-specific preamble,
and internal/models/NOTICE retains the MIT text and James Braid's copyright
for the derived catalogue. Existing SPDX and attribution notices remain.

The README identifies the project as a modified fork, credits upstream, and
separates Keenetic verification from inherited OpenWrt features. Modified
upstream files carry dated change notices: README.md, NOTICE, .gitignore,
internal/device/session.go and internal/device/session_test.go. New Go/shell
code carries GPL-2.0-or-later SPDX identifiers; the test Mongo initialization
script does too. JSON/YAML examples are part of the GPL-licensed project,
without adding syntax-invalid comment headers to JSON.

These checks address retaining notices and marking modifications under
[GPLv2 sections 1 and 2](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html).
Upstream's permission to use later GPL versions comes from its project/file
notices, rather than an alteration to the GPL license document itself.

## Linked dependencies

The reviewed module versions are golang.org/x/crypto v0.59.0 and
golang.org/x/sys v0.50.0. Their top-level LICENSE files are the same BSD
3-Clause text, Copyright 2009 The Go Authors. The installed Go toolchain's
top-level license has the same text. A verbatim copy is retained as
LICENSES/Go-BSD-3-Clause.txt; nested source copyright notices are retained
when Go vendors the dependencies for a bundle.

The BSD notice allows redistribution subject to retaining its notice,
conditions and disclaimer, including with binary distributions. The GNU
Project lists the [modified BSD license as GPL-compatible](https://www.gnu.org/licenses/license-list.html#ModifiedBSD).
The catalogue's separate MIT notice is also included. No third-party
copyright has been reassigned to the fork's maintainers.

## Binary distribution

tools/build-keenetic.sh now places these alongside each binary:

- LICENSE and NOTICE.
- licenses/Go-BSD-3-Clause.txt and licenses/model-catalogue-MIT.txt.
- source/: a snapshot of the compilation/installation source, go.mod/go.sum,
  documentation and vendored Go module dependencies, including their notices.

The installer checks the required license files and copies them into
/opt/share/licenses/unifi-keeneticos. It does not copy the source snapshot
onto the router; recipients keep it in the delivered bundle.

Distribute the **whole bundle**, including source/, when sharing a compiled
binary. A bare executable or an archive omitting source/ is not the release
format reviewed here. GPLv2 section 3 provides other distribution options,
but this project uses accompanying corresponding source rather than a
three-year written offer. When publishing releases, keep matching source
and dependency notices available with the exact binary, and rebuild the
bundle after source changes. Do not substitute a link to a moving branch.
The compiler/standard-library system-component exception is described in
GPLv2 section 3; the external Go modules are included explicitly.

Build output and private controller .env files are ignored by Git. The
repository publishes source, not the local compiled test executables or
controller credentials. The corresponding-source packaging does not copy
testing/controller/.env, runtime state, SSH keys, or the .git directory.

## Separate software and remaining release checks

KeeneticOS, Entware, UniFi OS Server, the LinuxServer legacy controller image
and MongoDB are separate software. This repository does not relicense or
redistribute their binaries. The optional test-environment scripts download
or reference them separately, under their respective terms. Their names and
trademarks do not imply affiliation or endorsement.

Recheck dependency versions/notices and inspect generated archives whenever
dependencies, toolchains or packaging change. This review does not cover
third-party container-image contents, firmware redistribution, trademark
clearance, or a jurisdiction-specific legal opinion.
