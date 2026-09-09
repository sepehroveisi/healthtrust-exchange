#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/docker-compose.rc2.yml" --profile demo-api)
API="http://127.0.0.1:8090"

json_assert() {
  python3 -c 'import json,sys; data=json.load(sys.stdin); assert eval(sys.argv[1], {"__builtins__": {"len": len, "all": all}}, {"data": data}), data' "$1"
}

counts() {
  "${COMPOSE[@]}" exec -T rc2-postgres psql -qAt -U healthtrust_rc2 -d healthtrust_rc2 \
    -c "SELECT (SELECT count(*) FROM authority_ledger.authority_assertions) || ':' || (SELECT count(*) FROM authority_ledger.response_versions) || ':' || (SELECT count(*) FROM authority_ledger.ledger_submissions) || ':' || (SELECT coalesce(sum(attempt_count),0) FROM authority_ledger.ledger_submissions);"
}

printf 'Resetting disposable RC2 demo state...\n'
"${COMPOSE[@]}" down --volumes --remove-orphans
"${COMPOSE[@]}" up -d --wait rc2-postgres besu-hospital-validator besu-payer-validator besu-staffing-validator
"$ROOT/contracts/scripts/deploy-local.sh"

printf 'Starting with controlled mutation disabled...\n'
HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED=false "${COMPOSE[@]}" up -d --build --wait rc2-demo-api
curl --fail --silent "$API/readyz" | json_assert 'data["status"] == "ready"'
curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8" | json_assert 'data["ID"] == "EVENT-PHASE8" and data["Ledger"]["state"] == "CONFIRMED"'
curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/responses" | json_assert 'len(data) == 3 and all(item["State"] == "ACTION_COMPLETED" and item["Ledger"]["state"] == "CONFIRMED" for item in data)'
curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/verification/bundle" | json_assert 'data["status"] == "VERIFIED" and len(data["responses"]) == 3'
disabled_code="$(curl --silent --output /tmp/healthtrust-rc2-disabled.json --write-out '%{http_code}' -X POST -H 'Content-Type: application/json' --data '{"target":"HOSPITAL_A_POLICY"}' "$API/api/rc2/demo/tamper")"
test "$disabled_code" = "403"

clean_counts="$(counts)"
test "$clean_counts" = "1:12:13:13"
"${COMPOSE[@]}" restart rc2-demo-api
"${COMPOSE[@]}" up -d --wait rc2-demo-api
test "$(counts)" = "$clean_counts"
curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/verification/bundle" | json_assert 'data["status"] == "VERIFIED"'

printf 'Recreating only the API with controlled mutation explicitly enabled...\n'
HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED=true "${COMPOSE[@]}" up -d --force-recreate --wait rc2-demo-api
before_response="$(curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/responses/HOSPITAL-A")"
curl --fail --silent -X POST -H 'Content-Type: application/json' --data '{"target":"HOSPITAL_A_POLICY"}' "$API/api/rc2/demo/tamper" | json_assert 'data["before"]["status"] == "VERIFIED" and data["after"]["status"] == "FAILED" and data["ledgerUnchanged"] is True'
after_response="$(curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/responses/HOSPITAL-A")"
BEFORE_RESPONSE="$before_response" AFTER_RESPONSE="$after_response" python3 -c 'import json,os; before=json.loads(os.environ["BEFORE_RESPONSE"]); after=json.loads(os.environ["AFTER_RESPONSE"]); assert before["Ledger"] == after["Ledger"]'
curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/verification/bundle" | json_assert 'data["status"] == "FAILED" and data["authority"]["status"] == "VERIFIED" and {x["organizationId"]: x["result"]["status"] for x in data["responses"]} == {"HOSPITAL-A":"FAILED","PAYER-B":"VERIFIED","STAFFING-AGENCY-C":"VERIFIED"}'
test "$(counts)" = "$clean_counts"

printf 'Restarting in the failed state...\n'
HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED=true "${COMPOSE[@]}" restart rc2-demo-api
HEALTHTRUST_RC2_DEMO_TAMPER_ENABLED=true "${COMPOSE[@]}" up -d --wait rc2-demo-api
curl --fail --silent "$API/api/rc2/authority-events/EVENT-PHASE8/verification/bundle" | json_assert 'data["status"] == "FAILED" and {x["organizationId"]: x["result"]["status"] for x in data["responses"]}["HOSPITAL-A"] == "FAILED"'
test "$(counts)" = "$clean_counts"

printf 'RC2 runtime smoke PASS counts=%s\n' "$clean_counts"
