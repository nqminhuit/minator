.MAKEFLAGS: -k

# This approach works but may slightly slow down make on large files. Use it if you're fine with that tradeoff.
PHONY_TARGETS := $(shell grep -E '^[a-zA-Z0-9_-]+:.*' $(MAKEFILE_LIST) | \
	sed -E 's/^([a-zA-Z0-9_-]+):.*/\1/' | sort -u)
.PHONY: $(PHONY_TARGETS)

BUILD_DIR := build

all: run

run: ## start server in dev mode
	go run main.go

infra: ## start infra services like forgejo, postgres,...
	podman run --replace -d --name forgejo -p 3000:3000 -p 2222:22 -v /opt/compose/forgejo:/data -v /etc/timezone:/etc/timezone:ro -v /etc/localtime:/etc/localtime:ro codeberg.org/forgejo/forgejo:12.0.0
	podman run --replace -d --name minator-postgres -e POSTGRES_PASSWORD=123456 -p 5432:5432 docker.io/postgres:latest

fmt: ## Format Go source files
	go fmt ./...

tidy: ## Tidy up Go modules
	go mod tidy

clean: ## Clean build output
	rm -rf $(BUILD_DIR)/*

sample-test:
	@status=$$(shuf -n1 -e inprogress success failed); \
	previous=$$(shuf -n1 -e Forgejo MySQL Redis MongoDB); \
	current=$$(shuf -n1 -e PostgreSQL SQLite Cassandra MariaDB); \
	json=$$(printf '{"name":"Backup","status":"%s","details":{"previousBackup":"%s","currentBackup":"%s"}}' $$status $$previous $$current); \
	curl "localhost:18080/api/service/status" \
		-H "content-type:application/json" \
		-d "$$json"

help: ## Print available commands and their usage
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
