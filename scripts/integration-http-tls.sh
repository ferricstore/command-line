#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

image="$(./scripts/resolve-ferricstore-image.sh)"
suffix="$$-$RANDOM"
container="ferric-command-line-http-$suffix"
cli_image="ferric-command-line-http-integration:$suffix"
tls_dir="$(mktemp -d "${TMPDIR:-/tmp}/ferric-command-line-http-tls.XXXXXX")"
binary_dir="$(mktemp -d "${TMPDIR:-/tmp}/ferric-command-line-http-bin.XXXXXX")"
username="cli-http"
password="cli-http-secret-$suffix"
restricted_username="cli-http-restricted"
restricted_password="cli-http-restricted-secret-$suffix"

cleanup() {
  status=$?
  if [[ "$status" != 0 ]]; then
    docker logs "$container" >&2 2>/dev/null || true
  fi
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker image rm -f "$cli_image" >/dev/null 2>&1 || true
  case "$tls_dir" in
    "${TMPDIR:-/tmp}"/ferric-command-line-http-tls.*) rm -rf "$tls_dir" ;;
  esac
  case "$binary_dir" in
    "${TMPDIR:-/tmp}"/ferric-command-line-http-bin.*) rm -rf "$binary_dir" ;;
  esac
}
trap cleanup EXIT

for command in docker openssl curl; do
  command -v "$command" >/dev/null || {
    echo "$command is required" >&2
    exit 1
  }
done

run_go() {
  if command -v go >/dev/null 2>&1; then
    go "$@"
  elif command -v mise >/dev/null 2>&1; then
    mise exec -- go "$@"
  else
    echo "go or mise is required" >&2
    return 1
  fi
}

openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj "/CN=FerricStore CLI Test CA" -keyout "$tls_dir/ca.key" -out "$tls_dir/ca.pem" >/dev/null
openssl req -newkey rsa:2048 -nodes -subj "/CN=localhost" -keyout "$tls_dir/server.key" -out "$tls_dir/server.csr" >/dev/null
printf '%s\n' "subjectAltName=DNS:localhost,IP:127.0.0.1" "extendedKeyUsage=serverAuth" >"$tls_dir/extensions.cnf"
openssl x509 -req -in "$tls_dir/server.csr" -CA "$tls_dir/ca.pem" -CAkey "$tls_dir/ca.key" -CAcreateserial -days 1 -out "$tls_dir/server.pem" -extfile "$tls_dir/extensions.cnf" >/dev/null
chmod 700 "$tls_dir"
chmod 600 "$tls_dir/ca.key"
chmod 644 "$tls_dir/ca.pem" "$tls_dir/server.pem" "$tls_dir/server.key"
rm -f "$tls_dir/ca.key" "$tls_dir/ca.srl" "$tls_dir/server.csr" "$tls_dir/extensions.cnf"

docker run --detach --rm --name "$container" \
  -p 127.0.0.1::8080 \
  --mount "type=bind,source=$tls_dir/server.pem,target=/tls/server.pem,readonly" \
  --mount "type=bind,source=$tls_dir/server.key,target=/tls/server.key,readonly" \
  -e FERRICSTORE_PROTECTED_MODE=false \
  -e FERRICSTORE_FLOW_SCHEDULER_ENABLED=false \
  -e FERRICSTORE_HTTP_ENABLED=true \
  -e FERRICSTORE_HTTP_BIND=0.0.0.0 \
  -e FERRICSTORE_HTTP_PORT=8080 \
  -e FERRICSTORE_HTTP2_ENABLED=true \
  -e FERRICSTORE_HTTP_TLS_ENABLED=true \
  -e FERRICSTORE_HTTP_TLS_CERT_FILE=/tls/server.pem \
  -e FERRICSTORE_HTTP_TLS_KEY_FILE=/tls/server.key \
  "$image" >/dev/null

port="$(docker port "$container" 8080/tcp | sed 's/.*://')"
ready=false
for _ in $(seq 1 120); do
  if curl --silent --show-error --cacert "$tls_dir/ca.pem" "https://127.0.0.1:$port/health" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 0.5
done
[[ "$ready" == true ]] || {
  echo "FerricStore HTTPS listener did not become ready" >&2
  exit 1
}

docker exec "$container" bin/ferricstore rpc 'case FerricstoreServer.Acl.set_user("cli-http", ["on", "resetpass", ">'"$password"'", "resetkeys", "+@all", "~*", "&*"]) do :ok -> :ok; other -> raise "ACL bootstrap failed: #{inspect(other)}" end' >/dev/null
docker exec "$container" bin/ferricstore rpc 'case FerricstoreServer.Acl.set_user("cli-http-restricted", ["on", "resetpass", ">'"$restricted_password"'", "resetkeys", "-@all", "+ping", "~*", "&*"]) do :ok -> :ok; other -> raise "restricted ACL bootstrap failed: #{inspect(other)}" end' >/dev/null

export FERRICSTORE_HTTP_CLI_TEST=1
export FERRICSTORE_HTTP_URL="https://127.0.0.1:$port"
export FERRICSTORE_HTTP_CA_FILE="$tls_dir/ca.pem"
export FERRICSTORE_HTTP_USERNAME="$username"
export FERRICSTORE_HTTP_PASSWORD="$password"
export FERRICSTORE_HTTP_RESTRICTED_USERNAME="$restricted_username"
export FERRICSTORE_HTTP_RESTRICTED_PASSWORD="$restricted_password"
run_go test -tags=integration -count=1 -run '^TestIntegrationHTTPUsernamePasswordTLSAndACL$' ./internal/cli

binary="$binary_dir/ferric"
export FERRIC_CONFIG_DIR="$binary_dir/config"
unset FERRIC_PROFILE FERRIC_URL FERRIC_USERNAME FERRIC_PASSWORD FERRIC_PASSWORD_FILE FERRIC_CA_CERT_FILE
run_go build -o "$binary" ./cmd/ferric

login_output="$(printf '%s\n' "$password" | "$binary" auth login --url "$FERRICSTORE_HTTP_URL" --username "$username" --ca-cert "$tls_dir/ca.pem" --password-stdin --no-store)"
if [[ "$login_output" == *"$password"* ]]; then
  echo "HTTPS CLI login exposed the password" >&2
  exit 1
fi
if [[ "$login_output" != *"Authenticated as $username"* ]]; then
  echo "HTTPS CLI login returned unexpected output: $login_output" >&2
  exit 1
fi
if printf '%s\n' "definitely-wrong" | "$binary" auth login --url "$FERRICSTORE_HTTP_URL" --username "$username" --ca-cert "$tls_dir/ca.pem" --password-stdin --no-store >/dev/null 2>&1; then
  echo "HTTPS CLI login accepted an invalid password" >&2
  exit 1
fi
if printf '%s\n' "$password" | "$binary" auth login --url "$FERRICSTORE_HTTP_URL" --username "$username" --password-stdin --no-store >/dev/null 2>&1; then
  echo "HTTPS CLI login trusted the private server without its CA" >&2
  exit 1
fi

key="cli:http:shell:$suffix"
set_output="$(FERRIC_URL="$FERRICSTORE_HTTP_URL" FERRIC_USERNAME="$username" FERRIC_PASSWORD="$password" FERRIC_CA_CERT_FILE="$tls_dir/ca.pem" "$binary" store set "$key" allowed)"
get_output="$(FERRIC_URL="$FERRICSTORE_HTTP_URL" FERRIC_USERNAME="$username" FERRIC_PASSWORD="$password" FERRIC_CA_CERT_FILE="$tls_dir/ca.pem" "$binary" store get "$key")"
if [[ "$set_output" != "OK" || "$get_output" != "allowed" ]]; then
  echo "HTTPS CLI SET/GET returned $set_output/$get_output" >&2
  exit 1
fi
restricted_ping="$(FERRIC_URL="$FERRICSTORE_HTTP_URL" FERRIC_USERNAME="$restricted_username" FERRIC_PASSWORD="$restricted_password" FERRIC_CA_CERT_FILE="$tls_dir/ca.pem" "$binary" server ping)"
if [[ "$restricted_ping" != "PONG" ]]; then
  echo "restricted HTTPS CLI PING returned $restricted_ping" >&2
  exit 1
fi
if FERRIC_URL="$FERRICSTORE_HTTP_URL" FERRIC_USERNAME="$restricted_username" FERRIC_PASSWORD="$restricted_password" FERRIC_CA_CERT_FILE="$tls_dir/ca.pem" "$binary" store set "$key" blocked >/dev/null 2>&1; then
  echo "restricted HTTPS CLI user was allowed to SET" >&2
  exit 1
fi

docker build --quiet --build-arg VERSION=integration --tag "$cli_image" . >/dev/null
container_ping="$(docker run --rm \
  --network "container:$container" \
  -e FERRIC_URL=https://127.0.0.1:8080 \
  -e "FERRIC_USERNAME=$username" \
  -e "FERRIC_PASSWORD=$password" \
  -e FERRIC_CA_CERT_FILE=/run/config/ferric-ca.pem \
  --mount "type=bind,source=$tls_dir/ca.pem,target=/run/config/ferric-ca.pem,readonly" \
  "$cli_image" server ping)"
if [[ "$container_ping" != "PONG" ]]; then
  echo "container HTTPS CLI PING returned $container_ping" >&2
  exit 1
fi
