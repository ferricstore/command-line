# Container Usage

The official image is a small, one-shot Ferric CLI utility. Its entrypoint is
`ferric`, so arguments after the image name are normal CLI arguments. It does
not run a server or keep a background connection alive.

Release images are published to GitHub Container Registry for Linux amd64 and
arm64. Image tags follow the same FerricStore OSS compatibility version as the
CLI release:

~~~sh
docker run --rm ghcr.io/ferricstore/command-line:v0.11.11 version
docker run --rm ghcr.io/ferricstore/command-line:v0.11.11 server ping
~~~

Use an immutable digest instead of a tag when deployment reproducibility is
required.

## Authentication

Containers should normally use environment credentials rather than
`ferric auth login`. The image is distroless, runs as user 65532, and has no
desktop operating-system keyring.

For a FerricStore service on an existing Docker network:

~~~sh
docker run --rm \
  --network application \
  -e FERRIC_URL=ferrics://ferricstore:6388 \
  -e FERRIC_USERNAME=operator \
  -e FERRIC_PASSWORD_FILE=/run/secrets/ferric-password \
  --mount type=bind,source="$PWD/ferric-password",target=/run/secrets/ferric-password,readonly \
  ghcr.io/ferricstore/command-line:v0.11.11 \
  server ping
~~~

`FERRIC_PASSWORD` is also supported when the runtime injects secrets directly
into the environment. The CLI accepts `ferric://`, `ferrics://`, and
authenticated `https://` endpoints.

For the HTTP API with a private CA, mount the CA separately from the password:

~~~sh
docker run --rm \
  --network application \
  -e FERRIC_URL=https://ferricstore:8080 \
  -e FERRIC_USERNAME=operator \
  -e FERRIC_PASSWORD_FILE=/run/secrets/ferric-password \
  -e FERRIC_CA_CERT_FILE=/run/config/ferric-ca.pem \
  --mount type=bind,source="$PWD/ferric-password",target=/run/secrets/ferric-password,readonly \
  --mount type=bind,source="$PWD/ferric-ca.pem",target=/run/config/ferric-ca.pem,readonly \
  ghcr.io/ferricstore/command-line:v0.11.11 \
  server ping
~~~

Username/password authentication is never sent over plaintext `http://`.

The image supports a read-only root filesystem, all Linux capabilities
dropped, and `no-new-privileges`:

~~~sh
docker run --rm --read-only --cap-drop=ALL \
  --security-opt=no-new-privileges \
  ghcr.io/ferricstore/command-line:v0.11.11 version
~~~

## Kubernetes Utility Pod

For an unauthenticated command such as local help or version output, use the
image like a curl pod:

~~~sh
kubectl run ferric --rm -it --restart=Never \
  --image=ghcr.io/ferricstore/command-line:v0.11.11 \
  -- version
~~~

For an authenticated one-shot command, create a Secret and a short-lived Pod.
This example assumes the Secret has a `password` key:

~~~yaml
apiVersion: v1
kind: Pod
metadata:
  name: ferric
spec:
  automountServiceAccountToken: false
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    runAsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: ferric
      image: ghcr.io/ferricstore/command-line:v0.11.11
      args: ["server", "ping"]
      env:
        - name: FERRIC_URL
          value: ferrics://ferricstore.default.svc.cluster.local:6388
        - name: FERRIC_USERNAME
          value: operator
        - name: FERRIC_PASSWORD_FILE
          value: /run/secrets/ferric/password
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities:
          drop: ["ALL"]
      volumeMounts:
        - name: credentials
          mountPath: /run/secrets/ferric
          readOnly: true
  volumes:
    - name: credentials
      secret:
        secretName: ferric-cli
        defaultMode: 0444
        items:
          - key: password
            path: password
~~~

Read the result with `kubectl logs ferric`, then delete the Pod. Use a Job
instead when retry or completion tracking is useful.

## Use as a Dockerfile Copy Source

The binary is at `/usr/local/bin/ferric`, so another image can copy it without
installing Go:

~~~dockerfile
FROM ghcr.io/ferricstore/command-line:v0.11.11 AS ferric-cli
FROM debian:stable-slim
COPY --from=ferric-cli /usr/local/bin/ferric /usr/local/bin/ferric
~~~

The destination image must provide trusted CA certificates when it uses
`ferrics://` or `https://` with a private CA.
