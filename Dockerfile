# syntax=docker/dockerfile:1.19@sha256:b6afd42430b15f2d2a4c5a02b919e98a525b785b1aaff16747d2f623364e39b6

ARG GO_IMAGE=golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build \
      -buildvcs=false \
      -mod=readonly \
      -trimpath \
      -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$BUILD_DATE" \
      -o /out/ferric \
      ./cmd/ferric

FROM ${RUNTIME_IMAGE}

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="Ferric CLI" \
      org.opencontainers.image.description="Command-line interface for FerricStore and FerricFlow" \
      org.opencontainers.image.source="https://github.com/ferricstore/command-line" \
      org.opencontainers.image.url="https://github.com/ferricstore/command-line" \
      org.opencontainers.image.documentation="https://github.com/ferricstore/command-line/blob/main/docs/containers.md" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$BUILD_DATE"

ENV HOME=/home/nonroot \
    XDG_CONFIG_HOME=/home/nonroot/.config

COPY --from=build --chown=65532:65532 /out/ferric /usr/local/bin/ferric

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/ferric"]
CMD ["--help"]
