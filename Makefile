build-cli:
	go build -o dracula-cli cli/main.go
.PHONY: build-cli

build-server:
	go build -o dracula-server cmd/main.go
.PHONY: build-server

build-all:
	./build-all.sh
.PHONY: build-all
