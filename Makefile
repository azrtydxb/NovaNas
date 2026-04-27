# NovaNas top-level Makefile.
# Thin convenience wrappers — the heavy lifting lives in pnpm, cargo, go,
# and docker compose. Run `make help` to see targets.

.DEFAULT_GOAL := help
.PHONY: help dev-up dev-down dev-logs dev-reset dev-ps \
        dev-cluster-up dev-cluster-down dev-cluster-reset dev-load-image

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev-up: ## Start local dev stack (postgres, redis, keycloak, openbao, prometheus, api, ui)
	./dev/scripts/ensure-kubeconfig.sh
	cd dev && docker compose up -d --build

dev-down: ## Stop local dev stack (preserves volumes)
	cd dev && docker compose down

dev-logs: ## Tail logs from the dev stack
	cd dev && docker compose logs -f

dev-reset: ## Wipe all dev volumes (destructive)
	cd dev && docker compose down -v

dev-ps: ## Show dev stack status
	cd dev && docker compose ps

# ---- Full stack with a real kube-apiserver (kind) ---------------------------

dev-cluster-up: ## kind + compose
	./dev/kind/create-cluster.sh
	$(MAKE) dev-up

dev-cluster-down: ## Stop compose + delete kind cluster
	$(MAKE) dev-down
	./dev/kind/uninstall.sh

dev-cluster-reset: ## Full reset: tear everything down and re-create
	$(MAKE) dev-cluster-down
	$(MAKE) dev-cluster-up

dev-load-image: ## Rebuild+load a component image into the kind cluster
	@read -p "Component (api/ui/operators): " C; \
	docker build -t novanas/$$C:dev packages/$$C/; \
	kind load docker-image novanas/$$C:dev --name novanas-dev
