# Run from the repo root. See AGENTS.md "Commands".
TEST_COMPOSE := docker compose -f docker-compose.test.yml
INTEGRATION  := -tags integration -race -p 1

.PHONY: fmt fmt-check vet lint test test-integration test-race gate mobile-check load-test

fmt:
	cd backend && gofmt -w .

fmt-check:
	@out=$$(cd backend && gofmt -l .); \
	if [ -n "$$out" ]; then echo "gofmt: not formatted:"; echo "$$out"; exit 1; fi

# vet and lint also cover the integration build tag.
vet:
	cd backend && go vet ./... && go vet -tags integration ./...

lint:
	cd backend && go tool staticcheck ./... && go tool staticcheck -tags integration ./...

# No tags, no database: also what pre-push runs.
test:
	cd backend && go test ./...

test-integration:
	$(TEST_COMPOSE) up -d --wait
	cd backend && go test $(INTEGRATION) -count=1 ./internal/integration/...

test-race:
	$(TEST_COMPOSE) up -d --wait
	cd backend && go test $(INTEGRATION) -count=20 -run '^TestRace' ./internal/integration/...

gate: fmt-check vet lint test test-integration

mobile-check:
	@if [ ! -f mobile/package.json ]; then echo "mobile/ not created"; exit 0; fi; \
	cd mobile && npm run lint && npm run typecheck && npm test

# Stack must be up. Both cache modes, k6 in Docker (D51).
load-test:
	LOADTEST_UID=$$(id -u) LOADTEST_GID=$$(id -g) loadtest/run.sh
