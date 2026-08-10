package socket

import "errors"

// ErrPayloadTooLarge reports a frame whose declared payload exceeds the
// transport's allocation limit.
var ErrPayloadTooLarge = errors.New("socket: payload too large")
