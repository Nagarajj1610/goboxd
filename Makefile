.PHONY: build run test integration lint

COMPOSE ?= docker compose
TOOLS   := $(COMPOSE) --profile tools run --rm tools

build:
	$(COMPOSE) build goboxd

run:
	$(COMPOSE) up goboxd

test:
	$(TOOLS) go test ./...

integration:
	$(COMPOSE) up -d goboxd
	@echo "Waiting for goboxd to start..."
	@sleep 5
	$(COMPOSE) --profile tools run --rm -e API_URL=http://goboxd:8080 tools go test -tags=integration ./tests/...
	$(COMPOSE) down goboxd

lint:
	$(TOOLS) golangci-lint run ./...
