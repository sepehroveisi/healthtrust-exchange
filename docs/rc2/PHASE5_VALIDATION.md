# RC2 Phase 5 — Final Validation

Phase 5 validates the bounded RC2 authority-response ledger across real
PostgreSQL persistence, signed Besu submission, receipt confirmation, and
restart-safe reconciliation. PostgreSQL remains the operational system of
record. Besu provides independently verifiable commitments, lineage, and shared
evidence.

## Integrated scenario

The final test persists Hospital A, Payer B, Staffing Agency C, the synthetic
HHS-OIG authority, pseudonymous professional `PRV-7F31A`, and one synthetic
original exclusion assertion. The assertion and every response version are
created atomically with `LOCAL_PENDING` submission intent and progress through
the frozen durable lifecycle.

Hospital A, Payer B, and Staffing Agency C independently complete:

`RECEIVED → UNDER_REVIEW → DECIDED → ACTION_COMPLETED`

Each stream has its own policy, decision, and action commitments and its own
authorized application signer. Shared evidence does not mean shared decisions.
Cross-organization submissions are rejected.

## Durability and reconstruction

The scenario closes and recreates PostgreSQL stores, submission/reconciliation
services, and Besu clients. It recovers three deliberate interruption states:

- broadcast completed while PostgreSQL remained `LOCAL_PENDING`;
- a durable `SUBMITTED` transaction whose receipt was not yet processed;
- a successful receipt whose local confirmation update was omitted.

Recovery uses real receipts and identity-indexed contract evidence and never
blindly rebroadcasts. A second full reconstruction finds no unresolved work;
reconciling every confirmed operation changes no transaction hash, block
metadata, attempt count, domain version, or chain record.

## Consistency and privacy

Every PostgreSQL `CONFIRMED` row is matched against a genuine chain transaction
hash, block number, block hash, and immutable contract record. Independent reads
through all three validator RPC endpoints agree on the assertion, authority
head, twelve response versions, predecessor lineage, states, commitments, and
timestamps.

Only approved identifiers, timestamps, enums, lineage, and commitments enter
Besu. Professional details, source documents, policy text, decision/action
explanations, reviewer/operator references, supporting evidence, patient data,
private keys, and raw signed transactions are excluded from ledger state and
logs.

## Regression matrix

- Phase 3: 27 Solidity contract tests and three-validator QBFT behavior.
- Phase 4: canonical golden vectors, adapters, signer authorization, receipts,
  and multi-node reads.
- Phase 5A: migrations, constraints, repositories, histories, and atomic pairs.
- Phase 5B: normal submission, ordering, failure classification, and durable
  confirmation.
- Phase 5C: chain-first reconciliation, crash windows, conflict/unavailability
  handling, identifier boundaries, idempotency, and batch convergence.
- Phase 5D: the complete three-organization scenario, actual reconstruction,
  PostgreSQL/Besu consistency, autonomy, and three-node agreement.

The frozen PolicyV1 golden commitment remains
`0x2c8b64b3f633a38d1afddb020aff4add3b435c0974666ed8ff4821b868bb4d21`.

## What Phase 5 does not prove

Blockchain does not prevent PostgreSQL mutation. Phase 5 does not implement the
user-facing integrity/tamper demonstration, Phase 6 APIs, production deployment,
real authority integration, compliance certification, enterprise scalability,
or Byzantine-security claims beyond the configured three-validator demo.
ResponseLedger stores the response snapshot fields rather than the separate
PostgreSQL canonical response commitment, and reconciliation event lookup scans
from genesis in this bounded network.

Validated claim: **Phase 5 PASS: durable PostgreSQL persistence, signed Besu
submission, confirmation, and restart-safe reconciliation have been validated
for the bounded RC2 authority-response ledger.**
