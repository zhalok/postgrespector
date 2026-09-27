#!/usr/bin/env bash
# Seeds the customer dump into the postgres container.
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-postgres}"

OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="${OUT_DIR}/customer_dump.sql"

if [ ! -f "${SQL_FILE}" ]; then
  echo "customer_dump.sql not found, downloading it first"
  "${OUT_DIR}/download-customer-dump.sh"
fi

if ! docker inspect -f '{{.State.Running}}' "${CONTAINER_NAME}" 2>/dev/null | grep -q true; then
  echo "Container '${CONTAINER_NAME}' is not running. Start it with 'docker compose up -d postgres' first." >&2
  exit 1
fi

echo "Seeding ${SQL_FILE} into ${CONTAINER_NAME}/${PGDATABASE}"
docker exec -i "${CONTAINER_NAME}" psql -U "${PGUSER}" -d "${PGDATABASE}" < "${SQL_FILE}"

echo "Seed complete"
