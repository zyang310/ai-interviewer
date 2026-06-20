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

// DisplayInfo describes one active display, used to populate the region picker.
type DisplayInfo struct {
	Index  int    `json:"index"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Label  string `json:"label"`
}

// Capturer periodically takes a screenshot and keeps the latest one in memory.
// It captures a user-defined region (a fraction of a chosen display); when the
// region width is zero it falls back to the full display. All exported methods
// are safe for concurrent use.
type Capturer struct {
	mu     sync.RWMutex
	latest string // base64-encoded PNG, empty until first capture
	cancel context.CancelFunc
	done   chan struct{}

	// Region to capture, expressed as fractions (0..1) of the display so the
	// selection survives Retina/resolution scaling. rw <= 0 means full display.
	displayIndex   int
	rx, ry, rw, rh float64
}

// NewCapturer creates a Capturer but does not start it.
func NewCapturer() *Capturer {
	return &Capturer{}
}

// SetRegion configures which display and sub-region to capture. Coordinates are
// fractions (0..1) of the display. A width <= 0 means "capture the full display".
func (c *Capturer) SetRegion(displayIndex int, x, y, w, h float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.displayIndex = displayIndex
	c.rx, c.ry, c.rw, c.rh = x, y, w, h
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
	c.mu.RLock()
	idx, x, y, w, h := c.displayIndex, c.rx, c.ry, c.rw, c.rh
	c.mu.RUnlock()

	encoded, err := grab(idx, x, y, w, h)
	if err != nil {
		// Non-fatal: keep the previous screenshot.
		return
	}

	c.mu.Lock()
	c.latest = encoded
	c.mu.Unlock()
}

// grab captures the given fractional region of a display and returns a
// base64-encoded PNG. A width <= 0 captures the full display.
func grab(displayIndex int, fx, fy, fw, fh float64) (string, error) {
	rect, err := regionRect(displayIndex, fx, fy, fw, fh)
	if err != nil {
		return "", err
	}

	img, err := screenshot.CaptureRect(rect)
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

// regionRect maps a fractional region of a display to absolute virtual-screen
// coordinates. Fractions are clamped to [0,1]; a width <= 0 selects the full
// display. The rect math is scale-invariant, so it is correct on Retina too.
func regionRect(displayIndex int, fx, fy, fw, fh float64) (image.Rectangle, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return image.Rectangle{}, fmt.Errorf("capture: no active displays found")
	}
	if displayIndex < 0 || displayIndex >= n {
		displayIndex = 0
	}

	b := screenshot.GetDisplayBounds(displayIndex)
	if fw <= 0 || fh <= 0 {
		return b, nil // full display
	}

	fx = clamp01(fx)
	fy = clamp01(fy)
	fw = clamp01(fw)
	fh = clamp01(fh)
	if fx+fw > 1 {
		fw = 1 - fx
	}
	if fy+fh > 1 {
		fh = 1 - fy
	}

	dw, dh := float64(b.Dx()), float64(b.Dy())
	rect := image.Rect(
		b.Min.X+int(fx*dw),
		b.Min.Y+int(fy*dh),
		b.Min.X+int((fx+fw)*dw),
		b.Min.Y+int((fy+fh)*dh),
	)
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return b, nil // degenerate selection: fall back to full display
	}
	return rect, nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ListDisplays enumerates all active displays for the region picker.
func ListDisplays() []DisplayInfo {
	n := screenshot.NumActiveDisplays()
	out := make([]DisplayInfo, 0, n)
	for i := 0; i < n; i++ {
		b := screenshot.GetDisplayBounds(i)
		out = append(out, DisplayInfo{
			Index:  i,
			X:      b.Min.X,
			Y:      b.Min.Y,
			Width:  b.Dx(),
			Height: b.Dy(),
			Label:  fmt.Sprintf("Display %d (%d×%d)", i+1, b.Dx(), b.Dy()),
		})
	}
	return out
}

// SnapshotDisplay captures a single full (uncropped) display as a base64 PNG,
// used by the region selector so the user can draw a rectangle over it.
func SnapshotDisplay(displayIndex int) (string, error) {
	return grab(displayIndex, 0, 0, 0, 0)
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

// CaptureOnce takes a single full-primary-display screenshot immediately.
// Returns a base64-encoded PNG or an error.
func CaptureOnce() (string, error) {
	return grab(0, 0, 0, 0, 0)
}
