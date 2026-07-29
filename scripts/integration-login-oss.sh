#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

oss_version="$(tr -d '[:space:]' <FERRICSTORE_VERSION)"
image="${FERRICSTORE_IMAGE:-ghcr.io/ferricstore/ferricstore:${oss_version#v}@sha256:ee49d39e3b15cd6298537a88818647e71bcfc7571921e88bcf3a201311c690fc}"
suffix="$$-$RANDOM"
bootstrap_name="ferric-command-line-login-bootstrap-$suffix"
server_name="ferric-command-line-login-$suffix"
volume_name="ferric-command-line-login-$suffix"
node_hostname="ferric-command-line-login-$suffix"
ready_log="$(mktemp)"
binary_dir="$(mktemp -d)"
binary_path="$binary_dir/ferric"
container_image="ferric-command-line-oss-integration:$suffix"
export FERRIC_CONFIG_DIR="$binary_dir/config"
unset FERRIC_PROFILE
unset FERRIC_URL FERRIC_USERNAME FERRIC_PASSWORD FERRIC_PASSWORD_FILE
unset FERRIC_CONTROL_URL FERRIC_ORGANIZATION FERRIC_CLUSTER
unset FERRIC_API_TOKEN FERRIC_API_TOKEN_FILE

cleanup() {
  local status=$?
  if [[ "$status" != 0 ]]; then
    docker logs --tail 200 "$bootstrap_name" >&2 2>/dev/null || true
    docker logs --tail 200 "$server_name" >&2 2>/dev/null || true
  fi
  docker rm -f "$bootstrap_name" "$server_name" >/dev/null 2>&1 || true
  docker volume rm -f "$volume_name" >/dev/null 2>&1 || true
  docker image rm -f "$container_image" >/dev/null 2>&1 || true
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
  local package="$1"
  local pattern="$2"
  for _ in $(seq 1 60); do
    if run_go_test -tags=integration -count=1 -run "$pattern" "$package" >"$ready_log" 2>&1; then
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
wait_for_test ./internal/auth '^TestIntegrationOSSBootstrap$'
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
wait_for_test ./internal/auth '^TestIntegrationOSSLogin$'
export FERRICSTORE_OSS_CLI_TEST=1
run_go_test -tags=integration -count=1 -run '^TestIntegrationOSSWorkflowQueryAndSchedule$' ./internal/cli

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

environment_status="$(FERRIC_URL="ferric://$FERRICSTORE_OSS_ADDR" FERRIC_USERNAME="$FERRICSTORE_OSS_USERNAME" FERRIC_PASSWORD="$FERRICSTORE_OSS_PASSWORD" "$binary_path" auth status)"
if [[ "$environment_status" != *"Authenticated"* || "$environment_status" != *"Source: environment"* ]]; then
  echo "environment authentication returned unexpected status: $environment_status" >&2
  exit 1
fi
if [[ "$environment_status" == *"$FERRICSTORE_OSS_PASSWORD"* ]]; then
  echo "environment authentication status exposed the password" >&2
  exit 1
fi

password_file="$binary_dir/oss-password"
umask 077
printf '%s\n' "$FERRICSTORE_OSS_PASSWORD" >"$password_file"
file_ping="$(FERRIC_URL="ferric://$FERRICSTORE_OSS_ADDR" FERRIC_USERNAME="$FERRICSTORE_OSS_USERNAME" FERRIC_PASSWORD_FILE="$password_file" "$binary_path" server ping)"
if [[ "$file_ping" != "PONG" ]]; then
  echo "secret-file environment authentication returned unexpected PING: $file_ping" >&2
  exit 1
fi

docker build --quiet --build-arg VERSION=integration --tag "$container_image" . >/dev/null

container_status="$(docker run --rm \
  --network "container:$server_name" \
  -e FERRIC_URL=ferric://127.0.0.1:6388 \
  -e "FERRIC_USERNAME=$FERRICSTORE_OSS_USERNAME" \
  -e "FERRIC_PASSWORD=$FERRICSTORE_OSS_PASSWORD" \
  "$container_image" auth status)"
if [[ "$container_status" != *"Authenticated"* || "$container_status" != *"Source: environment"* ]]; then
  echo "container environment authentication returned unexpected status: $container_status" >&2
  exit 1
fi
if [[ "$container_status" == *"$FERRICSTORE_OSS_PASSWORD"* ]]; then
  echo "container authentication status exposed the password" >&2
  exit 1
fi

# The temporary directory remains mode 0700; 0444 lets the image's non-root
# user read only the bind-mounted file through the container runtime.
chmod 0444 "$password_file"
container_file_ping="$(docker run --rm \
  --network "container:$server_name" \
  -e FERRIC_URL=ferric://127.0.0.1:6388 \
  -e "FERRIC_USERNAME=$FERRICSTORE_OSS_USERNAME" \
  -e FERRIC_PASSWORD_FILE=/run/secrets/ferric-password \
  --mount "type=bind,source=$password_file,target=/run/secrets/ferric-password,readonly" \
  "$container_image" server ping)"
if [[ "$container_file_ping" != "PONG" ]]; then
  echo "container secret-file authentication returned unexpected PING: $container_file_ping" >&2
  exit 1
fi
