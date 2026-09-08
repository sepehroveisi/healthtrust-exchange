# Professional Authority Response Ledger contracts

This Foundry workspace implements the minimal RC2 shared-evidence contract layer.
It does not implement organizational business policy, application users, production
identity proof, PostgreSQL persistence, or Go integration.

## Toolchain

- Foundry `v1.8.1`, pinned in the helper scripts.
- Solidity `0.8.30`, EVM target `london`, pinned in `foundry.toml`.
- Optimized IR compilation is enabled for deterministic builds and the integrated test.
- No JavaScript contract-development dependencies are required.

Run formatting, compilation, tests, and ABI privacy validation from the repository root:

```bash
./contracts/scripts/forge.sh fmt --check
./contracts/scripts/forge.sh build
./contracts/scripts/forge.sh test
./contracts/scripts/privacy-check.sh
```

## Responsibilities and wiring

`OrganizationRegistry` is administered by the immutable deployment administrator.
It records a pseudonymous organization ID, RC2 organization type, active state,
registration time, and one authorized submission address.

`AuthorityRegistry` has the same immutable administrator and records the synthetic
`HHS-OIG-DEMO` exclusion authority plus its separately authorized submitter.
Registration is attribution for this demo, not proof of real-world authority identity.

`AuthorityEventRegistry` references `AuthorityRegistry`. It accepts only authorized
authority submissions, stores immutable complete assertion evidence, validates the
`ORIGINAL`, `CORRECTION`, `SUPERSESSION`, and `REINSTATEMENT` lineage, and advances a
derived current-head pointer without modifying history. RC2 supports only `EXCLUSION`.

`ResponseLedger` references both `OrganizationRegistry` and
`AuthorityEventRegistry`. For every `(eventId, organizationId)` it accepts only that
organization's signer and appends complete response snapshots through exactly:
`RECEIVED -> UNDER_REVIEW -> DECIDED -> ACTION_COMPLETED`. It does not inspect or
constrain another organization's response.

The contracts are deliberately non-upgradeable. RC2 favors small immutable reference
contracts over proxy administration and does not claim a production governance model.

## Authorization identities

The deterministic local demo uses distinct, publicly documented accounts:

| Purpose | Address |
| --- | --- |
| Bootstrap/deployer administrator | `0xae39ab287e6e3d2fc48d6e51d1445f3a8106e743` |
| Authority submitter | `0xe05fcc23807536bee418f142d19fa0d21bb0cff7` |
| Hospital A submitter | `0x0376aac07ad725e01357b1725b5cec61ae10473c` |
| Payer B submitter | `0xb040e0faac56886b0f29af446544aed0a154ed29` |
| Staffing Agency C submitter | `0xf5a5e415061470a8b9137959180901aea72450a4` |

These accounts are not Besu validators, application users, staff identities, or
professional authorities. Their private keys and funded balances are demo-only,
public test credentials in `infra/besu/genesis.json` and this document. Deactivation
in a registry disables its submitter. Production requires secure custody, rotation,
delegation, and governance that are intentionally absent here.

## Evidence and privacy boundary

HealthTrust commitments are backend-produced SHA-256 values accepted as `bytes32`.
Contracts never parse JSON or reconstruct off-chain evidence. Solidity uses keccak256
only for internal response-stream mapping keys; this does not alter evidence hashes.

Storage, calldata, constructor arguments, custom errors, and emitted events contain
only pseudonymous fixed-width identifiers, commitments, enum states, lineage IDs,
domain timestamps, ledger timestamps, addresses, and contract dependencies. They do
not contain provider names or real identifiers, policy text, notes, explanations,
documents, PHI, staff names, or application-user identities. Run
`scripts/privacy-check.sh` to inspect the generated public ABI/event surface.

`effectiveTime` and `receiptTimestamp` are caller-supplied domain timestamps.
`recordedAt` is the block timestamp and represents only ledger recording time.

## Local Besu deployment

Contract deployment is explicit and is never part of application startup. Reset and
start the RC2 demo chain, then deploy:

```bash
docker compose -f docker-compose.rc2.yml down --volumes
docker compose -f docker-compose.rc2.yml up -d --wait
./contracts/scripts/deploy-local.sh
```

The deploy script requires the demo deployer nonce to be zero, broadcasts all four
deployments plus registry bootstrap transactions, and prints addresses and transaction
hashes from Foundry's broadcast receipt. Override `DEMO_DEPLOYER_PRIVATE_KEY`,
`RPC_URL`, or `HOST_RPC_URL` only for an intentionally configured environment.

## Append-only guarantees and limitations

Immutable version IDs reject duplicates. Successors must reference the current head,
which prevents branches from stale writes. Historical structs and history arrays are
never rewritten; only current-head pointers advance. Complete response snapshots must
preserve accumulated receipt, policy, and decision commitments.

A ledger commitment proves only that an authorized demo account recorded particular
evidence bytes at a chain position. It does not prove source truth, real-world identity,
policy correctness, action performance, compliance, organizational independence, or
production privacy/security. Current activation flags and head pointers are mutable
indexes over immutable registration/version history; emitted events preserve changes.
