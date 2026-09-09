# RC2 Phase 8 — Controlled Tamper Demonstration

> **Demo-only dangerous functionality.** This capability intentionally bypasses normal append-only workflow protections for two fixed synthetic records. It is disabled unless an operator deliberately sets `HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED=true` and constructs the HTTP handler with the demo service.

HealthTrust does not prevent the operational PostgreSQL database from being modified. The demonstration shows that evidence committed before the modification can be used to detect a later inconsistency.

## Purpose and boundary

Phase 8 reuses the read-only Phase 7 verifier to show a bounded sequence: matching PostgreSQL and Besu evidence is first `VERIFIED`; one allowlisted synthetic PostgreSQL field is deliberately changed; the already committed Besu record remains unchanged; verification then reports `FAILED`. The service does not submit, reconcile, repair, restore, or otherwise write to Besu.

The gate fails closed. A missing value, `false`, `TRUE`, `1`, whitespace, or any value other than the exact lowercase string `true` leaves the service disabled. A disabled service returns `ErrDemoTamperDisabled`; the optional HTTP route returns `403` and performs no database operation. Ordinary `rc2http.Handler` does not install the route at all. Installing it requires the explicit `HandlerWithDemo` composition path, providing a second mechanical boundary in addition to the service gate.

## Supported synthetic targets

The request contains only a target selector. No SQL, table, column, identifier, JSON path, or replacement value is accepted.

- `AUTHORITY_EVENT` changes the source-document bytes of the fixed `EVENT-PHASE8` original exclusion assertion. The fixture must belong to `HHS-OIG-DEMO` and pseudonymous subject `PRV-7F31A`.
- `HOSPITAL_A_POLICY` changes the fixed Hospital A policy text referenced by the `ACTION_COMPLETED` response for `EVENT-PHASE8`.

Both values are deterministic constants selected by the server. The implementation uses parameterized SQL, fixed table and column names, exact fixture predicates, and requires exactly one affected row. Payer B, Staffing Agency C, arbitrary events, arbitrary organizations, and arbitrary fields cannot be selected through this API.

## Preconditions and result

The target fixture and its durable submission must exist and be `CONFIRMED`. Phase 7 must return `VERIFIED`, with equal current-local and committed-ledger commitments, before the mutation can run. An already `FAILED` target is rejected as a conflict, so the endpoint cannot repeatedly rewrite an already altered row. An `INDETERMINATE` or unavailable verification is rejected without mutation.

A successful response contains `demoOnly: true`, a warning, target and bounded fixture identity, changed-field identifier, and before/after Phase 7 results with full safe hexadecimal commitments. Success requires:

- before local commitment equals the committed ledger commitment;
- after local commitment differs from the committed ledger commitment;
- Phase 7 changes from `VERIFIED` to `FAILED`; and
- the observed ledger commitment remains identical.

If the database write fails, does not affect exactly one row, or the postcondition is not `FAILED`, the operation reports failure and never claims a successful demonstration. The service deliberately performs no automatic rollback or repair after a successful mutation.

## HTTP behavior

`HandlerWithDemo` adds:

```text
POST /api/rc2/demo/tamper
{"target":"AUTHORITY_EVENT"}
```

The other allowed value is `HOSPITAL_A_POLICY`. Success returns `200`; unsupported input returns `400`; a missing fixture returns `404`; disabled mode returns `403`; a failed clean-state precondition returns `409`; and unavailable pre-mutation ledger verification returns `503`. Internal database or postcondition failures return `500` without SQL, credentials, stack traces, keys, or transactions.

## Reset and repeatability

The supported reset is to destroy and recreate the disposable RC2 PostgreSQL/Besu environment, deploy the frozen contracts, and seed the deterministic Phase 8 scenario again. There is no repair or restore endpoint. Do not enable this mechanism against retained or operational data.

## Privacy and ResponseLedger limitation

Source documents and policy text remain local. They are neither returned by the demo API nor sent to Besu. Responses contain only synthetic identifiers, statuses, safe commitments, and structured Phase 7 checks. No patient data, real professional identity, explanation, reviewer/operator reference, supporting evidence, credential, key, or raw transaction is exposed.

`ResponseLedger` does not contain a standalone aggregate `ResponseV1` commitment. The Hospital A demonstration therefore changes evidence contributing specifically to `PolicyV1` and compares the recomputed local policy commitment with the immutable policy commitment in the existing on-chain response snapshot. It does not claim that a complete response hash exists on-chain.

## Limitations and non-claims

The demonstration establishes only that its bounded PostgreSQL update did not change the previously committed record observed from the configured Besu validators, and that Phase 7 detected the resulting mismatch. It does not prove PostgreSQL cannot be modified, that Besu prevents local mutation, that original evidence or an authority assertion was truthful, or that an organizational response was correct. It does not identify an actor, provide complete forensic attribution, prove absolute ledger immutability or organizational independence, establish production readiness or HIPAA/GDPR compliance, show that DLT is necessary, or show that real healthcare organizations require this workflow.
