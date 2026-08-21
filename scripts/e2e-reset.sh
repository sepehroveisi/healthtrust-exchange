#!/usr/bin/env sh
set -eu

if [ "${HEALTHTRUST_E2E_RESET:-}" != "1" ]; then
  echo "Refusing to reset persistent data. Set HEALTHTRUST_E2E_RESET=1 for an explicit E2E reset." >&2
  exit 2
fi

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"

docker compose down --volumes --remove-orphans
docker compose build hospital-a-node hospital-b-node web
docker compose up --detach --wait
