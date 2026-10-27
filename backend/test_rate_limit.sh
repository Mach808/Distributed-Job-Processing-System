#!/usr/bin/env bash

set -u

API_KEY="${1:?Usage: bash ./test_rate_limit.sh <api-key> [count] [base-url]}"
COUNT="${2:-40}"
BASE_URL="${3:-http://localhost:8080}"

if ! [[ "$COUNT" =~ ^[1-9][0-9]*$ ]]; then
  echo "count must be a positive integer" >&2
  exit 1
fi

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

pids=()
for ((number = 1; number <= COUNT; number++)); do
  (
    curl --silent --show-error --output "$TEMP_DIR/body-$number" \
      --write-out '%{http_code}\n' \
      --request POST "$BASE_URL/jobs" \
      --header "X-API-Key: $API_KEY" \
      --header 'Content-Type: application/json' \
      --data "{\"type\":\"addition\",\"a\":$number,\"b\":1}" \
      > "$TEMP_DIR/status-$number"
  ) &
  pids+=("$!")
done

for pid in "${pids[@]}"; do
  wait "$pid"
done

echo "Status summary:"
cat "$TEMP_DIR"/status-* | sort | uniq -c | sort -n
echo
echo "Total requests: $COUNT"
echo "Accepted (201): $(grep -l '^201$' "$TEMP_DIR"/status-* | wc -l | tr -d ' ')"
echo "Rate limited (429): $(grep -l '^429$' "$TEMP_DIR"/status-* | wc -l | tr -d ' ')"
echo "Other responses: $(grep -L -E '^(201|429)$' "$TEMP_DIR"/status-* | wc -l | tr -d ' ')"

echo
echo "Non-success responses:"
for status_file in "$TEMP_DIR"/status-*; do
  status="$(cat "$status_file")"
  if [[ "$status" != "201" ]]; then
    number="${status_file##*-}"
    echo "request $number: HTTP $status - $(cat "$TEMP_DIR/body-$number")"
  fi
done
