#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

expected_version="0.12.19"
expected_digest="sha256:91364b8a9f1e20fc530154068c9a0dedbf11b2d134b92692b258feeb784eff7d"
if [[ "$(tr -d '[:space:]' <FERRICSTORE_IMAGE_VERSION)" != "$expected_version" ]]; then
  echo "FERRICSTORE_IMAGE_VERSION must track the current OSS integration image" >&2
  exit 1
fi
if [[ "$(tr -d '[:space:]' <FERRICSTORE_IMAGE_DIGEST)" != "$expected_digest" ]]; then
  echo "FERRICSTORE_IMAGE_DIGEST must pin the current OSS integration image" >&2
  exit 1
fi

fake_bin="$(mktemp -d)"
failure_output="$(mktemp)"
trap 'rm -rf "$fake_bin"; rm -f "$failure_output"' EXIT

cat >"$fake_bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$FAKE_DOCKER_CALL_FILE"
printf '%s\n' "Digest: ${FAKE_DOCKER_DIGEST}"
EOF
chmod +x "$fake_bin/docker"

export FAKE_DOCKER_CALL_FILE="$fake_bin/call"
export FAKE_DOCKER_DIGEST="$expected_digest"
resolved="$(PATH="$fake_bin:$PATH" ./scripts/resolve-ferricstore-image.sh)"
if [[ "$resolved" != "quay.io/ferricstore/ferricstore@$expected_digest" ]]; then
  echo "resolved image = $resolved" >&2
  exit 1
fi
if ! grep -Fq "quay.io/ferricstore/ferricstore:$expected_version" "$FAKE_DOCKER_CALL_FILE"; then
  echo "resolver did not verify the expected tag" >&2
  exit 1
fi

override="registry.example.com/ferricstore:test"
if [[ "$(FERRICSTORE_IMAGE="$override" PATH="$fake_bin:$PATH" ./scripts/resolve-ferricstore-image.sh)" != "$override" ]]; then
  echo "explicit FERRICSTORE_IMAGE override was not preserved" >&2
  exit 1
fi

FAKE_DOCKER_DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
if PATH="$fake_bin:$PATH" ./scripts/resolve-ferricstore-image.sh >"$failure_output" 2>&1; then
  echo "resolver accepted a tag whose registry digest changed" >&2
  exit 1
fi
if ! grep -Fq "expected $expected_digest" "$failure_output"; then
  echo "digest mismatch error was not actionable" >&2
  cat "$failure_output" >&2
  exit 1
fi
