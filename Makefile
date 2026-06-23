GO ?= go
BINARY ?= sim
SHOWCASE_BINARY ?= showcase-bot
BIN_DIR ?= bin

.PHONY: build build-sim build-showcase run run-showcase run-showcase-webhook demo test vet build-frontend clean

build: build-sim build-showcase

build-sim:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(BINARY) ./cmd/sim

build-showcase:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(SHOWCASE_BINARY) ./cmd/showcase-bot

run:
	$(GO) run ./cmd/sim

run-showcase:
	$(GO) run ./cmd/showcase-bot --mode polling

run-showcase-webhook:
	$(GO) run ./cmd/showcase-bot --mode webhook

demo:
	@echo "Terminal 1: make run"
	@echo "Terminal 2: make run-showcase"
	@echo "Webhook demo: restart Terminal 2 with make run-showcase-webhook"
	@echo "Open http://127.0.0.1:8080/ and send /start"

test: vet
	$(GO) test ./...

vet:
	$(GO) vet ./...

build-frontend:
	@if [ -f web/package.json ]; then \
		cd web && npm install && npm run build; \
	else \
		echo "web/package.json not present yet; keeping placeholder web/dist"; \
	fi

clean:
	rm -rf $(BIN_DIR)
