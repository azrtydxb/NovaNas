.PHONY: build test test-integration test-e2e lint fmt gen gen-sqlc gen-openapi gen-ts run clean migrate-up migrate-down migrate-status

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

gen: gen-sqlc gen-openapi

gen-sqlc:
	sqlc generate

gen-openapi:
	./scripts/gen-openapi.sh

gen-ts:
	./scripts/gen-ts-client.sh

run: build
	./$(BIN)

clean:
	rm -rf bin/ dist/

DB_URL ?= postgres://novanas:novanas@localhost:5432/novanas?sslmode=disable

migrate-up:
	goose -dir internal/store/migrations postgres "$(DB_URL)" up

migrate-down:
	goose -dir internal/store/migrations postgres "$(DB_URL)" down

migrate-status:
	goose -dir internal/store/migrations postgres "$(DB_URL)" status
