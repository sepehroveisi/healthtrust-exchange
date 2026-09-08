# RC2 Phase 5C — Reconciliation and Crash Recovery

PostgreSQL and Besu cannot commit atomically. Reconciliation therefore uses the
stable assertion or response-version identity to establish chain truth before it
considers any retry. It never treats `LOCAL_PENDING` alone as permission to
broadcast.

## Authoritative lookup and matching

The frozen contracts emit one identity-indexed event for every immutable record.
The adapter filters that event by contract address and full bytes32 identity,
requires exactly one non-removed log, verifies its successful receipt and block
metadata, and reads the corresponding contract state directly. The reconciler
then compares every immutable field exposed by the Phase 3 contract. Missing
events mean absent; duplicate/removed events, mismatched receipts, conflicting
state, or unavailable RPC results fail closed.

Canonical PostgreSQL identifiers remain unchanged. IDs longer than the existing
1–32 byte on-chain boundary fail explicitly; reconciliation never truncates or
hashes them.

## State-specific behavior

- `LOCAL_PENDING`: perform chain lookup first. A matching record is recovered to
  `CONFIRMED` using its real event transaction and block metadata. An absent
  record may use the existing Phase 5B submission attempt with the same identity.
  Conflict or unavailable state prevents submission.
- `SUBMITTED`: inspect the durable transaction hash first. A pending receipt
  remains `SUBMITTED`; a failed receipt becomes `SUBMISSION_FAILED`; a successful
  receipt is cross-checked against matching contract state before confirmation.
- `SUBMISSION_FAILED`: chain lookup still runs first. Matching state is recovered,
  conflicting or unavailable state fails closed, and absence is not retried
  because Phase 5B currently emits no explicitly retryable failed-state class.
- `CONFIRMED`: reconciliation is a no-op.

Reconciliation errors are recorded without incrementing `attempt_count`. Only an
actual new broadcast does so. Bounded batch processing is sequential and returns
one result per operation; repeated runs converge without creating domain versions
or duplicate transactions.

## Crash windows

1. Database commit before broadcast: absence is proven, then normal submission
   proceeds.
2. Broadcast before transaction-hash persistence: matching state plus the indexed
   event recovers the genuine transaction hash, block number, and block hash.
3. Transaction hash persisted before receipt: pending stays `SUBMITTED`; a later
   successful receipt completes confirmation without rebroadcast.
4. Receipt observed before database confirmation: the known receipt is observed
   again and only the PostgreSQL confirmation transition is repaired.

Contract duplicate rejection remains defense in depth, not the recovery
algorithm. The chain-first decision happens before any broadcast.

## Boundaries and limitations

The response contract stores its ledger snapshot fields but not the separate
canonical PostgreSQL response commitment. Reconciliation compares all actual
on-chain response fields and verifies the local intended commitment against the
immutable local version; it does not invent an absent contract field. Historical
event lookup currently scans from genesis, suitable for the bounded RC2 network
but requiring indexed deployment ranges for larger production chains. This phase
does not add a daemon, distributed locks, contract changes, or Phase 6 APIs.
