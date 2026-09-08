# RC2 Phase 6 Application Workflow

Phase 6 is the application orchestration boundary for the bounded professional-authority response ledger. It accepts one synthetic `EXCLUSION` event type and coordinates independent organization response streams using the frozen Phase 4 canonical evidence and ledger adapter and the Phase 5 persistence, submission, and reconciliation services.

## Authority event ingestion

`RegisterAuthorityEvent` validates the configured authority and pseudonymous professional subject, creates or validates the event series, canonicalizes an `ORIGINAL` assertion with effect `EXCLUSION_ACTIVE`, and atomically persists the assertion with a `LOCAL_PENDING` submission. It then delegates submission to Phase 5B. A confirmation failure does not delete the durable local assertion; the returned read model reports its current ledger state separately.

Event reads expose the assertion and series identifiers, authority and pseudonymous subject identifiers, event semantics and effective time, current series head, canonical commitment, operational creation time, and durable transaction/block status. `GetAuthorityHistory` returns the append-only assertion sequence in deterministic order. Phase 6 does not provide correction, supersession, or reinstatement commands.

## Organization response workflow

Hospital A, Payer B, and Staffing Agency C each create exactly one response stream for the same event. No response is created implicitly for another organization. Every stream advances only through:

`RECEIVED` → `UNDER_REVIEW` → `DECIDED` → `ACTION_COMPLETED`

Each transition appends a new immutable response version, links it to its predecessor, preserves the event, organization, and response-stream identities, creates its Phase 4 canonical commitment, and atomically persists the version with a `LOCAL_PENDING` submission before delegating to Phase 5B.

Policies, decision explanations and reviewer references, action explanations and operator references, and supporting evidence remain organization-local PostgreSQL data. Only bounded identifiers, lifecycle state, timestamps, and commitments cross the ledger adapter boundary.

Current-response reads are scoped by `(eventID, organizationID)`. History reads never merge organizations and return the four version states in deterministic append order. The application response keeps domain state and ledger state distinct: a locally valid version may be `LOCAL_PENDING`, `SUBMITTED`, `CONFIRMED`, or `SUBMISSION_FAILED` at the submission layer.

## Idempotency and organization boundary

Stable event and response identities provide request idempotency. Repeating an authority event or response start with the same immutable canonical payload returns the existing durable object and current status. Reusing an identity with different immutable content returns a conflict and creates no duplicate.

The HTTP caller supplies its acting organization through `X-Organization-ID`. The service verifies it matches the organization in the response command, so one organization cannot start or advance another organization's stream. This is a bounded application authorization guard, not production authentication or OAuth/OIDC.

## Reconciliation

`ReconcileOperation` and `ReconcileUnresolved` delegate directly to Phase 5C. They do not reproduce receipt interpretation, retry rules, contract calls, or conflict detection. This permits durable work that survived an unavailable ledger or delayed confirmation to converge later.

## HTTP API

- `POST /api/rc2/authority-events`
- `GET /api/rc2/authority-events`
- `GET /api/rc2/authority-events/{eventID}`
- `POST /api/rc2/authority-events/{eventID}/responses/{organizationID}`
- `POST /api/rc2/authority-events/{eventID}/responses/{organizationID}/advance`
- `GET /api/rc2/authority-events/{eventID}/responses`
- `GET /api/rc2/authority-events/{eventID}/responses/{organizationID}`
- `GET /api/rc2/authority-events/{eventID}/responses/{organizationID}/history`
- `POST /api/rc2/reconciliation/{operationID}`
- `POST /api/rc2/reconciliation?limit={positive integer}`

Malformed input returns `400`, missing resources `404`, domain/authorization/on-chain conflicts `409`, and unavailable or pending ledger confirmation `503`. Responses do not expose stack traces, database details, RPC URLs, private keys, signer material, or raw transactions.

## Synthetic integration scenario

The integration fixture uses `HHS-OIG-DEMO`, pseudonymous subject `PRV-7F31A`, Hospital A, Payer B, and Staffing Agency C. It registers one synthetic exclusion assertion, advances three separate response histories to `ACTION_COMPLETED`, verifies evidence from another Besu validator, rejects cross-organization mutation, and proves a locally pending response survives a temporary submission failure and is confirmed by reconciliation.

## Limitations and non-claims

This phase has no frontend, production identity provider, real HHS/OIG feed, real practitioner or patient data, FHIR/DID integration, notifications, or distributed workflow engine. It does not establish clinical validity, regulatory compliance, production readiness, enterprise scalability, or that blockchain is necessary for the use case.
