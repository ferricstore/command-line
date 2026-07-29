#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

usage() {
  echo "usage: $0 <release-tag> [ferricstore-sdk-version]" >&2
}

if [[ $# -lt 1 || $# -gt 2 || -z "${1:-}" ]]; then
  usage
  exit 2
fi

release_tag="$1"
sdk_version="${2:-}"
version_file="${FERRICSTORE_VERSION_FILE:-FERRICSTORE_VERSION}"

if [[ ! -s "$version_file" ]]; then
  echo "$version_file must declare the supported FerricStore OSS release" >&2
  exit 1
fi
oss_version="$(tr -d '[:space:]' <"$version_file")"

if [[ -z "$sdk_version" ]]; then
  sdk_version="$(go list -m -f '{{.Version}}' github.com/ferricstore/ferricstore-go)"
fi

if [[ "$sdk_version" != "$oss_version" ]]; then
  echo "pinned FerricStore SDK $sdk_version must match FerricStore OSS $oss_version" >&2
  echo "update the SDK dependency before releasing this FerricStore line" >&2
  exit 1
fi

if [[ "$release_tag" != "$oss_version" ]]; then
  echo "release tag $release_tag must match FerricStore OSS $oss_version" >&2
  echo "update the pinned SDK and compatibility documentation before releasing a new FerricStore line" >&2
  exit 1
fi

echo "release tag $release_tag matches FerricStore OSS/SDK $oss_version"
