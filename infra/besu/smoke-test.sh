#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.rc2.yml" -p healthtrust-rc2)
EXPECTED_CHAIN_ID="0x3176b"
EXPECTED_VALIDATORS=(
  "0x00a44a018a4978be20038d45f2667fd580a48802"
  "0x2eb9243c96d8f7e24521491ff831908023145564"
  "0x41b1a80e8eabf5e2d764d7180209f2cc09c6d08a"
)
NODES=(hospital payer staffing)
PORTS=(8545 8546 8547)
SERVICES=(besu-hospital-validator besu-payer-validator besu-staffing-validator)

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

pass() {
  printf 'PASS: %s\n' "$*"
}

rpc() {
  local port="$1"
  local method="$2"
  local params="${3:-[]}"
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}" \
    "http://127.0.0.1:$port"
}

result() {
  python3 -c 'import json,sys
data=json.load(sys.stdin)
if "error" in data:
    raise SystemExit("RPC error: " + json.dumps(data["error"]))
print(json.dumps(data["result"], separators=(",", ":")) if not isinstance(data["result"], str) else data["result"])'
}

hex_to_dec() {
  python3 -c 'import sys; print(int(sys.stdin.read().strip(), 16))'
}

container_id() {
  "${COMPOSE[@]}" ps -q "$1"
}

printf 'HealthTrust Exchange RC2 — Besu QBFT smoke test\n\n'

for i in "${!NODES[@]}"; do
  cid="$(container_id "${SERVICES[$i]}")"
  [[ -n "$cid" ]] || fail "${SERVICES[$i]} is not running"
  health="$(docker inspect --format '{{.State.Health.Status}}' "$cid")"
  [[ "$health" == "healthy" ]] || fail "${NODES[$i]} health is $health"
done
pass "all three Besu containers are healthy"

for i in "${!NODES[@]}"; do
  peer_hex="$(rpc "${PORTS[$i]}" net_peerCount | result)"
  peer_count="$(printf '%s' "$peer_hex" | hex_to_dec)"
  (( peer_count == 2 )) || fail "${NODES[$i]} has $peer_count peers; expected 2"
done
pass "each validator has exactly two permissioned peers"

expected="$(printf '%s\n' "${EXPECTED_VALIDATORS[@]}" | sort | tr '\n' ' ')"
for i in "${!NODES[@]}"; do
  actual="$(rpc "${PORTS[$i]}" qbft_getValidatorsByBlockNumber '["latest"]' | result | \
    python3 -c 'import json,sys; print(" ".join(sorted(x.lower() for x in json.load(sys.stdin))) + " ")')"
  [[ "$actual" == "$expected" ]] || fail "${NODES[$i]} validator set differs from genesis"
done
pass "all nodes report the expected three-validator set"

for i in "${!NODES[@]}"; do
  chain_id="$(rpc "${PORTS[$i]}" eth_chainId | result)"
  [[ "$chain_id" == "$EXPECTED_CHAIN_ID" ]] || \
    fail "${NODES[$i]} chain ID is $chain_id; expected $EXPECTED_CHAIN_ID"
done
pass "all nodes report chain ID 202603"

height_before="$(rpc 8545 eth_blockNumber | result | hex_to_dec)"
sleep 5
height_after="$(rpc 8545 eth_blockNumber | result | hex_to_dec)"
(( height_after > height_before )) || fail "block height did not progress from $height_before"
pass "block production progressed from height $height_before to $height_after"

converged=false
for _ in {1..20}; do
  heads=()
  for port in "${PORTS[@]}"; do
    heads+=("$(rpc "$port" eth_getBlockByNumber '["latest",false]' | result | \
      python3 -c 'import json,sys; b=json.load(sys.stdin); print(b["number"] + ":" + b["hash"])')")
  done
  if [[ "${heads[0]}" == "${heads[1]}" && "${heads[1]}" == "${heads[2]}" ]]; then
    converged=true
    break
  fi
  sleep 0.25
done
[[ "$converged" == true ]] || fail "nodes did not expose an identical latest head"
pass "all nodes converge on head ${heads[0]}"

printf '\nAll Phase 2 smoke checks passed.\n'
