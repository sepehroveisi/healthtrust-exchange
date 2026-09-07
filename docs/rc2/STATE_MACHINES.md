# RC2 state machines

## General rule

Authority assertions and organization responses are append-only. A new fact creates
a new immutable version linked to its predecessor. “Current” is a derived view of
the valid chain head, never a mutable historical row.

## Authority assertion identity

An authority-event series represents one continuing authority matter:

- `eventSeriesId`: stable pseudonymous identity for the series.
- `eventId`: immutable identity of one assertion/version in the series.
- `previousEventId`: immediate predecessor; absent only for the original.
- `targetEventId`: assertion being corrected, superseded, or lifted; absent for the
  original.
- `assertionKind`: `ORIGINAL`, `CORRECTION`, `SUPERSESSION`, or `REINSTATEMENT`.
- `authorityEffect`: `EXCLUSION_ACTIVE` or `EXCLUSION_LIFTED`.

Every assertion is a complete snapshot of the authority facts required by the
canonical event commitment. The event type remains `EXCLUSION`; assertion kind
describes why a later assertion exists.

### Valid authority history

```text
E1 ORIGINAL (EXCLUSION_ACTIVE)
  -> E2 CORRECTION (EXCLUSION_ACTIVE, target=E1)
  -> E3 SUPERSESSION (EXCLUSION_ACTIVE, target=E2)
  -> E4 REINSTATEMENT (EXCLUSION_LIFTED, target=E3)
```

Rules:

1. `ORIGINAL` has no predecessor or target and is the first series assertion.
2. A later assertion must name the current head as `previousEventId`.
3. `CORRECTION` identifies the inaccurate assertion in `targetEventId`, supplies a
   complete corrected snapshot, and normally retains the exclusion effect.
4. `SUPERSESSION` identifies the replaced assertion and supplies a complete new
   authority assertion.
5. `REINSTATEMENT` identifies the active exclusion assertion it lifts and has
   `EXCLUSION_LIFTED` effect.
6. Historical rows, commitments, and head ancestry are never overwritten.
7. An assertion cannot target another series, itself, or a nonexistent assertion.
8. A contract append must compare `previousEventId` with the registered current
   head. Concurrent stale appends revert; the caller must reload and reconcile.
9. RC2 v1 does not allow a second original, branching history, or an assertion
   following a lifted head. A later real-world exclusion would start a new series.

### Current applicability resolution

For a series, validate the lineage from its original to the unique registered head.
The head's complete snapshot is the currently applicable assertion. If its effect
is `EXCLUSION_ACTIVE`, the exclusion is applicable. If it is
`EXCLUSION_LIFTED`, the series no longer represents an active exclusion.

Display labels such as “corrected”, “superseded”, and “reinstated” are derived from
the assertion chain. They are not destructive updates to an original event. If
lineage is missing, branched, cyclic, or disagrees with the contract head, current
applicability is `UNRESOLVED` and the UI must not guess.

## Organization response identity

Each `(eventId, organizationId)` pair owns an independent response stream:

- `responseId`: stable identity of the stream.
- `responseVersionId`: immutable identity of one complete response snapshot.
- `previousResponseVersionId`: immediate predecessor; absent only for `RECEIVED`.
- `eventId`: exact authority assertion to which the organization responds.
- `organizationId`: responding organization.

A response to a corrected or reinstated authority assertion uses that new `eventId`.
It does not rewrite the organization's response to an earlier assertion.

## Response snapshot

Every response version repeats the accumulated committed response fields:

- Response and version identities.
- Event and organization identities.
- Current response state.
- Receipt timestamp.
- Policy version commitment, when selected.
- Decision commitment, when decided.
- Action commitment, when completed.
- Immediate predecessor.

Complete snapshots make each version independently verifiable. Off-chain policy,
review, decision, and action detail remains in separate immutable local records.

## Allowed response transitions

```text
none -> RECEIVED -> UNDER_REVIEW -> DECIDED -> ACTION_COMPLETED
```

| New state | Required snapshot values |
|---|---|
| `RECEIVED` | Receipt time; no predecessor; no decision/action commitment |
| `UNDER_REVIEW` | Predecessor in `RECEIVED`; policy commitment |
| `DECIDED` | Predecessor in `UNDER_REVIEW`; same policy commitment; decision commitment |
| `ACTION_COMPLETED` | Predecessor in `DECIDED`; same policy and decision commitments; action commitment |

A later snapshot must preserve immutable accumulated values from its predecessor.
It may add only values introduced by the new state.

Invalid transitions include:

- Skipping, repeating, or moving backward through states.
- Appending after `ACTION_COMPLETED`.
- Replacing a policy or commitment inherited from a predecessor.
- Pointing to another response stream, event, or organization.
- Branching from a predecessor that is no longer the stream head.
- Supplying decision data before `DECIDED` or action data before
  `ACTION_COMPLETED`.
- Reporting a successful state whose commitment transaction is not confirmed.

The three streams are independent. Hospital A may suspend privileges, Payer B may
hold enrollment, and Staffing Agency C may terminate an assignment. No response
transition reads or constrains another organization's policy outcome.

## Blockchain submission state

The local submission record has a separate operational lifecycle:

```text
LOCAL_PENDING -> SUBMITTED -> CONFIRMED
       |             |
       +-----------> SUBMISSION_FAILED
```

These states do not replace event or response states.

### `LOCAL_PENDING`

The immutable off-chain version, canonicalization version, commitment, idempotency
key, intended contract operation, and safe ledger DTO are committed to PostgreSQL
before any RPC call.

### `SUBMITTED`

The RPC accepted a signed transaction and returned a transaction hash. PostgreSQL
stores that hash, signer/account reference, chain ID, contract address, nonce where
safe, submission time, and attempt count. Submission is not confirmation.

### `CONFIRMED`

A successful receipt exists at a block number, the expected contract address and
method/event identifiers match, and emitted identifiers/commitments equal the
pending record. Only then may the UI label ledger evidence confirmed.

### `SUBMISSION_FAILED`

Submission was rejected, the transaction reverted, confirmation exceeded policy,
or the observed receipt did not match. The failure category and sanitized reason
are stored. A timeout with a known transaction hash must be reconciled before a new
transaction is sent.

## Retry and reconciliation

Every logical operation has a stable idempotency key enforced in PostgreSQL and a
unique version ID enforced by the contract. Retries reuse both. Duplicate contract
submissions either resolve to the existing committed version or revert without
creating a second logical record.

After restart, a reconciler processes `LOCAL_PENDING`, `SUBMITTED`, and retryable
failures. It queries known transaction hashes first, checks contract state by
version ID second, and submits only when neither proves prior acceptance. Backoff
is bounded and attempts are observable. A failed transaction never mutates the
off-chain version or invents a new version ID.

## UI projection

The UI shows `Pending ledger submission`, `Submitted — awaiting confirmation`,
`Confirmed`, or `Submission failed`. Event/response state and ledger submission
state appear separately. Failed and pending versions remain visible; neither is
presented as shared evidence.
