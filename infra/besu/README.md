# RC2 Besu QBFT consortium

This directory defines the deterministic local validator-node infrastructure for
HealthTrust Exchange RC2. It does not connect the RC1 application to Besu and it
does not implement transaction signing, contracts, or authority workflows.

## Architecture

The demo has exactly three QBFT validators:

| Demo consortium organization | Besu service | Validator address |
| --- | --- | --- |
| Hospital A | `besu-hospital-validator` | `0x00a44a018a4978be20038d45f2667fd580a48802` |
| Payer B | `besu-payer-validator` | `0x2eb9243c96d8f7e24521491ff831908023145564` |
| Staffing Agency C | `besu-staffing-validator` | `0x41b1a80e8eabf5e2d764d7180209f2cc09c6d08a` |

This one-to-one mapping is a demonstration topology, not a requirement of the
HealthTrust domain model. A consortium organization, Besu validator node,
transaction signer, application user, and authority source are separate
identities. Phase 2 implements only the validator nodes.

All peers run on the isolated `besu-consortium` Docker network. Fixed private IP
addresses make the permitted enode URLs deterministic. Discovery is disabled;
the three predetermined nodes connect through `static-nodes.json` and no manual
post-start admission is required.

The RC2 network lives in `docker-compose.rc2.yml` rather than the root RC1
Compose file. This keeps RC1's application, PostgreSQL databases, and custom
blockchain runtime unchanged while later RC2 phases are developed.

## Pinned Besu version

The network pins `hyperledger/besu:25.12.0` (image digest observed during Phase
2 validation: `sha256:4ef9e934bb321d916ff245981fd65100dc41446b26b2bed47a2b7d09139a3cbe`).
This stable release supports the QBFT genesis fields, QBFT JSON-RPC namespace,
static peers, and file-based node permissioning used here. Pinning avoids
unreviewed behavior changes from `latest`; no external Ethereum tools are added.

## QBFT genesis parameters

| Parameter | Value | Purpose |
| --- | --- | --- |
| Chain/network ID | `202603` (`0x3176b`) | Local RC2 identity; not a public chain |
| Consensus | QBFT | Deterministic proof-of-authority validator set |
| Validators | 3, encoded in `extraData` | Predetermined RC2 demo membership |
| Block period | 2 seconds | Responsive local demonstration |
| Request timeout | 4 seconds | Simple value above the block period |
| Epoch length | 30,000 blocks | Besu's uncomplicated reference value |
| Gas limit | `0x1fffffffffffff` | Ample local contract-development capacity |
| Base fee | zero | Gas accounting without local token economics |
| Allocations | none | No funded accounts or cryptocurrency behavior |

There is no mining, public-chain connection, token, or external RPC dependency.
The empty `alloc` object is deliberate. Future contract deployment will require
a separately defined demo transaction signer and funding policy; validator keys
must not be reused for that purpose.

## Deterministic demo keys

The `nodes/*/key` and `key.pub` files are **DEMO ONLY** validator-node identities.
They were generated once with Besu 25.12.0 and are committed alongside the
genesis file so every checkout gets the same topology and validator addresses.
They are intentionally public, provide no production key-management guarantees,
and must never control real assets or production infrastructure. Production
would require protected keys, rotation, independent custody, and organizational
admission/governance outside this reference implementation.

## Permissioning

Local file-based node permissioning enables only the three enodes listed in
`permissions_config.toml`. Discovery is disabled and the same set is used as
static peers. Validator membership is separately fixed in the genesis QBFT
`extraData`. Account permissioning and contract-based permissioning are not
implemented because Phase 2 creates no transaction signers or governance
contracts.

This is deterministic demo admission, not production-grade consortium
governance. Peer discovery is disabled, node connections are allowlisted, and
host RPC ports bind only to `127.0.0.1`. The Docker bridge is not marked
`internal` because Docker Desktop for macOS does not publish host ports from an
internal-only network.

## Ports

| Validator | Host HTTP JSON-RPC | Container P2P | Fixed container IP |
| --- | --- | --- | --- |
| Hospital | `127.0.0.1:8545` | `30303` (not published) | `172.28.250.11` |
| Payer | `127.0.0.1:8546` | `30303` (not published) | `172.28.250.12` |
| Staffing | `127.0.0.1:8547` | `30303` (not published) | `172.28.250.13` |

Only `ETH`, `NET`, `WEB3`, and `QBFT` JSON-RPC APIs are enabled. Admin, debug,
miner, account-management, WebSocket, GraphQL, and Engine APIs are not exposed.
RPC has no authentication or TLS and is suitable only for loopback development.

## Operate the network

Run commands from the repository root.

Start and wait for health:

```bash
docker compose -f docker-compose.rc2.yml up -d --wait
```

Validate the running network (requires Docker, `curl`, and Python 3):

```bash
./infra/besu/smoke-test.sh
```

View logs:

```bash
docker compose -f docker-compose.rc2.yml logs -f
```

Stop containers while preserving their chain data:

```bash
docker compose -f docker-compose.rc2.yml down
```

Reset the demo chain by explicitly removing only RC2 Compose volumes:

```bash
docker compose -f docker-compose.rc2.yml down --volumes
```

The reset is destructive to local RC2 chain history. It does not touch RC1's
PostgreSQL volumes because the projects and volume names are separate.

## Failure behavior

QBFT with three validators requires a two-validator quorum. If one validator is
temporarily stopped, the other two can continue producing blocks. Restarting the
stopped validator reconnects it through the static peer set and synchronizes it
to the shared head.

To observe this with the staffing validator:

```bash
docker compose -f docker-compose.rc2.yml stop besu-staffing-validator
# Query Hospital twice and observe eth_blockNumber increase.
docker compose -f docker-compose.rc2.yml start besu-staffing-validator
./infra/besu/smoke-test.sh
```

If two validators stop, the remaining validator cannot reach quorum: RPC remains
available but block production stalls. This demo shows normal quorum behavior;
it does not prove Byzantine resilience, production availability, secure
organizational governance, or recovery from arbitrary faults.

## Known limitations and production non-claims

- Validator identities and fixed IPs are public, local-demo material.
- Permissioning is a shared local file, not decentralized admission governance.
- RPC is unauthenticated HTTP bound to the host loopback interface.
- The network has no contracts, transaction signer, application adapter, or
  authority-ledger events yet.
- Persistent Docker volumes are convenient local state, not backup or disaster
  recovery.
- Three validators demonstrate quorum but are too few for a production network.
- The organizational mapping is illustrative and does not constrain the domain.
- Zero base fee removes demo friction; it does not define production economics.

The network therefore demonstrates a genuine local QBFT consortium topology and
consensus only. It makes no claim that the current healthcare workflow is stored
on Besu until later phases implement and verify that integration.
