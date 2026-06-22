GO ?= go
BINARY ?= sim
BIN_DIR ?= bin

.PHONY: build run test vet build-frontend clean

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(BINARY) ./cmd/sim

run:
	$(GO) run ./cmd/sim

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
