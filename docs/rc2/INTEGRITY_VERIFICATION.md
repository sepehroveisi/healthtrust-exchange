# RC2 Phase 7 — Integrity Verification

Phase 7 answers one bounded question: does current durable PostgreSQL evidence still agree with evidence previously committed to the configured Besu consortium ledger? The verifier is read-only. It reports inconsistency; it does not prevent PostgreSQL writes, repair data, reconcile submission state, or submit blockchain transactions.

## Result semantics

- `VERIFIED`: every required local canonical, immutable ledger, lineage, and confirmed transaction check agrees.
- `FAILED`: verification completed and found at least one cryptographic, identity, state, lineage, or transaction-metadata mismatch.
- `INDETERMINATE`: no mismatch was established, but required ledger or receipt evidence could not be obtained or was not confirmed.

Checks contain a machine-readable category and status, bounded object identity, safe expected/observed identifiers or commitments, and a deterministic classification. Bundle aggregation is strict: any `FAILED` result makes the bundle `FAILED`; otherwise any `INDETERMINATE` result makes it `INDETERMINATE`; only all-verified evidence produces `VERIFIED`.

## Authority-event verification

The service reads the PostgreSQL assertion and series, hashes the current source document locally, reconstructs `AuthorityEventV1`, and invokes the frozen Phase 4 canonical commitment function. It compares that current commitment with both the durable intended commitment and the immutable Besu assertion commitment. It also compares event, series, authority, event type, root/predecessor lineage, current series head, and confirmed transaction hash, successful receipt, block number, and block hash.

Authority history is read in deterministic append order. Its first assertion must remain the `ORIGINAL` root, each later item must reference its persisted predecessor, ledger predecessor fields must agree, and the PostgreSQL latest item must match the ledger current head. Phase 7 does not add correction, supersession, or reinstatement commands.

## Organization-response verification

Each organization is verified separately. For every persisted response version, the service locally reconstructs the applicable Phase 4 `PolicyV1`, `DecisionV1`, `ActionV1`, and `ResponseV1` evidence. It compares the recomputed local response commitment with the durable intended commitment and compares the individual ledger snapshot fields actually stored by `ResponseLedger`:

- response and version identity;
- event and organization identity;
- lifecycle state and receipt timestamp;
- predecessor identity;
- policy, decision, and action commitments.

The verifier requires exactly one `RECEIVED` root and validates the frozen sequence `RECEIVED → UNDER_REVIEW → DECIDED → ACTION_COMPLETED`, stable stream/event/organization identity, predecessor linkage, immutable ledger ordering, and agreement between local latest state and the contract's latest response.

`ResponseLedger` does not store the separate canonical PostgreSQL `ResponseV1` commitment. Phase 7 therefore does not claim a direct on-chain comparison of that aggregate hash and does not redesign the contract. It verifies the canonical local hash against the durable submission intent and independently verifies every response snapshot field and nested evidence commitment that the frozen contract actually stores.

## Transaction evidence

For a PostgreSQL submission marked `CONFIRMED`, verification reads the receipt without changing submission state. The receipt must exist, be successful, and match the durable transaction hash, block number, and block hash. Missing, pending, or inaccessible receipt evidence is `INDETERMINATE`; a returned failed receipt or mismatched durable metadata is `FAILED`. This is not Phase 5C reconciliation and never invokes it.

## HTTP API

- `GET /api/rc2/authority-events/{eventID}/verification`
- `GET /api/rc2/authority-events/{eventID}/responses/{organizationID}/verification`
- `GET /api/rc2/authority-events/{eventID}/verification/bundle`

Successful verification execution returns `200`, including a structured `FAILED` integrity result. Invalid identifiers return `400`, missing domain objects return `404`, and unavailable required ledger evidence returns `503` with the structured `INDETERMINATE` result. No mutation endpoint exists.

## Privacy and trust boundary

PostgreSQL remains the operational system of record and may contain organization-local policy text, explanations, reviewer/operator references, source/supporting documents, and other private evidence. These values are used only for local canonical recomputation. They are never sent to Besu or included in verification responses. The ledger boundary remains restricted to bounded pseudonymous identifiers, lifecycle values, timestamps, lineage, transaction metadata, and commitments.

The test suite uses direct SQL only inside integration-test code to prove that PostgreSQL mutation succeeds while prior Besu evidence remains unchanged and makes the inconsistency detectable. Production/application code contains no tamper, mutation, corruption, or automatic repair endpoint.

## Limitations and non-claims

`VERIFIED` means agreement with previously committed bytes, not truth. Phase 7 does not prove that an authority source was correct or truthful, that a professional committed wrongdoing, that an organizational decision or action was correct, or that a ledger was necessary. It does not make PostgreSQL immutable, prevent tampering, establish forensic completeness, provide real organizational independence, implement real HHS/OIG integration, establish production readiness, or certify HIPAA, GDPR, or other regulatory compliance.
