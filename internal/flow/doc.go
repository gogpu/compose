// Package flow provides pull-based frame pacing for the compose library.
//
// The compositor uses a [Controller] to decide when to request frames from each
// connected module. This implements the Wayland frame callback pattern: modules
// render only when the compositor asks, preventing them from flooding the
// compositor with unsolicited frames.
//
// The controller is passive (no goroutines, no timers). The compositor's render
// loop polls [Controller.ShouldRequest] on each tick and calls
// [Controller.FrameRequested] after sending a request to a module. When a frame
// arrives, the compositor calls [Controller.FrameDelivered]. If a module fails
// to respond in time, [Controller.FrameMissed] adaptively reduces the effective
// request rate.
//
// All exported methods are safe for concurrent use from multiple goroutines.
// The package is standalone with no internal dependencies.
package flow
