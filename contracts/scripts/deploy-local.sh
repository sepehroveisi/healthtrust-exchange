#!/usr/bin/env bash
set -euo pipefail

CONTRACTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEMO_DEPLOYER_PRIVATE_KEY="${DEMO_DEPLOYER_PRIVATE_KEY:-0x000000000000000000000000000000000000000000000000000000000000b007}"
export DEMO_DEPLOYER_PRIVATE_KEY
RPC_URL="${RPC_URL:-http://host.docker.internal:8545}"
HOST_RPC_URL="${HOST_RPC_URL:-http://127.0.0.1:8545}"

nonce="$(curl --fail --silent --show-error \
  --header 'Content-Type: application/json' \
  --data '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0xae39ab287e6e3d2fc48d6e51d1445f3a8106e743","latest"],"id":1}' \
  "$HOST_RPC_URL" | python3 -c 'import json,sys; print(int(json.load(sys.stdin)["result"], 16))')"
if [[ "$nonce" != "0" ]]; then
  printf 'Demo deployer nonce is %s, expected 0. Reset the RC2 demo volumes first.\n' "$nonce" >&2
  exit 1
fi

"$CONTRACTS_DIR/scripts/forge.sh" script script/DeployAuthorityLedger.s.sol \
  --rpc-url "$RPC_URL" --broadcast --slow

python3 - "$CONTRACTS_DIR/broadcast/DeployAuthorityLedger.s.sol/202603/run-latest.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    broadcast = json.load(handle)

print("HealthTrust RC2 contract deployment")
for tx in broadcast["transactions"]:
    if tx.get("transactionType") == "CREATE":
        print(f'{tx["contractName"]}: {tx["contractAddress"]} tx={tx["hash"]}')
PY
