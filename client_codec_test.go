package compose

import (
	"bytes"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestClientSetCompressionConcurrentPublishFrame(t *testing.T) {
	addr := tempSocket(t)

	srv, err := Listen(addr)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	const (
		width  = 32
		height = 32
		frames = 128
	)
	expectedPixels := makePixels(width, height, 0xA5)
	var received atomic.Int64
	var invalid atomic.Bool
	srv.OnFrame(func(f Frame) {
		if f.Width != width || f.Height != height || !bytes.Equal(f.Pixels, expectedPixels) {
			invalid.Store(true)
		}
		received.Add(1)
	})

	client, err := Dial(addr, WithName("codec-race"), WithFrameSize(width, height))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	start := make(chan struct{})
	errs := make(chan error, frames)
	var wg sync.WaitGroup
	wg.Add(2)

	// Publish and switch codecs from separate goroutines. The scheduler hint
	// keeps both operations interleaved while retaining deterministic inputs.
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < frames; i++ {
			if err := client.PublishFrame(Frame{
				Pixels: expectedPixels,
				Width:  width,
				Height: height,
			}); err != nil {
				errs <- err
			}
			runtime.Gosched()
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < frames*2; i++ {
			if i%2 == 0 {
				client.SetCompression("lz4")
			} else {
				client.SetCompression("raw")
			}
			runtime.Gosched()
		}
	}()

	close(start)
	wg.Wait()
	close(errs)
	for publishErr := range errs {
		t.Errorf("PublishFrame: %v", publishErr)
	}

	if !waitFor(t, func() bool { return received.Load() == frames }) {
		t.Fatalf("received %d/%d frames", received.Load(), frames)
	}
	if invalid.Load() {
		t.Fatal("received frame did not match the published payload")
	}
}
