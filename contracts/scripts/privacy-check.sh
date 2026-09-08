#!/usr/bin/env bash
set -euo pipefail

CONTRACTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$CONTRACTS_DIR/scripts/forge.sh" build >/dev/null

python3 - "$CONTRACTS_DIR/out" <<'PY'
import json
import pathlib
import sys

forbidden = {
    "providerName",
    "providerIdentifier",
    "policyText",
    "reviewNotes",
    "decisionExplanation",
    "actionExplanation",
    "supportingDocument",
    "supportingDocumentFilename",
    "patientData",
    "phi",
    "staffName",
    "applicationUserId",
}
contracts = {
    "OrganizationRegistry.sol/OrganizationRegistry.json",
    "AuthorityRegistry.sol/AuthorityRegistry.json",
    "AuthorityEventRegistry.sol/AuthorityEventRegistry.json",
    "ResponseLedger.sol/ResponseLedger.json",
}
observed = set()
for relative in contracts:
    artifact = pathlib.Path(sys.argv[1], relative)
    data = json.loads(artifact.read_text(encoding="utf-8"))
    for entry in data["abi"]:
        for item in entry.get("inputs", []) + entry.get("outputs", []):
            observed.add(item.get("name", ""))
            observed.update(component.get("name", "") for component in item.get("components", []))
leaked = sorted(forbidden & observed)
if leaked:
    raise SystemExit("Forbidden ABI fields: " + ", ".join(leaked))
print("PASS: contract ABI and event fields contain no prohibited operational-detail names")
PY
