# NovaNas top-level Makefile.
# Thin convenience wrappers — the heavy lifting lives in pnpm, cargo, go,
# and docker compose. Run `make help` to see targets.

.DEFAULT_GOAL := help
.PHONY: help dev-up dev-down dev-logs dev-reset dev-ps

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev-up: ## Start local dev stack (postgres, redis, keycloak, openbao, prometheus, api, ui)
	cd dev && docker compose up -d --build

dev-down: ## Stop local dev stack (preserves volumes)
	cd dev && docker compose down

dev-logs: ## Tail logs from the dev stack
	cd dev && docker compose logs -f

dev-reset: ## Wipe all dev volumes (destructive)
	cd dev && docker compose down -v

dev-ps: ## Show dev stack status
	cd dev && docker compose ps
