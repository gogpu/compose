package compose

// defaultMaxModules is the default maximum number of concurrent module
// connections a Server will accept.
const defaultMaxModules = 16

// defaultFPS is the default frames-per-second for a Client that does not
// specify WithFPS.
const defaultFPS = 1

// ServerOption configures a Server. Use With* functions to create options.
type ServerOption func(*serverConfig)

// ClientOption configures a Client. Use With* functions to create options.
type ClientOption func(*clientConfig)

type serverConfig struct {
	maxModules  int
	compression string // "", "raw", "lz4"
}

type clientConfig struct {
	name   string
	width  uint32
	height uint32
	fps    uint16
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		maxModules:  defaultMaxModules,
		compression: "",
	}
}

func defaultClientConfig() clientConfig {
	return clientConfig{
		name:   "module",
		width:  400,
		height: 300,
		fps:    defaultFPS,
	}
}

// WithMaxModules sets the maximum number of concurrent module connections
// the Server will accept. Values less than 1 are clamped to 1.
// Default: 16.
func WithMaxModules(n int) ServerOption {
	return func(c *serverConfig) {
		if n < 1 {
			n = 1
		}
		c.maxModules = n
	}
}

// WithCompression enables frame payload compression on the Server.
// Supported values: "lz4". Any other value (including "") uses raw
// pass-through (no compression).
// Default: no compression.
func WithCompression(algo string) ServerOption {
	return func(c *serverConfig) {
		c.compression = algo
	}
}

// WithName sets the human-readable module name sent during the handshake.
// The name is used for logging, slot assignment, and module identification.
// Names longer than 63 bytes are silently truncated.
// Default: "module".
func WithName(name string) ClientOption {
	return func(c *clientConfig) {
		c.name = name
	}
}

// WithFrameSize sets the initial frame dimensions in pixels.
// The compositor may acknowledge different dimensions during handshake.
// Default: 400x300.
func WithFrameSize(width, height uint32) ClientOption {
	return func(c *clientConfig) {
		c.width = width
		c.height = height
	}
}

// WithFPS sets the module's preferred frame rate.
// A value of 0 defaults to 1 FPS (suitable for static content).
// Default: 1.
func WithFPS(fps uint16) ClientOption {
	return func(c *clientConfig) {
		c.fps = fps
	}
}
