# RC2 canonical commitment formats

## Purpose

RC2 hashes explicit, versioned binary representations. It never hashes arbitrary
JSON, database rows, Go structs, or Solidity ABI output. PostgreSQL stores the
canonicalization version and resulting SHA-256 digest with each immutable version.
Contracts may store backend-computed commitments; Solidity need not recompute them.

## Common envelope

Every canonical object begins with:

```text
magic       4 bytes   ASCII "HTRL" (48 54 52 4c)
version     2 bytes   unsigned big-endian; 0001 for V1
objectType  1 byte    01 event, 02 policy, 03 decision, 04 action, 05 response
```

This envelope is the canonicalization version and domain separator. A future format
uses a new version and never silently changes V1.

## Primitive encodings

| Type | Encoding |
|---|---|
| Unsigned integer | Fixed width, big-endian |
| Timestamp | Signed 64-bit big-endian Unix nanoseconds, UTC |
| Enum | Unsigned 16-bit big-endian code from the tables below |
| Identifier | `u32be(byteLength) || ASCII bytes` |
| Text | `u32be(byteLength) || UTF-8 bytes` |
| Hash | Exactly 32 raw bytes |
| Nullable T | `00` for null; `01 || encode(T)` when present |

Input text must be valid UTF-8 and Unicode NFC. Non-NFC input is normalized before
persistence and hashing at the ingestion boundary. The canonical encoder rejects
invalid UTF-8 and any non-NFC value received from a lower layer. No trimming, case
folding, newline conversion, or locale conversion occurs inside the encoder.

Identifiers are synthetic/pseudonymous ASCII matching `[A-Z0-9][A-Z0-9._-]{0,127}`.
They never contain real provider identifiers or staff names. Empty required strings,
unknown enum values, invalid timestamps, invalid UTF-8, and wrong-size hashes are
rejected rather than encoded.

SHA-256 is applied to the complete byte sequence. Internally and on-chain the result
is 32 raw bytes/`bytes32`; APIs and UI render lowercase hexadecimal as `0x` followed
by exactly 64 digits.

## Enum codes

```text
EventType:       EXCLUSION=0001
AssertionKind:   ORIGINAL=0001, CORRECTION=0002,
                 SUPERSESSION=0003, REINSTATEMENT=0004
AuthorityEffect: EXCLUSION_ACTIVE=0001, EXCLUSION_LIFTED=0002
ResponseState:   RECEIVED=0001, UNDER_REVIEW=0002,
                 DECIDED=0003, ACTION_COMPLETED=0004
```

Decision and action codes are organization-local controlled strings, not shared
semantic judgments. V1 commits their exact UTF-8 value but does not ask the contract
to interpret them.

## `AuthorityEventV1`

Object type `01`; fields follow in exactly this order:

1. `eventId` — Identifier.
2. `eventSeriesId` — Identifier.
3. `previousEventId` — nullable Identifier.
4. `targetEventId` — nullable Identifier.
5. `authorityId` — Identifier.
6. `providerReference` — synthetic/pseudonymous Identifier.
7. `eventType` — EventType enum.
8. `assertionKind` — AssertionKind enum.
9. `authorityEffect` — AuthorityEffect enum.
10. `effectiveTime` — Timestamp.
11. `sourceDocumentHash` — Hash of the canonical off-chain source document bytes.

The provider name, real-world identifier, source document, ingestion timestamp,
staff identity, and database metadata are excluded. Only the resulting event
commitment and safe fields needed for shared lineage may enter calldata or events.

## `PolicyV1`

Object type `02`; fields follow in exactly this order:

1. `policyId` — Identifier.
2. `organizationId` — Identifier.
3. `policyVersion` — Identifier.
4. `effectiveFrom` — Timestamp.
5. `effectiveUntil` — nullable Timestamp.
6. `policyText` — Text.

The full canonical preimage remains off-chain. Only its commitment is used in a
response transaction.

## `DecisionV1`

Object type `03`; fields follow in exactly this order:

1. `decisionId` — Identifier.
2. `eventId` — Identifier.
3. `organizationId` — Identifier.
4. `policyVersionHash` — Hash.
5. `decisionCode` — Text.
6. `decisionExplanation` — Text.
7. `decidedAt` — Timestamp.
8. `reviewerReference` — Identifier local to the application.

Decision explanation and reviewer reference are in the off-chain preimage but never
sent individually in transaction calldata or emitted events.

## `ActionV1`

Object type `04`; fields follow in exactly this order:

1. `actionId` — Identifier.
2. `eventId` — Identifier.
3. `organizationId` — Identifier.
4. `decisionHash` — Hash.
5. `actionCode` — Text.
6. `actionExplanation` — Text.
7. `supportingEvidenceHash` — nullable Hash.
8. `completedAt` — Timestamp.
9. `operatorReference` — Identifier local to the application.

Action explanation, evidence content/filename, and operator reference remain
off-chain. The contract receives only the resulting action commitment inside a safe
response DTO.

## `ResponseV1`

Object type `05`; fields follow in exactly this order:

1. `responseId` — Identifier.
2. `responseVersionId` — Identifier.
3. `previousResponseVersionId` — nullable Identifier.
4. `eventId` — Identifier.
5. `organizationId` — Identifier.
6. `responseState` — ResponseState enum.
7. `receiptTimestamp` — Timestamp.
8. `policyVersionHash` — nullable Hash.
9. `decisionHash` — nullable Hash.
10. `actionHash` — nullable Hash.

This is a complete snapshot. Values accumulated in a predecessor are repeated and
must remain byte-identical in every descendant.

The example timestamp below is `1767225600000000000` Unix nanoseconds.

## Human-readable derivation example

For this synthetic `PolicyV1` input:

```text
policyId       = POLICY-HOSPITAL-A-EXCLUSION
organizationId = HOSPITAL-A
policyVersion  = V1
effectiveFrom  = 2026-01-01T00:00:00.000000000Z
effectiveUntil = null
policyText      = Suspend scheduling while exclusion is active.
```

the preimage is constructed as:

```text
48 54 52 4c                         # "HTRL"
00 01                               # V1
02                                  # Policy object
00 00 00 1b || "POLICY-HOSPITAL-A-EXCLUSION"
00 00 00 0a || "HOSPITAL-A"
00 00 00 02 || "V1"
18 86 72 51 ed fa 00 00             # signed BE Unix nanoseconds
00                                  # effectiveUntil is null
00 00 00 2d || "Suspend scheduling while exclusion is active."
```

Concatenating those bytes and applying SHA-256 produces:

```text
0x2c8b64b3f633a38d1afddb020aff4add3b435c0974666ed8ff4821b868bb4d21
```

Phase 4 must reproduce this digest as an immutable golden vector in backend and
contract-adapter tests. Implementations generate the preimage from typed values;
they must not hash the comments or human-readable rendering above.

## Test requirements

Tests must prove field-order sensitivity, null versus empty distinction, enum code
stability, UTC equivalence, nanosecond sensitivity, UTF-8/NFC behavior, length-prefix
ambiguity resistance, wrong-size hash rejection, deterministic repetition, and
different domain/type/version separation. A mutation test must change one off-chain
field, recompute the commitment, and demonstrate mismatch with confirmed ledger
evidence.
