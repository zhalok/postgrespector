#!/usr/bin/env bash
# Interactively prompts for pgbench params and runs the customer_last_page
# load test inside the postgres container.
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-postgres}"
SCRIPT_PATH="${SCRIPT_PATH:-/pgbench/customer_last_page.sql}"

if ! docker inspect -f '{{.State.Running}}' "${CONTAINER_NAME}" 2>/dev/null | grep -q true; then
  echo "Container '${CONTAINER_NAME}' is not running. Start it with 'docker compose up -d postgres' first." >&2
  exit 1
fi

read -rp "Number of concurrent clients [-c] (default 10): " clients
clients="${clients:-10}"

read -rp "Number of worker threads [-j] (default 2, must be <= clients): " threads
threads="${threads:-2}"

read -rp "Duration in seconds [-T] (default 60): " duration
duration="${duration:-60}"

if [ "${threads}" -gt "${clients}" ]; then
  echo "Number of threads (-j=${threads}) must be <= number of clients (-c=${clients})." >&2
  exit 1
fi

echo
echo "Running: pgbench -U ${PGUSER} -d ${PGDATABASE} -f ${SCRIPT_PATH} -c ${clients} -j ${threads} -T ${duration} --progress=10"
echo

docker exec -it "${CONTAINER_NAME}" pgbench \
  -U "${PGUSER}" -d "${PGDATABASE}" \
  -f "${SCRIPT_PATH}" \
  -c "${clients}" -j "${threads}" -T "${duration}" \
  --progress=10
