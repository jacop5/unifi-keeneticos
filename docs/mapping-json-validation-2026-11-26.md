# Strict mapping JSON validation — 2026-11-26

The `-slots`, `-networks` and `-experimental-ft-domains` loaders now share a
strict JSON object parser. Duplicate decoded keys (including escaped equivalents)
are rejected instead of silently selecting the last value. Top-level `null`,
non-object inputs, null/non-string/nested values, malformed JSON and trailing
data are also rejected. Valid string mappings and empty objects retain their
planner semantics. Parse diagnostics identify the flag without exposing mapping
keys or values; file-read errors remain available.

All four load sites use the helper, including offline and runtime slot loading.
Initialization order and the separate `-plan-blocked` array format are unchanged.

Helper tests cover types, escapes, duplicates, malformed/trailing input, redaction
and file-read errors. Real CLI offline planning tests exercise all three flags
with duplicate-key failures and valid controls, with a native-command sentinel.
An initial test harness positional-argument error was corrected before acceptance.
Independent adversarial source review and targeted tests accepted the change;
the lead independently passed package race tests. The first lead race attempt
was blocked by sandbox loopback binding, then passed with local-server permission.

Clean candidate `6386b11920af0b86544aa41797c259e3b48e6b99` passed the full offline
release gate: uncached race, vet, Python tooling, MIPS little/big-endian packages,
manifest checks and vendored-source offline rebuilds. Local machine-readable
report: `dist/validation/offline-release.json` (ignored build output).

No router/controller access or deployment occurred. Runtime slot loading was
reviewed through the shared helper; it was not tested end-to-end on hardware.
