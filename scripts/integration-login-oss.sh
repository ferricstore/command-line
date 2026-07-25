#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

image="${FERRICSTORE_IMAGE:-ghcr.io/ferricstore/ferricstore:0.10.3@sha256:f78a6f716cef8a1ef0a36ff620e653f9615bf9ba45abe9d86c990234fb9850d3}"
suffix="$$-$RANDOM"
bootstrap_name="ferric-command-line-login-bootstrap-$suffix"
server_name="ferric-command-line-login-$suffix"
volume_name="ferric-command-line-login-$suffix"
node_hostname="ferric-command-line-login-$suffix"
ready_log="$(mktemp)"
binary_dir="$(mktemp -d)"
binary_path="$binary_dir/ferric"
export FERRIC_CONFIG_DIR="$binary_dir/config"
unset FERRIC_PROFILE

cleanup() {
  local status=$?
  if [[ "$status" != 0 ]]; then
    docker logs --tail 200 "$bootstrap_name" >&2 2>/dev/null || true
    docker logs --tail 200 "$server_name" >&2 2>/dev/null || true
  fi
  docker rm -f "$bootstrap_name" "$server_name" >/dev/null 2>&1 || true
  docker volume rm -f "$volume_name" >/dev/null 2>&1 || true
  rm -f "$ready_log"
  rm -rf "$binary_dir"
}
trap cleanup EXIT

run_go_test() {
  if command -v mise >/dev/null 2>&1; then
    mise exec -- go test "$@"
  else
    go test "$@"
  fi
}

wait_for_test() {
  local pattern="$1"
  for _ in $(seq 1 60); do
    if run_go_test -tags=integration -run "$pattern" ./internal/auth >"$ready_log" 2>&1; then
      return 0
    fi
    sleep 1
  done
  cat "$ready_log" >&2
  return 1
}

docker volume create "$volume_name" >/dev/null
docker run -d --name "$bootstrap_name" --hostname "$node_hostname" -e "FERRICSTORE_NODE_NAME=ferricstore@${node_hostname}" -e FERRICSTORE_PROTECTED_MODE=false -v "$volume_name:/data" -p 127.0.0.1::6388 "$image" >/dev/null

bootstrap_port="$(docker port "$bootstrap_name" 6388/tcp | awk -F: 'NR == 1 {print $NF}')"
if [[ -z "$bootstrap_port" ]]; then
  echo "failed to resolve bootstrap native port" >&2
  exit 1
fi

export FERRICSTORE_OSS_ADDR="127.0.0.1:${bootstrap_port}"
export FERRICSTORE_OSS_USERNAME="default"
export FERRICSTORE_OSS_PASSWORD="cli-oss-password-$suffix"
export FERRICSTORE_OSS_BOOTSTRAP=1
wait_for_test '^TestIntegrationOSSBootstrap$'
unset FERRICSTORE_OSS_BOOTSTRAP

docker stop --time 30 "$bootstrap_name" >/dev/null
docker rm "$bootstrap_name" >/dev/null
docker run -d --name "$server_name" --hostname "$node_hostname" -e "FERRICSTORE_NODE_NAME=ferricstore@${node_hostname}" -e FERRICSTORE_PROTECTED_MODE=true -v "$volume_name:/data" -p 127.0.0.1::6388 "$image" >/dev/null

protected_port="$(docker port "$server_name" 6388/tcp | awk -F: 'NR == 1 {print $NF}')"
if [[ -z "$protected_port" ]]; then
  echo "failed to resolve protected native port" >&2
  exit 1
fi

export FERRICSTORE_OSS_ADDR="127.0.0.1:${protected_port}"
export FERRICSTORE_OSS_LOGIN_TEST=1
wait_for_test '^TestIntegrationOSSLogin$'

if command -v mise >/dev/null 2>&1; then
  mise exec -- go build -o "$binary_path" ./cmd/ferric
else
  go build -o "$binary_path" ./cmd/ferric
fi

login_output="$(printf '%s\n' "$FERRICSTORE_OSS_PASSWORD" | "$binary_path" auth login --url "ferric://$FERRICSTORE_OSS_ADDR" --username "$FERRICSTORE_OSS_USERNAME" --password-stdin --no-store)"
if [[ "$login_output" != *"Authenticated as $FERRICSTORE_OSS_USERNAME"* ]]; then
  echo "CLI login returned unexpected output: $login_output" >&2
  exit 1
fi
if [[ "$login_output" == *"$FERRICSTORE_OSS_PASSWORD"* ]]; then
  echo "CLI login exposed the password" >&2
  exit 1
fi
if printf '%s\n' "definitely-wrong" | "$binary_path" auth login --url "ferric://$FERRICSTORE_OSS_ADDR" --username "$FERRICSTORE_OSS_USERNAME" --password-stdin --no-store >/dev/null 2>&1; then
  echo "CLI login accepted an invalid OSS password" >&2
  exit 1
fi
