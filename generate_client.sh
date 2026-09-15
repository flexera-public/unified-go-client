#!/usr/bin/env bash
# Regenerate the split client_gen_*.go files from the unified OpenAPI spec.
# Run from the repo root (called via `go generate .` or `make generate-client`).
#
# The unified-openapi submodule is consumed at its checked-in commit. This
# script never fetches upstream specs or regenerates unified-openapi/openapi3.json.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_FILE="$SCRIPT_DIR/unified-openapi/openapi3.json"
SPEC_CONFIG="$SCRIPT_DIR/unified-openapi/generator/specs.yaml"
TEMP_CLIENT="$SCRIPT_DIR/.client_gen.tmp.go"

cleanup() {
  rm -f "$TEMP_CLIENT"
}
trap cleanup EXIT

# Ensure the committed submodule revision is available locally.
git submodule update --init --recursive unified-openapi

if [ ! -f "$SPEC_FILE" ]; then
  echo "Error: committed spec not found at $SPEC_FILE" >&2
  exit 1
fi

echo "→ Generating Go client from $SPEC_FILE"
rm -f "$TEMP_CLIENT"
go tool oapi-codegen -config "$SCRIPT_DIR/config.yaml" "$SPEC_FILE"

echo "→ Splitting generated client by source spec and concern"
go run "$SCRIPT_DIR/cmd/split-client" \
  --input "$TEMP_CLIENT" \
  --output-dir "$SCRIPT_DIR" \
  --spec "$SPEC_FILE" \
  --spec-config "$SPEC_CONFIG"
rm -f "$SCRIPT_DIR/client_gen.go"

echo "✓ split client regenerated"
