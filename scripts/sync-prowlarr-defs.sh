#!/usr/bin/env bash
# Downloads all Prowlarr v11 Cardigann definitions into the bundled definitions directory.
set -euo pipefail

DEST="internal/indexers/cardigann/definitions"
REPO_URL="https://github.com/Prowlarr/Indexers/archive/refs/heads/master.tar.gz"

echo "Downloading Prowlarr indexer definitions..."
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -sL "$REPO_URL" | tar xz -C "$TMP"

# Remove existing definitions (they'll be replaced by Prowlarr's)
echo "Clearing existing definitions..."
rm -f "$DEST"/*.yml

# Copy v11 definitions
cp "$TMP"/Indexers-master/definitions/v11/*.yml "$DEST/"

echo "Copied $(ls "$DEST"/*.yml | wc -l | tr -d ' ') definitions"
