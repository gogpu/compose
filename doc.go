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
// Multi-window in gogpu (ADR-010) and the compose model are different concepts.
// Multi-window shares one GPU Device across N native windows inside a single
// process. The compose model is the opposite: N independent processes, each
// with its own GPU Device, cooperating over IPC to produce a single composed
// display.
//
// Status: design phase. See the repository README for the roadmap and the
// linked ui#75 discussion for context.
//
// Part of the GoGPU ecosystem: https://github.com/gogpu
package compose
