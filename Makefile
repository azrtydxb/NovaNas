.PHONY: build test test-integration test-e2e lint fmt gen run clean

GO ?= go
BIN := bin/nova-api

build:
	$(GO) build -o $(BIN) ./cmd/nova-api

test:
	$(GO) test ./...

test-integration:
	$(GO) test -tags=integration ./test/integration/...

test-e2e:
	$(GO) test -tags=e2e ./test/e2e/...

lint:
	golangci-lint run

fmt:
	$(GO) fmt ./...

gen:
	sqlc generate

run: build
	./$(BIN)

clean:
	rm -rf bin/ dist/
