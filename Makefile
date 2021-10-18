
build-cli:
	go build -o dracula-cli cli/main.go
.PHONY: build-cli

build-server:
	go build -o dracula cmd/main.go
.PHONY: build-server
