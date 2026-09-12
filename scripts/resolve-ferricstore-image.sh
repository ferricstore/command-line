#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -n "${FERRICSTORE_IMAGE:-}" ]]; then
  printf '%s\n' "$FERRICSTORE_IMAGE"
  exit 0
fi

image_repository="${FERRICSTORE_IMAGE_REPOSITORY:-quay.io/ferricstore/ferricstore}"
version_file="${FERRICSTORE_IMAGE_VERSION_FILE:-FERRICSTORE_IMAGE_VERSION}"
digest_file="${FERRICSTORE_IMAGE_DIGEST_FILE:-FERRICSTORE_IMAGE_DIGEST}"

if [[ ! -s "$version_file" ]]; then
  echo "$version_file must declare the OSS integration image version" >&2
  exit 1
fi
image_version="$(tr -d '[:space:]' <"$version_file")"
if [[ ! "$image_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "$version_file must contain one semantic image version" >&2
  exit 1
fi

if [[ ! -s "$digest_file" ]]; then
  echo "$digest_file must pin the OSS integration image" >&2
  exit 1
fi
image_digest="$(tr -d '[:space:]' <"$digest_file")"
if [[ ! "$image_digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo "$digest_file must contain one sha256 image digest" >&2
  exit 1
fi

resolved_digest="$(docker buildx imagetools inspect "$image_repository:$image_version" | awk '$1 == "Digest:" { print $2; exit }')"
if [[ -z "$resolved_digest" || "$resolved_digest" != "$image_digest" ]]; then
  echo "$image_repository:$image_version resolves to ${resolved_digest:-no digest}, expected $image_digest" >&2
  echo "update FERRICSTORE_IMAGE_VERSION and FERRICSTORE_IMAGE_DIGEST together" >&2
  exit 1
fi

printf '%s@%s\n' "$image_repository" "$image_digest"
