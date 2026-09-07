# RC2 architecture plan

## Product framing

**HealthTrust — Professional Authority Response Ledger** is a portfolio-grade,
evidence-informed reference implementation exploring how independent healthcare
organizations can create independently verifiable evidence of how they respond to
professional-authority events.

> Shared evidence does not mean shared decisions.

RC2 supports one synthetic authority-event type, `EXCLUSION`, from
`HHS-OIG-DEMO`, concerning pseudonymous provider `PRV-7F31A`. Hospital A, Payer B,
and Staffing Agency C receive the same assertion and apply different synthetic
local policies. It is not a validated commercial, clinical, or compliance product.

## System context

```text
Synthetic authority source
        |
        v
Go application API ---- PostgreSQL (off-chain operational truth)
        |
        v
ResponseLedger adapter ---- Besu QBFT consortium
                              |       |       |
                           Hospital  Payer  Staffing
                           validator validator validator
        |
        v
React/Vinext demonstration UI
```

The three organizations may operate one validator each in the local demo. **This
one-to-one mapping is a demonstration topology, not a requirement of the
HealthTrust domain model.** A consortium organization, validator, transaction
signer, application user, and professional authority remain distinct identities.

## Domain boundaries

| Boundary | Responsibility | Excluded responsibility |
|---|---|---|
| Authority | Immutable assertions and their lineage | Organizational decisions |
| Organization response | Local receipt, review, decision, and action evidence | Authority truth |
| Evidence | Versioned canonical commitments and verification | Content correctness |
| Ledger | Shared commitments, attribution, ordering, and history | Local workflow execution |
| Persistence | Off-chain documents, policy, explanations, users, and workflow state | Consortium consensus |
| Presentation | Five-minute demonstration and explicit status | Authoritative decision making |

RC1 patient records, consent, referral, visit, and patient-matching concepts do
not belong to these boundaries. RC2 uses new packages and routes until RC1 runtime
code can be retired from the RC2 branch. The immutable `rc1` tag preserves RC1.

## Identity separation

| Identity | Meaning | Relationship |
|---|---|---|
| Consortium organization | Domain participant that owns local policy and responses | May operate zero or more validators |
| Besu validator | Consensus node identified by its Besu validator key | Operated by an organization only by deployment configuration |
| Transaction signer | Account authorized by a contract to submit a ledger action | May be a service account delegated by an organization or authority |
| Application user | Off-chain user acting through the application | Authenticated/authorized locally; never inferred from an Ethereum address |
| Professional authority | Approved source of authority assertions | Has an authority registry entry and a separately governed submitter |

Contract permissions must bind transaction signers to registry roles. That
binding is not proof of a person's real-world identity. Application-user identity
and permissions remain off-chain. Validator participation does not automatically
grant application or contract authority.

## Demo topology

The permissioned local consortium has three QBFT validators:

- Hospital Validator, representing Hospital A in demo deployment configuration.
- Payer Validator, representing Payer B.
- Staffing Validator, representing Staffing Agency C.

All validator and transaction-signer keys are local demo credentials. They make
no production key-management, organizational-control, or governance claim. There
is no public blockchain, token, cryptocurrency, mining, or marketplace.

Besu is selected because it provides a practical permissioned consortium
environment, EVM smart contracts, visible transaction history, and
developer-friendly local deployment. The specification does not claim that Besu
is uniquely correct for production.

## Contracts

Four minimal contracts are planned:

1. `OrganizationRegistry` records pseudonymous organization ID, type, active flag,
   registration time, and authorized submitter relationships.
2. `AuthorityRegistry` records authority ID, authority type, active flag,
   registration time, and authorized submitter relationships.
3. `AuthorityEventRegistry` appends event commitments and assertion lineage. It
   maintains a current head pointer without modifying prior assertions.
4. `ResponseLedger` appends response snapshots and predecessor links independently
   for each `(eventId, organizationId)` response stream.

Contracts validate caller authorization, uniqueness, predecessor/head matching,
and permitted state transitions. Contracts do not select a policy, decision, or
action for an organization.

## On-chain and off-chain boundary

Permitted on-chain values are pseudonymous IDs, canonical SHA-256 commitments,
state codes, predecessor IDs, effective/receipt times, organization/authority
references, and transaction/block metadata.

The following must never appear in contract storage, transaction calldata,
constructor arguments, revert strings, or emitted events:

- Provider names or real provider identifiers.
- Policy text, review notes, or decision/action explanations.
- Supporting documents or their filenames.
- PHI or patient records.
- Staff names or application-user identifiers.

PostgreSQL stores the synthetic provider record, source document, policy text,
review notes, decision explanation, action details, supporting-evidence metadata,
application users and permissions, and organization-local operational state.
Values intended for the ledger are assembled in a dedicated safe DTO and tested
against an allowlist before transaction submission.

## Responsibility of the ledger

The ledger provides shared commitments, attribution of submitted ledger actions,
append-only historical evidence, and independently verifiable transaction
history. It provides a common observation surface without centralizing local
decision authority.

The ledger does **not** prevent PostgreSQL mutation, prove that original off-chain
content was true, prove a local decision was correct, enforce a downstream action,
establish compliance, establish practitioner identity in production, or prove
real-world adoption. Integrity verification detects post-commitment mutation; it
does not prevent mutation.

## PostgreSQL responsibility

PostgreSQL is the operational system of record for RC2. It stores immutable event
and response versions, local detail, and the blockchain submission lifecycle.
Database constraints enforce local uniqueness and predecessor consistency where
possible. Besu confirmation is recorded only after a matching successful receipt
is observed. PostgreSQL and Besu are never described as one atomic transaction.

## Primary flow

1. Import a synthetic `EXCLUSION` assertion and persist it locally.
2. Compute the versioned event commitment and enqueue ledger submission.
3. Confirm the contract receipt and expose the assertion to three organizations.
4. Each organization appends `RECEIVED`, `UNDER_REVIEW`, `DECIDED`, and
   `ACTION_COMPLETED` response snapshots using its own policy.
5. Reconstruct the event and all response histories from PostgreSQL plus confirmed
   ledger metadata.
6. Recompute commitments and compare them with contract evidence.
7. In explicit demo mode only, mutate a designated off-chain synthetic field.
8. Re-run verification and display expected and current hashes as a detected
   mismatch.

## Failure modes

The UI must distinguish local persistence failure, pending submission, RPC
unavailability, rejected transaction, reverted receipt, confirmation timeout,
commitment mismatch, history/predecessor conflict, and validator/network
unavailability. A pending or submitted item is never displayed as confirmed.
Organizations may progress locally only according to an explicitly documented
policy when ledger submission is unavailable; the demo default is to show the
failure and stop the shared-evidence transition.

## Explicit non-claims and exclusions

RC2 is synthetic and not production-ready. It is not HIPAA certification,
regulatory guidance, a validated compliance product, production OIG integration,
or proof that a decision/action occurred outside the system. It has no FHIR,
patient records, patient consent, DID framework, credential wallet, token, AI,
production IAM, Kubernetes, or speculative microservice architecture.
