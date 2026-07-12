.PHONY: web build build-dev dev test test-go test-web e2e clean lint fmt

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
