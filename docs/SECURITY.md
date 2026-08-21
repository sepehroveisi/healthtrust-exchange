# Security model

HealthTrust Exchange is an engineering prototype. It demonstrates security boundaries and failure handling; it is not production-secure and makes no HIPAA, GDPR, FHIR, or other compliance claim.

## Current controls

- Actors are registered with Ed25519 public keys and organization membership.
- Blockchain transactions are signed and checked against registered identities and role policy.
- Hospital-to-hospital requests carry signed peer envelopes bound to method, path, body hash, source, target, nonce, and expiry.
- Requester proofs bind doctor, organization, record, issuance, expiry, and a unique proof ID.
- PostgreSQL uniqueness constraints durably reject replayed proof and peer-request IDs.
- Source hospitals authorize access using active patient consent and doctor/organization identity.
- Trusted discovery uses signed, body-bound peer envelopes and durable replay prevention. It returns no clinical content and grants no retrieval authority.
- Walk-in consent is bound to the correlated patient, requesting doctor and organization, source organization, access request, and one resolved source record.
- Canonical SHA-256 record hashing detects changes to committed clinical fields.
- Idempotency keys and request hashes prevent duplicate or conflicting mutations.
- Pending-state reconciliation closes recoverable dual-write failure windows.

## Trust assumptions

- Each hospital protects its database, process, and signing material.
- Demo identity and node private keys are deterministically derived from public source-code labels. They are publicly reproducible, intentionally insecure, and exist only so the demo is repeatable. A production deployment would generate and hold unrelated keys in managed cryptographic infrastructure.
- `network_patient_id` and the organization-scoped demo consent signer are prototype identity mechanisms, not production patient authentication or a master patient index.
- Registered identity data is trusted for the current chain history.
- The two-node prototype does not solve Byzantine consensus or malicious-majority behavior.

## Known limitations

- Deterministic demo keys are intentionally reproducible and are not secrets.
- Demo Mode is not real authentication or session management.
- Local HTTP has no production TLS or mTLS; application signatures do not replace transport security.
- There is no secret manager, HSM, PKI lifecycle, certificate rotation, or historical actor-key rotation.
- Historical blocks validate against the current registry state.
- There is no production monitoring, incident response, backup policy, privacy impact assessment, or regulatory certification.
- Organization-specific timezone policy is not implemented; demo visit-day boundaries use the current prototype's fixed time handling.
- The frontend dependency graph may contain documented advisories in development/build-only transitive tooling. Review `npm audit` before publication or deployment; accepted prototype risks are not production acceptance.
- Clinical models are fictional and deliberately minimal.

Never use real patient data or production credentials with this repository.
