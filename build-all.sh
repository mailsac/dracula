#!/usr/bin/env bash
set -e

VERSION=$(git describe --tags)
BUILD=$(git rev-parse HEAD)
PLATFORMS="darwin linux"
ARCHITECTURES="amd64 arm64"

LDFLAGS="-X main.Version=${VERSION} -X main.Build=${BUILD}"

rm -rf out/
mkdir out/

for GOOS in $PLATFORMS; do
  for GOARCH in $ARCHITECTURES; do
    echo $GOOS $GOARCH
    go build -ldflags="${LDFLAGS}" -o "out/dracula-cli_${GOOS}-${GOARCH}-${VERSION}" cli/main.go
    go build -ldflags="${LDFLAGS}" -o "out/dracula-server_${GOOS}-${GOARCH}-${VERSION}" cmd/main.go
  done
done
