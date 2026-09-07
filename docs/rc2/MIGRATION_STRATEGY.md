# RC1 to RC2 migration strategy

## Objective

RC2 is a deliberate domain and ledger evolution, not an in-place conversion of
patient-exchange data. The public `rc1` tag remains the historical source of truth
for the HealthTrust Exchange prototype. Development occurs only on
`rc2/professional-authority-ledger` through normal forward commits.

No history, tag, or RC1 commit is rewritten. RC1 data is not transformed into
professional-authority data because the domains have no honest semantic mapping.

## Reuse boundary

Reusable engineering patterns include the Go HTTP service foundation, PostgreSQL
access, structured logging, explicit migrations, idempotency/reconciliation ideas,
canonical length-prefix encoding, frontend framework, Docker build patterns, and
test infrastructure.

RC1 patient, clinical record, visit, referral, consent, record discovery, requester
proof, healthcare policy, custom blockchain, transaction pool, peer block broadcast,
and chain-recovery modules remain isolated. They must not become aliases for RC2
authority or response concepts.

## Incremental transition

1. Freeze the RC2 specification in `docs/rc2`.
2. Add the Besu demo topology without changing the RC1 application entry point.
3. Add and test four minimal contracts.
4. Add new Go authority/evidence/ledger packages and a separate RC2 executable.
5. Add an isolated PostgreSQL schema and repositories.
6. Complete one vertical authority-event commitment before response workflows.
7. Add independent organization response streams and verification.
8. Replace the frontend product surface after APIs are stable.
9. Add guarded mutation and complete E2E validation.
10. Retire RC1 runtime code from the RC2 branch only after replacement coverage is
    proven. The `rc1` tag retains the prior implementation.

Small commits and relevant validation are required at every step. No phase silently
continues past a serious failure.

## Database strategy

The first RC2 database migration should be additive and create an
`authority_ledger` PostgreSQL schema. It must not drop, rename, reinterpret, or copy
RC1 clinical tables.

Planned logical tables are:

- `organizations`, `authorities`, and `providers` for local operational references.
- `authority_events` for stable series identity.
- `event_versions` for immutable authority assertions and predecessor/target links.
- `policies`, `reviews`, `decisions`, and `actions` for immutable local evidence.
- `response_records` for immutable complete response snapshots.
- `blockchain_transactions` for the submission/reconciliation lifecycle.

They live under the RC2 schema to avoid collision with RC1 `organizations` and
`transactions`. Foreign keys, unique constraints, check constraints, and partial
indexes enforce local invariants. Current views are projections over immutable
history, not mutable source rows.

The existing Go `schemaSQL` duplicates numbered RC1 migration definitions. RC2 must
not add a third schema source. Before RC2 persistence is activated, one authoritative
migration runner must be selected. Numbered up/down SQL files are the intended
artifact; application startup may invoke the runner but must not embed a divergent
schema copy.

## Ledger deployment state

Contract deployments are environment data, not source-domain entities. Persist and
validate chain ID, contract name/version, deployed address, deployment transaction,
and deployment block. The application must fail closed when configured addresses or
chain identity do not match expected contract code/version.

Validator keys and transaction-signer keys are separate. Local reproducible demo
credentials must be labeled synthetic and must not be reused outside the demo.

## Non-atomic commitment lifecycle

For every event or response version, one local database transaction persists:

- The immutable off-chain object/version.
- Canonicalization version and SHA-256 commitment.
- Stable operation idempotency key.
- Intended contract operation and allowlisted ledger DTO.
- A `LOCAL_PENDING` blockchain transaction record.

Only after that transaction commits may the application submit to Besu. On RPC
acceptance, the returned transaction hash advances the record to `SUBMITTED`. A
matching successful receipt advances it to `CONFIRMED`. Rejection, revert, invalid
receipt, or exhausted confirmation policy advances it to `SUBMISSION_FAILED` with a
sanitized category.

There is no distributed transaction and no rollback of one system by pretending to
undo the other.

## Retry and restart recovery

A stable version ID is the contract-level uniqueness key; a stable operation key is
the PostgreSQL idempotency key. A retry never creates a new logical version merely
because an RPC result was lost.

On restart, reconciliation:

1. Loads nonterminal blockchain transaction records.
2. Checks a known transaction hash and validates any receipt.
3. Checks contract state by immutable version ID if the hash is absent or ambiguous.
4. Marks confirmed evidence when matching state is found.
5. Submits only if no prior acceptance exists and retry policy permits it.
6. Uses bounded backoff and records attempt/error metadata.

A known hash that is pending or temporarily unavailable is never blindly replaced
with another transaction. Contract duplicate protection is the final defense, not a
substitute for local idempotency.

## Cutover and rollback

Docker Compose will eventually switch from two RC1 hospital nodes to the RC2 API,
PostgreSQL, three Besu validators, and contract deployment/bootstrap tooling. That
change occurs only after backend and contract smoke tests pass.

Application rollback may select an earlier compatible RC2 image. Database rollback
is permitted only before incompatible RC2 data is written or through a tested
forward repair. A SQL down migration cannot erase or reverse confirmed blockchain
history. Destroying local demo volumes is a separate, explicitly guarded demo reset,
not a production rollback strategy.

## Frontend and documentation transition

The existing Sites/Vinext build infrastructure and `.openai/hosting.json` remain.
RC2 pages replace the product surface only after stable APIs exist. RC1 screenshots
and documentation remain understandable through the tag; the RC2 README transition
is intentionally deferred until the documentation phase.

## Completion criteria

Migration is complete only when the RC2 Compose smoke test, contract tests, Go tests
and race checks, PostgreSQL integration tests, frontend validation, and required
Playwright scenarios pass; no RC1 patient concept is exposed by the RC2 runtime; and
the README accurately distinguishes RC1 history from RC2 claims.
