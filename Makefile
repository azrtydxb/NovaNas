.PHONY: build all-binaries deploy-bin deploy-package test test-integration test-e2e lint fmt gen gen-sqlc gen-openapi verify-openapi gen-ts run clean migrate-up migrate-down migrate-status

GO ?= go
BIN := bin/nova-api

build:
	$(GO) build -o $(BIN) ./cmd/nova-api

# all-binaries builds every command target into bin/. Used by the .deb
# packaging and by the real-host validation harness.
all-binaries:
	mkdir -p bin
	$(GO) build -o bin/nova-api ./cmd/nova-api
	$(GO) build -o bin/nova-nvmet-restore ./cmd/nova-nvmet-restore
	$(GO) build -o bin/nova-iscsi-restore ./cmd/nova-iscsi-restore
	$(GO) build -o bin/zfs-validate ./cmd/zfs-validate
	$(GO) build -o bin/zfs-validate-neg ./cmd/zfs-validate-neg

# deploy-bin builds deployment-related binaries into bin/
deploy-bin: all-binaries
	mkdir -p bin
	$(GO) build -o bin/nova-bao-unseal ./cmd/nova-bao-unseal

# deploy-package creates a tarball with binaries, systemd units, and sample configs
# ready for deployment to a target host.
deploy-package: deploy-bin
	mkdir -p dist
	tar --exclude='*.enc' --exclude='*.token' \
		-czf dist/novanas-deployment.tar.gz \
		-C . bin/nova-api bin/nova-nvmet-restore bin/nova-iscsi-restore bin/nova-bao-unseal \
		-C . deploy/systemd/ deploy/openbao/ deploy/keycloak/ deploy/postgres/
	@echo "Deployment package created: dist/novanas-deployment.tar.gz"

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

# verify-openapi regenerates the typed OpenAPI bindings and asserts the
# committed file under internal/api/oapi/ is identical to the freshly
# generated one. Intended for CI: fails non-zero if a developer forgot
# to commit the regenerated types after editing api/openapi.yaml.
verify-openapi: gen-openapi
	git diff --exit-code -- internal/api/oapi/types.go

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
