<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/gogpu/.github/main/assets/logo.png">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/gogpu/.github/main/assets/logo.png">
    <img src="https://raw.githubusercontent.com/gogpu/.github/main/assets/logo.png" alt="GoGPU Logo" width="100" />
  </picture>
</p>

<h1 align="center">compose</h1>

<p align="center">
  <strong>Pure Go multi-process composition library</strong><br>
  Independent processes render UI content into offscreen buffers, and a single<br>
  compositor process composes them onto one display. Zero CGO.<br>
  First-class for <a href="https://github.com/gogpu/gg">gogpu/gg</a> and <a href="https://github.com/gogpu/ui">gogpu/ui</a> — works with any Go rendering library, or even non-Go modules speaking the wire protocol.
</p>

<p align="center">
  <a href="https://github.com/gogpu/compose/actions/workflows/ci.yml"><img src="https://github.com/gogpu/compose/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://app.codecov.io/gh/gogpu/compose/branch/main"><img src="https://codecov.io/gh/gogpu/compose/branch/main/graph/badge.svg" alt="Coverage"></a>
  <a href="https://pkg.go.dev/github.com/gogpu/compose"><img src="https://pkg.go.dev/badge/github.com/gogpu/compose.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/gogpu/compose"><img src="https://goreportcard.com/badge/github.com/gogpu/compose" alt="Go Report Card"></a>
  <a href="https://github.com/gogpu/compose/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/gogpu/compose"><img src="https://img.shields.io/badge/Pure_Go-Zero_CGO-brightgreen" alt="Zero CGO"></a>
</p>

---

## Installation

```bash
go get github.com/gogpu/compose
```

## What is compose?

The `compose` library lets Go applications combine content from several independent processes onto a single display.

Each module is a separate OS binary that renders into an offscreen buffer using [gogpu/gg](https://github.com/gogpu/gg) primitives or [gogpu/ui](https://github.com/gogpu/ui) widgets, then ships pixels over a Unix socket or shared memory ring buffer to a compositor process. The compositor positions and blits each module's frame onto the screen at its assigned slot.

This gives you:

- **Process isolation** — a crash in one module never takes down the display
- **Hot-pluggable modules** — start, stop, replace, and update individual modules without restarting the compositor
- **Cross-language modules** — anything that can write RGBA to a Unix socket can participate, not just Go
- **Independent module lifecycles** — each module ships, releases, and updates on its own schedule
- **Cross-platform** — Linux, macOS, **Windows**, FreeBSD as first-class targets. Windows supports the Unix domain socket transport natively via `AF_UNIX` (since Windows 10 1803, April 2018), so the same `net.Listen("unix", ...)` works on every desktop OS. Future: Redox OS.
- **Zero CGO** — Pure Go on the wire and on every supported platform. Shared memory uses POSIX `mmap` on Unix-likes and `CreateFileMapping` on Windows, behind a unified Go interface

```go
package main

import (
    "github.com/gogpu/compose"
    "github.com/gogpu/gg"
)

func main() {
    // Module side: render a 400x120 frame, ship it to the compositor
    client, _ := compose.Dial("/tmp/compose.sock")
    defer client.Close()

    dc := gg.NewContext(400, 120)
    dc.ClearWithColor(gg.Transparent)
    dc.SetRGB(1, 1, 1)
    dc.DrawString("Hello from module", 20, 60)

    client.PublishFrame(compose.Frame{
        Name:   "hello-module", // human-readable; the wire ID is assigned at handshake
        Pixels: dc.Image(),
    })
}
```

```go
// Compositor side: listen for module frames, place them onto a gogpu window
srv, _ := compose.Listen("/tmp/compose.sock")
srv.OnFrame(func(f compose.Frame) {
    // 'layout' is your application's slot-assignment helper that
    // decides where each module's pixels go on the screen.
    layout.Place(f.Name, f.Pixels)
})
```

> Unix socket transport, wire protocol v1, LZ4 compression, pull-based flow control. See the [Roadmap](#roadmap) for current progress.

## Why a separate library?

The `compose` library deliberately lives outside `gogpu/ui` and `gogpu/gogpu`:

- **Not UI-specific.** A module can render with `gg`, with `ui`, or with any third-party Go rendering code. It can even be written in another language and pipe raw RGBA into the socket. Anchoring this in a UI library would be the wrong scope.
- **Not app-framework specific.** `gogpu` is about "one process renders one window". Multi-process composition is a different problem area with different lifecycle, trust, and protocol concerns.
- **Platform-specific dependencies belong in their own module.** Unix domain sockets, shared memory primitives (`mmap` on Unix, `CreateFileMapping` on Windows), ring buffers, build-tagged transport implementations — users of the UI framework should not pay for them transitively.
- **The IPC protocol is a stable compatibility surface.** It needs its own versioning discipline and release cadence so that protocol changes do not force a UI framework release and vice versa.
- **Precedent.** Qt ships Qt Remote Objects as a separate module. GTK ships Broadway as a separate display backend. Flutter separates Engine from Framework. Mature UI ecosystems consistently separate infrastructure layers from widget layers.

## How is this different from multi-window?

| Concern | [Multi-window in gogpu](https://github.com/orgs/gogpu/discussions/167) | Multi-process compositor (this library) |
|---|---|---|
| Processes | 1 | N + 1 |
| GPU Devices | 1 shared | N independent |
| Resource sharing | pipelines, textures, buffers | only pixels via IPC |
| Trust model | full trust between windows | process isolation |
| Crash isolation | whole app exits | one module dies, compositor lives |
| Cross-language | Go only | any language with POSIX sockets |
| Hot-reload | not needed (one binary) | first-class concern |
| Communication | function calls (zero cost) | IPC framing (small latency) |
| Use cases | IDEs, dialogs, multi-doc editors | smart mirrors, kiosks, modular dashboards, plugin hosts |

The two are **layers, not alternatives**. A compose-based application can use multi-window internally if its compositor process needs to span multiple physical monitors. A module can use multi-window internally if it wants sub-windows. They compose cleanly.

## Use cases

- **Smart mirrors and kiosks** — modular displays where third-party modules render time, weather, calendars, notifications, transit, news
- **Modular dashboards** — independent data sources (each as its own process, possibly on different teams or repos) feeding one operations display
- **Plugin hosts** — applications that load untrusted third-party plugins and need crash containment
- **Cross-language UIs** — Go compositor with modules written in Rust, Python, or C
- **Embedded HMIs** — industrial control panels with hot-pluggable functional blocks
- **Live event displays** — concert visuals, sports broadcasts, conference signage with independently-developed segments

## Architecture

```
                 ┌────────────────────────────────────┐
                 │   Display  (one physical screen)   │
                 └─────────────────┬──────────────────┘
                                   │
                 ┌─────────────────┴──────────────────┐
                 │   Compositor process               │
                 │   (gogpu window owns the surface)  │
                 │                                    │
                 │   • accepts module connections     │
                 │   • assigns slots in the layout    │
                 │   • blits incoming frames          │
                 │   • handles hot-plug & lifecycles  │
                 └────┬────────────┬────────────┬─────┘
                      │            │            │
                Unix socket   Unix socket   shared memory
                      │            │            │
        ┌─────────────┴──┐ ┌───────┴──────┐ ┌───┴───────────┐
        │  Clock module  │ │ Weather mod. │ │ Notification  │
        │                │ │              │ │   module      │
        │  gg primitives │ │ gg primitives│ │ ui widgets    │
        │  1 Hz · static │ │ 0.1 Hz       │ │ 60 Hz · anim. │
        │  own process   │ │ own process  │ │ own process   │
        │  own GPU       │ │ own GPU      │ │ own GPU       │
        └────────────────┘ └──────────────┘ └───────────────┘
```

Each module owns its own process, its own GPU device (if it uses GPU at all), its own crash domain, and its own release lifecycle. The compositor is the only process that touches the actual display surface.

## IPC transports

Pixels travel through one of two transports, chosen per module:

| Transport | Best for | Throughput | Setup |
|---|---|---|---|
| **Unix domain socket** | Static / low-rate modules (clocks, weather, calendars) | Hundreds of MB/s on Pi-class hardware | Zero — just connect |
| **Shared memory ring buffer** | Animated / high-rate modules (notifications, video, charts) | Limited only by memory bandwidth, zero copy | `mmap` (POSIX) or `CreateFileMapping` (Windows) behind a unified interface |

**Cross-platform support:** Linux, macOS, FreeBSD, and **Windows 10+** (which gained `AF_UNIX` support in the 1803 release, April 2018, so Go's `net.Listen("unix", ...)` works natively on Windows). Future: Redox OS when its Go toolchain matures.

The library deliberately avoids Linux-specific kernel APIs (`io_uring`, `eventfd`, Linux-specific shm segments) so the design ports cleanly across all desktop OSes. Where POSIX and Windows diverge — fd passing for shared memory is `SCM_RIGHTS` on POSIX and named handles on Windows — a thin Go interface with build tags hides the difference from module authors.

## Wire protocol v1

64-byte fixed header, little-endian, cache-line aligned:

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
| 50 | Reserved | 6B | Future use |
| 56 | PayloadSize | 4B | Compressed payload bytes |
| 60 | UncompressedSize | 4B | Original pixel bytes |

Followed by `PayloadSize` bytes of pixel data (RGBA premultiplied, optionally LZ4-compressed).

The protocol is the stable compatibility surface of compose. Versioning is independent from the rest of the gogpu ecosystem so that protocol changes do not force a UI framework release.

## Roadmap

| Phase | Features | Status |
|-------|----------|--------|
| **Phase 1** | Wire protocol v1, Unix socket transport, LZ4 compression, public API | Complete |
| **Phase 2** | Reference examples: compositor + clock + notification (multi-process) | Next |
| **Phase 3** | Shared memory ring buffer transport (zero-copy) | Planned |
| **Phase 4** | Delta frames, compression negotiation | Planned |
| **Phase 5** | Multi-screen layout, layered z-order, fade transitions | Future |
| **Phase 6** | Cross-language module SDK (C header, Rust crate, Python) | Future |

## Design principles

1. **Example first, library second.** The reference example proves the pattern with real users before any API freezes. Library extraction happens after at least two real use cases agree on the shape.
2. **POSIX only.** No Linux-specific kernel APIs. Portable to Redox OS, FreeBSD, and other POSIX systems.
3. **Zero CGO.** Pure Go transports, pure Go protocol, single binary deployment per module.
4. **Process isolation as a feature, not an accident.** Crashes contained, modules hot-swappable, third-party modules sandboxed by default.
5. **Independent releases.** The `compose` library versions independently from `gogpu`, `ui`, and `gg`. The wire protocol has its own versioning discipline.
6. **Language-agnostic at the wire.** A module can be a Rust binary or a Python script as long as it speaks the protocol.

## Inspiration

The design of `compose` is informed by:

- **Android SurfaceFlinger** — the system compositor that combines layers from independent processes onto Android displays
- **Wayland compositors** — `wlroots`, `smithay`, KDE KWin, GNOME Mutter
- **MagicMirror²** — the JavaScript / Electron smart mirror project that the first user of `compose` is rewriting in Go
- **Qt Remote Objects** — Qt's process-isolated component framework
- **GTK Broadway** — GTK's HTML5 display backend, which composes widget pixels remotely
- **Flutter Engine vs Framework split** — the architectural separation between the rendering substrate and the UI toolkit

## Status and contributing

The `compose` library ships a working Unix socket transport with wire protocol v1, LZ4 compression, pull-based flow control, and hot-plug module lifecycle.

Known users: [KiGo](https://github.com/AgentNemo00/kigo) (modular Go application using offscreen rendering + multi-process composition).

If you have a use case for multi-process composition in Go, please join the [compose RFC discussion](https://github.com/orgs/gogpu/discussions/177) or open an issue describing your scenario.

## Part of the GoGPU Ecosystem

The `compose` library is part of [GoGPU](https://github.com/gogpu) — a Pure Go GPU computing ecosystem.

| Library | Purpose |
|:--------|:--------|
| [gogpu](https://github.com/gogpu/gogpu) | Application framework, windowing, multi-window |
| [wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU (Vulkan/Metal/DX12/GLES) |
| [naga](https://github.com/gogpu/naga) | Shader compiler (WGSL → SPIR-V/MSL/GLSL/HLSL/DXIL) |
| [gg](https://github.com/gogpu/gg) | 2D graphics with GPU acceleration |
| [g3d](https://github.com/gogpu/g3d) | 3D rendering (scene graph, PBR, GLTF) |
| [ui](https://github.com/gogpu/ui) | GUI toolkit (22+ widgets, 4 themes) |
| **[compose](https://github.com/gogpu/compose)** | **Multi-process composition (this library)** |
| [systray](https://github.com/gogpu/systray) | System tray (Win32/macOS/Linux) |

## License

MIT License — see [LICENSE](LICENSE) for details.
