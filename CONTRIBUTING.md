# Contributing to compose

Thank you for your interest in contributing to gogpu/compose! This document covers how to build, test, and submit changes.

## Prerequisites

- **Go 1.25+** ([download](https://go.dev/dl/))
- **golangci-lint** (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)

## Building

```bash
go build ./...
```

## Running Tests

```bash
go test ./...
```

## Running Tests with Coverage

```bash
go test -coverprofile=tmp/coverage.out ./...
go tool cover -html=tmp/coverage.out
```

## Running Linter

```bash
golangci-lint run --timeout=5m
```

## Code Standards

- **Pure Go** — zero CGO, zero platform-specific code in public API
- **Enterprise quality** — 90%+ test coverage, zero-alloc hot paths
- **Functional options** — use `With*` pattern for configuration
- **Internal packages** — implementation details live in `internal/`

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make changes with tests
4. Run `go fmt ./... && golangci-lint run --timeout=5m && go test ./...`
5. Commit with conventional format (`feat:`, `fix:`, `docs:`, etc.)
6. Open a pull request against `main`

## Commit Messages

```
feat: add shared memory transport
fix: handle connection timeout on Windows
docs: update wire protocol documentation
test: add benchmark for frame compression
chore: update lz4 dependency
```

## Architecture

The library follows a strict internal/ pattern:

```
compose/            # Public API (< 15 exported symbols)
├── internal/
│   ├── protocol/   # Wire format (64B header, handshake)
│   ├── codec/      # Compression (Raw, LZ4)
│   ├── conn/       # Module lifecycle
│   ├── flow/       # Pull-based pacing
│   └── transport/
│       ├── socket/ # Unix domain socket (Phase 1)
│       └── shm/    # Shared memory (Phase 2)
```

Users import only `"github.com/gogpu/compose"`. All implementation is hidden.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
