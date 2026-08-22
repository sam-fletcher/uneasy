.PHONY: build frontend server placeholder clean \
        test test-integration test-integration-run test-frontend-unit test-e2e \
        vet check-frontend check sqlc deadcode lint-install lint-check

# Full build: compile the frontend and produce a single Go binary that
# embeds it. Output: ./server
build: frontend server

frontend:
	cd frontend && npm ci && npm run build
	mkdir -p cmd/server/frontend_dist
	find cmd/server/frontend_dist -mindepth 1 -not -name .gitignore -exec rm -rf {} +
	cp -R frontend/build/. cmd/server/frontend_dist/

# Ensure the embed directory has at least one file so `go build` succeeds
# without a frontend build (CI Go-only checks, fresh clones, etc.).
placeholder:
	@mkdir -p cmd/server/frontend_dist
	@test -f cmd/server/frontend_dist/index.html || \
		printf '<!doctype html><html><body>Frontend not built. Run `make build`.</body></html>\n' \
		> cmd/server/frontend_dist/index.html

server: placeholder
	CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o server ./cmd/server

clean:
	rm -f server
	mkdir -p cmd/server/frontend_dist
	find cmd/server/frontend_dist -mindepth 1 -not -name .gitignore -exec rm -rf {} +

# ── Tests & checks ───────────────────────────────────────────────────────────
# `test` runs the unit tests with no DB requirement. `test-integration`
# expects TEST_DATABASE_URL to point at a Postgres the suite may TRUNCATE
# between cases — never a dev or production DB. `-p 1` serializes packages
# so they don't race on the shared schema.
#
# `check` is the one-shot pre-commit gate: vet → unit → integration →
# frontend type-check. Requires Postgres for the integration step; if you
# don't have it running, use `make check-fast` instead.

TEST_DATABASE_URL ?= postgres://uneasy:uneasy@localhost:5432/uneasy_test?sslmode=disable

test:
	go test -count=1 ./...

test-integration:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) \
		go test -tags=integration -count=1 -p 1 ./...

# Run a single integration test (or matching pattern) in one package.
# Usage: make test-integration-run RUN=TestFoo PKG=./handler/...
PKG ?= ./...
test-integration-run:
	@test -n "$(RUN)" || (echo "RUN=<test name or regex> required" && exit 1)
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) \
		go test -tags=integration -count=1 -run '$(RUN)' -v $(PKG)

# golangci-lint is pinned, and installed from the project's own release binaries
# rather than from a package manager. Upstream's install docs warn that
# "Homebrew can use an unexpected version of Go to build the binary", and that
# is not cosmetic: a linter built against an older Go panics outright on a newer
# stdlib ("file requires newer Go version go1.27") and takes this target down
# with it. `go install` has the same failure mode for the same reason — it
# compiles against whatever Go is local. The published binaries are built
# against the current Go, so pinning the version here pins the behaviour.
GOLANGCI_VERSION ?= v2.13.1

# Always invoked by absolute path, never through PATH. `go env GOPATH`/bin is
# where the install script puts it, but that directory is not on PATH by
# default on macOS — resolving through PATH would miss it entirely, or find
# some other build of the linter that happens to be installed.
GOBIN_DIR := $(shell go env GOPATH)/bin
GOLANGCI  := $(GOBIN_DIR)/golangci-lint

lint-install:
	curl -sSfL https://golangci-lint.run/install.sh \
		| sh -s -- -b $(GOBIN_DIR) $(GOLANGCI_VERSION)

# Offline guard: fail early and legibly when the installed linter has drifted
# from the pin, rather than letting it surface as puzzling lint differences or
# a panic mid-run.
lint-check:
	@$(GOLANGCI) --version 2>/dev/null | grep -q ' version $(GOLANGCI_VERSION:v%=%) ' || { \
		printf 'golangci-lint is not %s — run: make lint-install\n  have: ' '$(GOLANGCI_VERSION)'; \
		$(GOLANGCI) --version 2>/dev/null || echo 'nothing at $(GOLANGCI)'; \
		exit 1; }

vet: lint-check
	go build ./...
	go fix ./...
	go vet ./...
	$(GOLANGCI) run ./... --fix
	go vet -tags=integration ./...

check-frontend:
	cd frontend && npm run check

# Vitest unit tests for pure-TS lib/* modules. Fast, mockless, no DB.
test-frontend-unit:
	cd frontend && npm run test:unit

# Playwright end-to-end tests. Drives a real browser against the server-e2e
# container on :8090, which is bound to uneasy_test — never the dev DB.
# Assumes `docker compose up` is already running. Shares uneasy_test with
# `test-integration`, so don't run them concurrently (check runs them
# sequentially).
test-e2e:
	cd frontend && npm run test:e2e

check: check-fast test-integration test-e2e

check-fast: vet deadcode test check-frontend test-frontend-unit

# Regenerate sqlc bindings after touching db/queries/*.sql or migrations.
sqlc:
	sqlc generate

# Whole-program dead-code report (functions unreachable from main). Catches
# genuinely dead *exported* helpers that golangci-lint's `unused` can't —
# `unused` never flags exported identifiers in importable packages, which is
# how 8 dead Load*Data parsers once hid in game/.
#
# Flags: `-test` treats test functions as roots so production helpers exercised
# only by tests aren't false-flagged (e.g. LoadMakeWarData, used solely by a
# Make War integration test); `-tags integration` compiles the integration-
# tagged test files so those callers are visible. Trade-off: with `-test`, a
# truly dead helper that still has a lingering test will NOT be reported.
deadcode:
	go run golang.org/x/tools/cmd/deadcode@latest ./...
