# RC2 canonical evidence

This package implements the frozen `HTRL` V1 binary formats from
`docs/rc2/CANONICALIZATION.md`. It exposes typed canonical-byte and SHA-256
commitment functions for `AuthorityEventV1`, `PolicyV1`, `DecisionV1`,
`ActionV1`, and `ResponseV1`. It deliberately has no generic JSON or
"hash anything" entry point.

The encoder requires valid NFC UTF-8, exact enum codes, valid pseudonymous
identifiers, explicit nullable markers, and signed big-endian Unix nanoseconds.
Normalization belongs at ingestion; `NormalizeText` is provided for that boundary,
while the canonical encoder rejects a lower-layer non-NFC value.

Run the documented Policy vector and the additional immutable vectors with:

```bash
go test ./internal/evidence/canonical
```

Changing a golden digest is a format change and requires review against the Phase 1
specification. Do not regenerate expected values merely to satisfy a failing test.

