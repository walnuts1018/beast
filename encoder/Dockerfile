# syntax=docker/dockerfile:1.21
FROM golang:1.26.1-trixie AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod download -x

COPY . ./
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOOS=linux go build -trimpath -o encoder ./main.go && chmod +x ./encoder

FROM debian:13.3-slim
WORKDIR /app

RUN rm -f /etc/apt/apt.conf.d/docker-clean; echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache
RUN --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    apt-get -y update && apt-get upgrade -y && apt-get install -y ca-certificates ffmpeg

COPY --from=builder /build/encoder ./
CMD ["./encoder"]
