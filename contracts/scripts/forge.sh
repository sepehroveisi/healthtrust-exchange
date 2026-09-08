#!/usr/bin/env bash
set -euo pipefail

CONTRACTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

docker run --rm --entrypoint forge \
  --add-host host.docker.internal:host-gateway \
  --env DEMO_DEPLOYER_PRIVATE_KEY \
  --volume "$CONTRACTS_DIR:/work" \
  --workdir /work \
  ghcr.io/foundry-rs/foundry:v1.8.1 "$@"
