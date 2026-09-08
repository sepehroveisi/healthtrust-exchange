# RC2 PostgreSQL persistence

## Boundary and authority

PostgreSQL is the RC2 operational system of record. Besu stores shared commitments,
attribution, and lineage; it does not store professional details, source documents,
policy text, explanations, reviewer/operator references, or supporting evidence.
All RC2 tables live in the isolated `authority_ledger` schema. RC1 data is neither
copied nor transformed, and all demonstration content is synthetic.

Numbered files in `migrations/authority_ledger` are the only RC2 schema-definition
authority. `public.authority_ledger_migration_versions` records which numbered files
ran; it contains no schema SQL and is not a second definition source. Reapplying is
safe because recorded versions are skipped and each migration runs transactionally.

The down migration drops the entire RC2 schema. It is destructive and intended only
for disposable development/test databases. SQL rollback cannot remove or reverse
Besu history and is not a production blockchain rollback mechanism.

## Tables and invariants

Registries store organizations, authorities, and pseudonymous professional subjects.
Event series own immutable authority assertions. Composite foreign keys keep
predecessors and targets in the same series; partial unique indexes permit one root
and one successor per predecessor.

Policies, decisions, and actions retain organization-local operational evidence.
`response_streams` gives `response_id` stable meaning for one event/organization
pair. Immutable response versions use composite foreign keys and partial unique
indexes to prevent cross-stream ancestry, multiple roots, and branching successors.

PostgreSQL enforces identity uniqueness, reference integrity, enum sets, lineage
shape, signer length, exact 32-byte commitments, and ledger-submission metadata
shapes. Go additionally verifies policy/decision/action organization and event
compatibility when creating decisions, actions, and response versions. Phase 6 must
enforce the complete authority and response lifecycle semantics; Phase 5A does not
claim to implement that workflow state machine.

Canonical domain timestamps are signed 64-bit Unix nanoseconds stored as `bigint`,
preserving the Phase 4 representation exactly. Operational `created_at` and
`updated_at` values use `timestamptz`. SHA-256 commitments use `bytea` constrained to
exactly 32 bytes.

## Atomic local boundary

Creating a ledger-bound authority assertion or response version inserts both the
immutable domain version and its `LOCAL_PENDING` ledger submission in one PostgreSQL
transaction. Failure of either insert rolls back both. No blockchain call occurs
inside this transaction. Domain lifecycle state and ledger-submission state are
separate columns and separate state machines.

Phase 5A intentionally provides persistence primitives only. Transaction broadcast,
receipt processing, retry, reconciliation, and crash recovery remain unimplemented
until later Phase 5 sub-phases.
