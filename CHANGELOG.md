# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Wire protocol v1** (`internal/protocol/`) — 64-byte fixed header, 128-byte handshake messages, message types, encode/decode with zero allocations (100% coverage)
- **Codec package** (`internal/codec/`) — Raw pass-through + LZ4 block compression via `pierrec/lz4/v4` (97% coverage, 2.9 GB/s encode, 99.6% compression on GUI pixels)
- **Connection manager** (`internal/conn/`) — module registry with monotonic ID allocation, lifecycle state machine, hot-plug callbacks (98.9% coverage)
- **Flow controller** (`internal/flow/`) — pull-based frame pacing (Wayland frame callback pattern), adaptive rate reduction after missed frames (100% coverage)
- **Unix socket transport** (`internal/transport/socket/`) — framed Conn, Listener, Dialer for Unix domain sockets (95.1% coverage, 4.3 GB/s, 45μs latency)
- **Public API** — `compose.Listen()`, `compose.Dial()`, `Frame` type, functional options (`WithMaxModules`, `WithCompression`, `WithName`, `WithFrameSize`, `WithFPS`)
- **CI/CD** — GitHub Actions (build/test/lint on Ubuntu/macOS/Windows), Codecov, golangci-lint v2
