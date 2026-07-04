.PHONY: web build build-dev dev test test-go test-web e2e clean lint fmt

VERSION ?= 0.1.0-dev
LDFLAGS := -X github.com/izzadev/pine/internal/cli.version=$(VERSION)

# Build the SvelteKit frontend into web/build (embedded by go:embed)
web:
	cd web && npm run build

# Full build: frontend then binary
build: web
	go build -ldflags "$(LDFLAGS)" -o pine ./cmd/pine

# Build the Go binary only, using the empty embed stub (dev tag) — no frontend needed
build-dev:
	go build -tags dev -ldflags "$(LDFLAGS)" -o pine ./cmd/pine

# Run backend against the Vite dev server (frontend hot reload)
dev:
	go run -tags dev ./cmd/pine serve --dev --open

test: test-go

test-go:
	go test ./...

test-web:
	cd web && npm test

e2e:
	cd web && npm run test:e2e

lint:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -f pine
	rm -rf dist web/build web/.svelte-kit
