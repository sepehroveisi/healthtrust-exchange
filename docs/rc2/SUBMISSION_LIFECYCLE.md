# RC2 Phase 5B — Normal Submission Lifecycle

Phase 5B coordinates one already-durable `LOCAL_PENDING` operation during a
single process lifetime. It does not implement restart reconciliation.

## Ordering and states

The required order is: commit the domain version and `LOCAL_PENDING` row,
broadcast using the correct application signer, persist the returned transaction
hash as `SUBMITTED`, observe a successful receipt, then persist its transaction
hash, block number, and block hash as `CONFIRMED`.

- `LOCAL_PENDING` means the immutable domain version and submission intent exist
  in PostgreSQL, but no transaction hash is durable.
- `SUBMITTED` means a real transaction hash is durable. It is not confirmation.
- `CONFIRMED` means a successful receipt was observed and its block metadata is
  durable.
- `SUBMISSION_FAILED` means a definite broadcast failure or an explicitly failed
  mined receipt was classified. It never means a receipt merely timed out.

A timeout or temporary receipt RPC error leaves the row `SUBMITTED`. An ambiguous
broadcast error leaves it `LOCAL_PENDING`; Phase 5B does not retry because the RPC
may have accepted the transaction.

## Attempts, signers, and privacy

Every actual broadcast attempt increments `attempt_count` and sets
`last_attempt_at`. A classified failure sets `last_error_class`; successful
progress clears stale error classification. Authority assertions use the
authority application signer, organization responses use the organization
application signer, and neither uses QBFT validator keys.

Domain state and ledger-submission state are independent. Contract calls contain
only identifiers, timestamps, state enums, and commitments. PostgreSQL-only
display data, source documents, policy text, explanations, reviewer/operator
references, and supporting evidence are not passed to the ledger adapter.

## Explicit Phase 5C boundary

Two windows remain unresolved intentionally: broadcast can succeed before the
transaction hash is persisted, and a successful receipt can be observed before
confirmation metadata is persisted. Phase 5B surfaces these persistence errors
and never rebroadcasts internally. Phase 5C must reconcile both cases using
stable logical identity and authoritative contract reads.
