# RC2 Besu ledger adapter

The `ledger` package is the implementation-independent RC2 contract boundary. The
`besu` package signs EVM transactions, validates chain identity, invokes the four
Phase 3 contracts, waits for successful receipts, and provides typed reads.

The adapter uses go-ethereum v1.15.11 and small, reviewable ABI fragments containing
only the methods it invokes. This avoids a JavaScript generator stack. When a
contract method changes, update the fragment directly from the corresponding
`forge inspect <Contract> abi` result and run the real integration test; the adapter
does not treat event logs as authoritative state.

`Config` requires an RPC URL, expected chain ID, all four deployed addresses, and a
signer key. Contract addresses are not universal constants. Private keys come from
configuration and are never logged. The checked-in integration test uses only the
public deterministic `DEMO ONLY` Phase 3 accounts, which are distinct from validator
identities and are unsuitable for assets or production.

Canonical identifiers allow up to 128 bytes, but Phase 3 contract identifiers are
`bytes32`. The ledger boundary therefore accepts only 1–32 byte identifiers and
rejects longer values; it never truncates or implicitly hashes an identifier.

For a fresh integration run:

```bash
docker compose -f docker-compose.rc2.yml down --volumes
docker compose -f docker-compose.rc2.yml up -d --wait
./contracts/scripts/deploy-local.sh
go test -tags=integration -v ./internal/ledger/besu
docker compose -f docker-compose.rc2.yml down
```

A returned `ledger.Transaction` is confirmed by a successful receipt and includes
its transaction hash, block number, and block hash. Submission errors, contract
reverts, chain mismatch, transport failure, and receipt timeout have distinct error
categories. This local QBFT confirmation is not a stronger production-finality
claim. Phase 4 provides no PostgreSQL reconciliation, workflow API, production key
management, or business-policy implementation.
