#!/usr/bin/env bash
# Downloads the customer table pg_dump from the tpch-dummy-data-export release.
set -euo pipefail

RELEASE_URL="https://github.com/zhalok/tpch-dummy-data-export/releases/download/customer_pgdump/customer_dump.sql.gz"
OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_FILE="${OUT_DIR}/customer_dump.sql.gz"
SQL_FILE="${OUT_DIR}/customer_dump.sql"

echo "Downloading ${RELEASE_URL}"
curl -fL --progress-bar -o "${OUT_FILE}" "${RELEASE_URL}"

echo "Saved to ${OUT_FILE}"

echo "Decompressing ${OUT_FILE}"
gunzip -fk "${OUT_FILE}"

echo "Saved to ${SQL_FILE}"
