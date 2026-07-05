# Contributing to Pine

Thanks for your interest in improving Pine! Contributions of all kinds are
welcome — bug reports, features, docs, and tests.

## Ground rules

- Be kind and constructive.
- Keep pull requests focused; one logical change per PR is easiest to review.
- New behavior needs tests. Bug fixes should add a regression test.

## Development setup

Requirements: **Go 1.26+** and **Node 20+**.

```sh
git clone https://github.com/underworld14/pine
cd pine

# Backend-only (serves a dev placeholder for the UI):
make build-dev && ./pine --help

# Full build with the embedded web UI:
make build && ./pine init && ./pine open
```

To iterate on the frontend with hot reload, run the Vite dev server and point
the Go server at it:

```sh
cd web && npm install && npm run dev     # terminal 1 (localhost:5173)
make dev                                 # terminal 2 (pine serve --dev)
```

## Before you open a PR

Run the full check suite locally — CI runs the same:

```sh
make test          # Go unit + integration tests
make lint          # go vet
cd web && npm test # frontend (vitest)
cd web && npx playwright install chromium && npm run test:e2e   # end-to-end
```

## Project layout

- `internal/ticket` — pure domain: frontmatter parse/serialize, IDs, dependency graph.
- `internal/store` — the single atomic write path over `.pine/`.
- `internal/server` — HTTP+JSON API, SSE live sync, static UI serving.
- `internal/attach` — image optimizer (pure-Go WebP).
- `internal/search` — in-memory Bleve index.
- `internal/gitx` — git awareness behind a swappable interface.
- `internal/contextgen` / `internal/doctor` — AI context/prompt and health checks.
- `internal/cli` — the cobra command tree.
- `web/` — SvelteKit UI (embedded into the binary via `go:embed`).

## Commit messages

Short imperative subject line; explain the "why" in the body when it isn't
obvious. Conventional-commit prefixes (`feat:`, `fix:`, `docs:`) are appreciated
but not required.

## Releases

Maintainers cut releases by pushing a `vX.Y.Z` tag; a GitHub Actions workflow
cross-compiles the binaries (with the embedded UI) and publishes them.

## License

By contributing you agree that your contributions are licensed under the
project's [MIT License](LICENSE).
