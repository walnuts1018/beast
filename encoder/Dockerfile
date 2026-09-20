# syntax=docker/dockerfile:1.21

FROM golang:1.27-trixie AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/beast-encoder ./cmd/encoder

FROM debian:13.6-slim

RUN apt-get update \
    && apt-get install --no-install-recommends --yes ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 65532 --create-home --home-dir /tmp/beast-encoder beast

COPY --from=build /out/beast-encoder /usr/local/bin/beast-encoder
USER 65532:65532
WORKDIR /tmp/beast-encoder
ENTRYPOINT ["/usr/local/bin/beast-encoder"]
