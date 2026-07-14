.PHONY: web build build-dev dev test test-go cover test-web check-web e2e clean lint fmt

VERSION ?= 0.1.0-dev
LDFLAGS := -X main.version=$(VERSION)

# Build the SvelteKit frontend into web/build (embedded by go:embed)
web:
	cd web && npm run build

# Full release build: frontend, then binary with the embedded UI.
build: web
	go build -tags embedassets -ldflags "$(LDFLAGS)" -o pine ./cmd/pine

# Build the Go binary only (no embedded frontend) — serves a dev placeholder.
build-dev:
	go build -ldflags "$(LDFLAGS)" -o pine ./cmd/pine

# Run backend proxying the UI to the Vite dev server (frontend hot reload).
dev:
	go run ./cmd/pine serve --dev --open

test: test-go

test-go:
	go test ./...

# Local coverage gate (matches CI threshold). CI also runs with -race.
cover:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/{gsub(/%/,"",$$NF); print $$NF}'); \
	echo "total coverage: $${total}%"; \
	awk -v t="$$total" 'BEGIN { if ((t+0) < 90) { printf("coverage %.1f%% is below 90%%\n", t); exit 1 } }'

test-web:
	cd web && npm test

check-web:
	cd web && npm run check

e2e:
	cd web && npm run test:e2e

lint:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -f pine coverage.out
	rm -rf dist web/build web/.svelte-kit
