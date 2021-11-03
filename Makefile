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
