#!/usr/bin/env bash
set -euo pipefail

since="2m"

if [[ ${1:-} == "--since" ]]; then
  if [[ -z ${2:-} ]]; then
    echo "Usage: $0 [--since DURATION]" >&2
    exit 2
  fi
  since=$2
  shift 2
fi

if [[ $# -ne 0 ]]; then
  echo "Usage: $0 [--since DURATION]" >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/.." && pwd)
cd "$repo_root"

docker compose logs --since "$since" --no-log-prefix hospital-b-node hospital-a-node |
  jq -rs '
    def visual_short($width):
      tostring as $value
      | if ($value | length) > $width
        then $value[0:($width - 1)] + "…"
        else $value
        end;

    def clock:
      (.time | capture("T(?<value>[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{3})").value);

    def node:
      .node_id | sub("-node$"; "");

    def detail:
      if .msg == "clinical_record_persisted" then
        "record=\(.record_id | visual_short(18))  state=\(.commit_state)"
      elif .msg == "transaction_accepted" then
        "event=\(.event_type)  tx=\(.transaction_id | visual_short(30))"
      elif .msg == "block_created" or .msg == "block_received" then
        "height=\(.height)  block=\(.block_hash[0:12])"
      elif .msg == "block_accepted" then
        "height=\(.height)  block=\(.block_hash[0:12])  state=\(.validation)"
      else
        "record=\(.record_id | visual_short(18))  state=\(.commit_state)"
      end;

    map(select(
      .msg == "clinical_record_persisted" or
      .msg == "transaction_accepted" or
      .msg == "block_created" or
      .msg == "block_received" or
      .msg == "block_accepted" or
      .msg == "clinical_record_created"
    ))
    | sort_by(.time)
    | if length == 0 then
        "HealthTrust Exchange — Blockchain Commit Trace\n\nNo commit events found in the selected time window."
      else
        "HealthTrust Exchange — Blockchain Commit Trace\n\n" +
        (map(
          "\(clock)  \(node)  \(.msg)\n" +
          "              \(detail)"
        ) | join("\n\n"))
      end
  '
