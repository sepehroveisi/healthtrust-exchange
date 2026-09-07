# RC2 threat model

## Scope and assets

This model covers the synthetic local reference implementation, not a production
risk assessment. Assets include off-chain authority documents, pseudonymous provider
correlation, policy/review/decision/action detail, canonical commitments, immutable
event and response lineage, contract permissions, transaction and validator keys,
PostgreSQL state, and the integrity result shown to a reviewer.

## Trust boundaries

1. Browser to Go API.
2. Go API to PostgreSQL.
3. Go ledger adapter to Besu JSON-RPC.
4. Transaction signer to smart contracts.
5. Besu validator-to-validator consensus network.
6. Synthetic authority ingestion boundary.
7. Demo mutation control boundary.

The demo assumes its host and containers are controlled, its synthetic fixture is
known, and QBFT validators execute the configured chain. It does not assume that an
organization's decision is correct merely because its commitment is recorded.

## Identity threats

The consortium organization, Besu validator, transaction signer, application user,
and professional authority are distinct. Treating any one as another could let a
validator make authority assertions, a signer impersonate an application reviewer,
or a user gain consortium governance power.

Required controls include explicit registry roles, separate keys, local application
authorization, contract caller checks, environment-specific mappings, and audit
records of mapping changes. The demo's one organization-to-validator mapping is
topology only.

## Threat register

| Threat | Impact | Reference-implementation control | Residual limitation |
|---|---|---|---|
| Forged authority assertion | False exclusion evidence | Approved authority and submitter registry; signed transaction; synthetic fixtures | No production authority identity proof |
| Unauthorized response | False organizational attribution | Organization submitter allowlist and event/org binding | Demo keys are locally controlled |
| Compromised transaction signer | Valid-looking false commitments | Narrow contract role and revocation plan | No production HSM/KMS claim |
| Compromised validator | Censorship or consensus disruption | Three-validator QBFT topology and independent logs | Local demo is not operational independence or Byzantine governance proof |
| PostgreSQL mutation | Current evidence differs from committed evidence | Deterministic recomputation and mismatch display | Ledger detects; it does not prevent or repair mutation |
| Database deletion | Missing local explanation/history | Ledger commitment proves something was recorded | Hash alone cannot reconstruct deleted content |
| Calldata privacy leak | Sensitive data permanently replicated | Safe ledger DTO allowlist; contract/API tests inspect calldata and events | Pseudonymous metadata may still be correlatable |
| Event-log privacy leak | Sensitive data emitted despite safe storage | Events emit only IDs, commitments, states, and lineage | Public-looking demo IDs must remain synthetic |
| Hash substitution | Attacker commits a hash of false content | Authority/organization attribution and immutable lineage | Commitment does not prove original truth |
| Canonicalization ambiguity | Same-looking content hashes differently | Versioned binary format, NFC/UTF-8 rules, golden vectors | Cross-language implementation requires continued tests |
| History fork | Two “current” assertions or responses | Contract head comparison and predecessor validation | Concurrent stale writes require reconciliation |
| Invalid transition | Misleading lifecycle | Strict contract and Go transition validation | Does not prove real-world action occurred |
| RPC response spoofing | False confirmation | Validate chain ID, contract address, receipt status, event values, and block metadata | Transport hardening is demo-level |
| Lost RPC response | Duplicate submission | Stable version ID, operation idempotency, receipt/contract lookup before retry | Nonce management can still stall |
| Validator key disclosure | Node impersonation | Keys explicitly demo-only and separated from signers | No production rotation/custody control |
| Transaction-signer key disclosure | False attributed submissions | Contract role revocation and separate signer roles | Local demo credentials are not secure storage |
| Replay/duplicate request | Duplicate logical evidence | Database idempotency plus contract uniqueness | Must retain reconciliation records |
| Clock manipulation | Incorrect effective/receipt ordering | UTC nanoseconds, validation windows, ledger order shown separately | Blockchain time does not prove authority time |
| UI status confusion | Submitted shown as confirmed | Separate domain and submission states | Users may still over-trust blockchain language |
| Demo mutation exposed | Intentional corruption used as normal edit | Explicit demo-mode guard, synthetic-only target, warning, audit log | Not suitable for deployed production mode |
| Cross-organization policy coupling | Shared ledger dictates decisions | Contracts validate structure only; response streams independent | UI copy must preserve autonomy |
| Dependency/container compromise | Altered API, contracts, or nodes | Pinned versions and reproducible builds planned | No production supply-chain assurance |
| Denial of service | Ingestion/confirmation unavailable | Timeouts, retries, visible failure states | No HA or capacity claim |

## On-chain privacy rules

Privacy review covers storage, calldata, logs/events, constructor arguments, revert
messages, and transaction input retained by every node. None may contain provider
names, real provider identifiers, policy text, review notes, decision/action
explanations, supporting documents or filenames, PHI, staff names, application-user
IDs, or authentication data.

The demo uses `PRV-7F31A`, `HHS-OIG-DEMO`, `HOSPITAL-A`, `PAYER-B`, and
`STAFFING-AGENCY-C`. These are synthetic/pseudonymous demonstration identifiers.
Even pseudonymous IDs and timing can create linkability, so production privacy cannot
be inferred from the absence of names.

Contract tests must decode transaction calldata and emitted events and assert the
exact allowlist. Backend tests must prove that safe ledger DTOs cannot serialize
off-chain detail fields.

## Integrity demo safety

The mutation mechanism exists solely to demonstrate detection. It must be compiled
or enabled only through an explicit demo/test configuration, reject non-synthetic
targets, require an unmistakable endpoint and confirmation, log its use, and never
appear as ordinary editing. Verification returns the confirmed expected hash and
fresh current hash without revealing the preimage.

A successful verification means only that current canonical bytes match the
recorded commitment. A failure means they do not match. Neither result establishes
truth, correctness, completeness, compliance, or real-world action.

## Consortium and governance limitations

All three validators may run on one developer machine. That demonstrates protocol
topology, not independent operations. QBFT does not define real participant
onboarding, legal responsibility, key custody, dispute handling, emergency removal,
software governance, or production availability.

A malicious or compromised consortium participant may censor, delay, correlate, or
submit authorized-but-false evidence. Shared history makes actions attributable and
inspectable; it does not make participants honest.

## Failure handling requirements

Fail closed on unknown authority/organization/signer, wrong chain or contract,
invalid predecessor, invalid transition, commitment mismatch, reverted transaction,
or ambiguous confirmation. Preserve local pending/failed evidence for diagnosis.
Do not silently create replacement logical versions during retries.

Operational output must not log off-chain content or credentials. Logs may contain
pseudonymous IDs, transaction hash, block number, contract, submission state, and
sanitized failure category.

## Explicit non-claims

Blockchain evidence does not prove clinical truth, practitioner identity, authority
source authenticity in production, correctness of policy or decision, execution of
a downstream action, regulatory compliance, consortium independence, or superiority
to a centralized signed audit log. RC2 contains no real OIG ingestion, production
IAM, patient data, patient consent, FHIR, DID, credential wallet, token, AI, or
production key management.
