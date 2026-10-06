# Run from the repo root. See AGENTS.md "Commands".
TEST_COMPOSE := docker compose -f docker-compose.test.yml
INTEGRATION  := -tags integration -race -p 1

.PHONY: fmt fmt-check vet lint test test-integration test-race cover gate mobile-check

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

# Unit and integration tests together; -race is left out, coverage only.
# Per-package figures come from cover.out: statements covered / statements.
# Mobile coverage leaves out the test helpers in src/test/.
cover:
	$(TEST_COMPOSE) up -d --wait
	cd backend && go test -tags integration -p 1 -count=1 -coverpkg=./... -coverprofile=cover.out ./...
	cd backend && go tool cover -func=cover.out
	@cd backend && awk 'NR>1 { k=$$1" "$$2; st[k]=$$2; if ($$3>0) hit[k]=1; f=$$1; sub(":.*","",f); pk[k]=f; sub("/[^/]*$$","",pk[k]) } \
	END { for (k in st) { p=pk[k]; s[p]+=st[k]; if (k in hit) c[p]+=st[k] } for (p in s) printf "package %-76s %5.1f%% (%d/%d)\n", p, 100*c[p]/s[p], c[p], s[p] }' cover.out | sort
	@if [ -f mobile/package.json ]; then cd mobile && npx jest --coverage --coverageReporters=text-summary \
		--coveragePathIgnorePatterns=/node_modules/ --coveragePathIgnorePatterns=/src/test/; fi

gate: fmt-check vet lint test test-integration

mobile-check:
	@if [ ! -f mobile/package.json ]; then echo "mobile/ not created"; exit 0; fi; \
	cd mobile && npm run lint && npm run typecheck && npm test
