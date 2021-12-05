# gets last tag
VERSION := $(shell git describe --abbrev=0 --tags)

test:
	go vet ./...
	go test ./...
.PHONY: test

build-cli:
	go build -o dracula-cli cmd/cli/main.go
.PHONY: build-cli

build-server:
	go build -o dracula-server cmd/server/main.go
.PHONY: build-server

build-all:
	./build-all.sh
.PHONY: build-all

# Assumes build-all
build-docker:
	docker build --build-arg "DRACULA_VERSION=${VERSION}" --tag "ghcr.io/mailsac/dracula:${VERSION}" .
.PHONY: build-docker
test-docker:
	docker run -d --rm -p "3509:3509" --name dracula-server-test "ghcr.io/mailsac/dracula:${VERSION}"
	docker exec dracula-server-test /app/dracula-cli -count -k test
	docker kill dracula-server-test
push-docker:
	docker push "ghcr.io/mailsac/dracula:${VERSION}"
.PHONY: push-docker
