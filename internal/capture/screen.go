package capture

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
	"golang.org/x/image/draw"
)

const maxWidthPx = 1280 // resize screenshots wider than this before encoding

// Capturer periodically takes a screenshot and keeps the latest one in memory.
// All exported methods are safe for concurrent use.
type Capturer struct {
	mu     sync.RWMutex
	latest string // base64-encoded PNG, empty until first capture
	cancel context.CancelFunc
	done   chan struct{}
}

// NewCapturer creates a Capturer but does not start it.
func NewCapturer() *Capturer {
	return &Capturer{}
}

// Start begins periodic screen capture at the given interval.
// Calling Start again while already running is a no-op.
func (c *Capturer) Start(ctx context.Context, intervalMs int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		return // already running
	}

	captureCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})

	go c.loop(captureCtx, time.Duration(intervalMs)*time.Millisecond)
}

// Stop halts the capture loop and waits for the goroutine to exit.
func (c *Capturer) Stop() {
	c.mu.Lock()
	if c.cancel == nil {
		c.mu.Unlock()
		return
	}
	cancel := c.cancel
	done := c.done
	c.cancel = nil
	c.mu.Unlock()

	cancel()
	<-done
}

// Latest returns the most recent screenshot as a base64-encoded PNG string.
// Returns an empty string if no screenshot has been captured yet.
func (c *Capturer) Latest() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

func (c *Capturer) loop(ctx context.Context, interval time.Duration) {
	defer close(c.done)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Capture immediately on start, then on each tick.
	c.capture()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.capture()
		}
	}
}

func (c *Capturer) capture() {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return
	}

	// Capture the primary display (display 0).
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		// Non-fatal: keep the previous screenshot.
		return
	}

	resized := resizeIfNeeded(img)

	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	c.mu.Lock()
	c.latest = encoded
	c.mu.Unlock()
}

// resizeIfNeeded returns img unchanged if it fits within maxWidthPx, otherwise
// returns a new image scaled down proportionally using bilinear interpolation.
func resizeIfNeeded(img image.Image) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	if w <= maxWidthPx {
		return img
	}

	h := bounds.Dy()
	newW := maxWidthPx
	newH := int(float64(h) * float64(newW) / float64(w))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}

// CaptureOnce takes a single screenshot immediately (used for on-demand capture).
// Returns a base64-encoded PNG or an error.
func CaptureOnce() (string, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return "", fmt.Errorf("capture: no active displays found")
	}

	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return "", fmt.Errorf("capture: screenshot failed: %w", err)
	}

	resized := resizeIfNeeded(img)

	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return "", fmt.Errorf("capture: png encode: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
