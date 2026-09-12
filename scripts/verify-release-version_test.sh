#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

script="./scripts/verify-release-version.sh"
version_file="FERRICSTORE_VERSION"

if [[ ! -s "$version_file" ]]; then
  echo "$version_file must declare the supported FerricStore OSS release" >&2
  exit 1
fi
oss_version="$(tr -d '[:space:]' <"$version_file")"

success_output="$("$script" "$oss_version" "$oss_version")"
if [[ "$success_output" != *"$oss_version matches FerricStore OSS/SDK $oss_version"* ]]; then
  echo "unexpected success output: $success_output" >&2
  exit 1
fi

if command -v go >/dev/null 2>&1; then
  "$script" "$oss_version" >/dev/null
fi

failure_output="$(mktemp)"
bad_digest_file="$(mktemp)"
trap 'rm -f "$failure_output" "$bad_digest_file"' EXIT

if "$script" v9.9.9 "$oss_version" >"$failure_output" 2>&1; then
  echo "mismatched release and SDK versions unexpectedly passed" >&2
  exit 1
fi
if ! grep -Fq "release tag v9.9.9 must match FerricStore OSS $oss_version" "$failure_output"; then
  echo "mismatch error did not explain the required version relationship" >&2
  cat "$failure_output" >&2
  exit 1
fi

if "$script" "$oss_version" v9.9.9 >"$failure_output" 2>&1; then
  echo "mismatched SDK and OSS versions unexpectedly passed" >&2
  exit 1
fi
if ! grep -Fq "pinned FerricStore SDK v9.9.9 must match FerricStore OSS $oss_version" "$failure_output"; then
  echo "SDK mismatch error did not explain the required version relationship" >&2
  cat "$failure_output" >&2
  exit 1
fi

if "$script" "" "$oss_version" >"$failure_output" 2>&1; then
  echo "empty release tag unexpectedly passed" >&2
  exit 1
fi
if ! grep -Fq "usage:" "$failure_output"; then
  echo "empty-tag error did not print usage" >&2
  cat "$failure_output" >&2
  exit 1
fi

printf '%s\n' 'sha256:not-a-real-digest' >"$bad_digest_file"
if FERRICSTORE_IMAGE_DIGEST_FILE="$bad_digest_file" "$script" "$oss_version" "$oss_version" >"$failure_output" 2>&1; then
  echo "invalid OSS image digest unexpectedly passed" >&2
  exit 1
fi
if ! grep -Fq "must contain one sha256 image digest" "$failure_output"; then
  echo "invalid image digest error was not actionable" >&2
  cat "$failure_output" >&2
  exit 1
fi
