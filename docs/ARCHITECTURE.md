# Architecture

> **Module:** `github.com/gogpu/compose`
> **Pattern:** Two-tier transport (Unix socket + shared memory)
> **Inspiration:** Wayland wl_shm, Android SurfaceFlinger, PipeWire SPA

## Overview

```
┌─────────────────────────────────────────┐
│   Display (one physical screen)         │
└───────────────────┬─────────────────────┘
                    │
┌───────────────────┴─────────────────────┐
│   Compositor process                    │
│   compose.Listen("/tmp/compose.sock")   │
│                                         │
│   • accepts module connections          │
│   • assigns module IDs                  │
│   • pull-based frame requests           │
│   • composites frames onto display      │
└────┬────────────┬────────────┬──────────┘
     │            │            │
   socket       socket       socket
     │            │            │
┌────┴─────┐ ┌───┴──────┐ ┌───┴───────────┐
│  Clock   │ │ Weather  │ │ Notification  │
│  module  │ │  module  │ │    module     │
│  1 Hz    │ │  0.1 Hz  │ │  60 Hz anim  │
│  own PID │ │  own PID │ │  own PID     │
└──────────┘ └──────────┘ └───────────────┘
```

## Package Structure

```
compose/                            # Public API (13 exported symbols)
├── compose.go                      # Frame type
├── server.go                       # Server (compositor side)
├── client.go                       # Client (module side)
├── option.go                       # Functional options
├── error.go                        # Sentinel errors
│
└── internal/                       # Implementation (hidden)
    ├── protocol/                   # Wire format (64B header, handshake)
    ├── codec/                      # Compression (Raw, LZ4)
    ├── conn/                       # Module lifecycle (registry, hot-plug)
    ├── flow/                       # Pull-based pacing (Wayland pattern)
    └── transport/
        └── socket/                 # Unix domain socket transport
```

## Public API

Users import ONE package: `"github.com/gogpu/compose"`.

```go
// Compositor side
srv, _ := compose.Listen("/tmp/compose.sock",
    compose.WithMaxModules(8),
    compose.WithCompression("lz4"),
)
srv.OnFrame(func(f compose.Frame) { /* composite */ })
srv.OnConnect(func(id uint64, name string) { /* module joined */ })
srv.OnDisconnect(func(id uint64, name string) { /* module left */ })

// Module side
client, _ := compose.Dial("/tmp/compose.sock",
    compose.WithName("clock"),
    compose.WithFrameSize(400, 120),
    compose.WithFPS(1),
)
client.OnFrameRequest(func() { /* render and publish */ })
client.PublishFrame(compose.Frame{ Pixels: rgba, Width: 400, Height: 120 })
```

## Wire Protocol v1

64-byte fixed header (cache-line aligned, little-endian):

| Offset | Field | Size | Description |
|--------|-------|------|-------------|
| 0 | Magic | 4B | `0x434F4D50` ("COMP") |
| 4 | Version | 2B | Protocol version |
| 6 | MsgType | 1B | Frame, Handshake, Ack, FrameRequest, Resize, Disconnect |
| 7 | Flags | 1B | DirtyValid, Compressed, Keyframe |
| 8 | ModuleID | 8B | Compositor-assigned |
| 16 | Sequence | 8B | Monotonic frame counter |
| 24 | Timestamp | 8B | Monotonic nanoseconds |
| 32 | Width | 2B | Frame width |
| 34 | Height | 2B | Frame height |
| 36 | Stride | 4B | Bytes per row |
| 40 | DirtyRect | 8B | x, y, w, h (2B each) |
| 48 | PixelFormat | 1B | RGBA8, BGRA8 |
| 49 | Compression | 1B | None, LZ4, Zstd |
| 56 | PayloadSize | 4B | Compressed payload bytes |
| 60 | UncompressedSize | 4B | Original pixel bytes |

## Frame Delivery (ADR-002)

Both push and pull delivery coexist. No mode negotiation — inferred from behavior (Chromium pattern).

### Push (module-driven)

Module calls `PublishFrame()` whenever data changes. Server stores in per-module mailbox (latest-frame-wins). Compositor samples via `Snapshot()`.

```
Module: data changes → PublishFrame() → socket → server mailbox (overwrites previous)
Compositor: render tick → Snapshot() → latest frame from each module
```

### Pull (compositor-driven, Wayland pattern)

1. Compositor → Module: `RequestFrame`
2. Module renders → sends `Frame`
3. Frame stored in mailbox + OnFrame fires
4. Adaptive rate: 3 consecutive misses → halve request rate

### Mailbox semantics (Android BufferQueue / Vulkan MAILBOX)

Each module has one mailbox slot. When a module pushes faster than the compositor renders, intermediate frames are silently overwritten. The compositor always sees the latest frame. No stale frame accumulation, no FIFO backlog.

```go
// Compositor render tick:
frames := srv.Snapshot()
for id, frame := range frames {
    compositor.Blit(id, frame)
}
```

## Connection Lifecycle

```
Module connects → Handshake (name, size, fps)
                → Compositor assigns ID, fires OnConnect
                → Frame loop (pull-based)
                → Module crashes → EOF → OnDisconnect
                → Module reconnects → new handshake → same slot
```

## Design Principles

1. **Minimal public surface** — < 15 exported symbols, one import
2. **Enterprise internal/** — all implementation hidden behind `internal/`
3. **Zero allocations on hot path** — pre-allocated buffers, reused headers
4. **Cross-platform** — Linux, macOS, Windows (AF_UNIX), FreeBSD
5. **Zero CGO** — Pure Go transports, Pure Go compression
6. **Independent releases** — protocol versioned separately from API

## Performance

| Metric | Value |
|--------|-------|
| Header encode/decode | 6–24 ns, 0 allocs |
| LZ4 compression | 2.9 GB/s encode |
| GUI pixel compression ratio | 99.6% (flat color) |
| Socket throughput | 4.3 GB/s |
| Frame latency (192KB) | 45 μs |
| Flow control overhead | 37 ns/decision |

## Dependency Graph

```
compose (root) ──→ internal/protocol (leaf, no deps)
    ├──→ internal/transport/socket ──→ internal/protocol
    ├──→ internal/codec (standalone)
    ├──→ internal/flow (standalone)
    └──→ internal/conn (standalone)
```

No circular dependencies. `internal/protocol` is the leaf — imported by others, imports nothing internal.

## Part of GoGPU Ecosystem

```
naga (shaders) → wgpu (WebGPU) → gogpu (windowing) → gg (2D) → ui (widgets)
                                                                    ↓
                                                              compose (IPC)
```
