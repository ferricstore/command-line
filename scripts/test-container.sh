#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

suffix="$$-$RANDOM"
image="ferric-command-line-container-test:$suffix"
version="0.0.0-container-test"
commit="0123456789abcdef"
build_date="2026-01-02T03:04:05Z"

cleanup() {
  docker image rm -f "$image" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker build \
  --build-arg "VERSION=$version" \
  --build-arg "COMMIT=$commit" \
  --build-arg "BUILD_DATE=$build_date" \
  --tag "$image" \
  .

default_output="$(docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges "$image")"
if [[ "$default_output" != *"Manage FerricStore data, FerricFlow queues and workflows"* ]]; then
  echo "container default command did not print CLI help" >&2
  exit 1
fi

version_output="$(docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges "$image" version)"
expected_version="ferric $version (commit $commit, built $build_date)"
if [[ "$version_output" != "$expected_version" ]]; then
  echo "container version mismatch: $version_output" >&2
  exit 1
fi

container_user="$(docker image inspect --format '{{.Config.User}}' "$image")"
if [[ "$container_user" != "65532" && "$container_user" != "65532:65532" ]]; then
  echo "container must run as the distroless non-root user, got: $container_user" >&2
  exit 1
fi

entrypoint="$(docker image inspect --format '{{json .Config.Entrypoint}}' "$image")"
if [[ "$entrypoint" != '["/usr/local/bin/ferric"]' ]]; then
  echo "unexpected container entrypoint: $entrypoint" >&2
  exit 1
fi

source_label="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.source"}}' "$image")"
if [[ "$source_label" != "https://github.com/ferricstore/command-line" ]]; then
  echo "container is missing its repository source label" >&2
  exit 1
fi

version_label="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' "$image")"
if [[ "$version_label" != "$version" ]]; then
  echo "container version label mismatch: $version_label" >&2
  exit 1
fi

set +e
environment_error="$(docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges -e FERRIC_URL=ferric://store:6388 "$image" auth status 2>&1)"
environment_status=$?
set -e
if [[ "$environment_status" == 0 || "$environment_error" != *"FERRIC_USERNAME is required"* ]]; then
  echo "container did not resolve environment credentials before saved profiles: $environment_error" >&2
  exit 1
fi
