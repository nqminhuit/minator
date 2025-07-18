.MAKEFLAGS: -k

.PHONY: all build clean fmt vet

BUILD_DIR := build

all: run

run: ## start server in dev mode
	go run main.go

fmt: ## Format Go source files
	go fmt ./...

tidy: ## Tidy up Go modules
	go mod tidy

clean: ## Clean build output
	rm -rf $(BUILD_DIR)/*

help: ## Print available commands and their usage
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
