// Package compose is a Pure Go multi-process composition library for the gogpu
// ecosystem. It lets independent OS processes render UI content into offscreen
// buffers and ship pixels to a single compositor process that blits them onto
// the screen.
//
// The compositor model provides process isolation (a crash in one module does
// not take down the display), hot-pluggable third-party modules, and the
// possibility of cross-language modules — anything that can write RGBA to a
// Unix socket or shared memory segment can participate.
//
// # Quick Start
//
// Compositor (server) side:
//
//	srv, err := compose.Listen("/tmp/compose.sock",
//	    compose.WithMaxModules(8),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer srv.Close()
//
//	srv.OnFrame(func(f compose.Frame) {
//	    // Blit f.Pixels onto the compositor window.
//	})
//
// Module (client) side:
//
//	client, err := compose.Dial("/tmp/compose.sock",
//	    compose.WithName("clock"),
//	    compose.WithFrameSize(400, 120),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	client.OnFrameRequest(func() {
//	    // Render and publish a frame.
//	    _ = client.PublishFrame(compose.Frame{
//	        Pixels: renderClock(),
//	        Width:  400,
//	        Height: 120,
//	    })
//	})
//
// # Architecture
//
// Multi-window in gogpu (ADR-010) and the compose model are different concepts.
// Multi-window shares one GPU Device across N native windows inside a single
// process. The compose model is the opposite: N independent processes, each
// with its own GPU Device, cooperating over IPC to produce a single composed
// display.
//
// The public API consists of five types (Frame, Server, Client, ServerOption,
// ClientOption) and three constructor functions (Listen, Dial, and With*
// options). All implementation details live behind internal/ boundaries.
//
// Part of the GoGPU ecosystem: https://github.com/gogpu
package compose
