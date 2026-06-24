package hotkey

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	hook "github.com/robotn/gohook"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// edge is the transition produced by feeding one key event to a matcher.
type edge int

const (
	edgeNone edge = iota
	edgeDown      // the combo just became fully held
	edgeUp        // the combo was fully held and a required key was released
)

// matcher tracks which of a spec's required tokens are currently held and
// reports edge transitions. It is pure (no OS hook), so the combo logic and
// auto-repeat debounce are unit-testable without the global keyboard hook.
type matcher struct {
	required []Token
	held     map[Token]bool
	active   bool
}

func newMatcher(spec Spec) *matcher {
	return &matcher{required: spec.requiredTokens(), held: map[Token]bool{}}
}

// feed updates the held-state for token tok (down=true on KeyDown) and returns
// the resulting edge. Repeated downs for an already-held key (OS auto-repeat) or
// duplicate ups produce edgeNone, which is the debounce.
func (m *matcher) feed(tok Token, down bool) edge {
	if tok == "" || m.held[tok] == down {
		return edgeNone // unknown key, or no state change
	}
	m.held[tok] = down
	all := m.allHeld()
	switch {
	case all && !m.active:
		m.active = true
		return edgeDown
	case !all && m.active:
		m.active = false
		return edgeUp
	}
	return edgeNone
}

// allHeld reports whether every required token is currently held down.
func (m *matcher) allHeld() bool {
	for _, t := range m.required {
		if !m.held[t] {
			return false
		}
	}
	return true
}

// Status is a snapshot of the listener for the Settings UI — chiefly to drive
// the macOS Input-Monitoring permission hint.
type Status struct {
	Running     bool   `json:"running"`     // hook is up and push-to-talk is enabled
	HookEnabled bool   `json:"hookEnabled"` // the OS hook has delivered at least one event
	Spec        string `json:"spec"`        // canonical hotkey, e.g. "Ctrl+Space"
	Label       string `json:"label"`       // OS-appropriate display label
	Goos        string `json:"goos"`        // runtime.GOOS
	Error       string `json:"error,omitempty"`
}

// Listener runs a single global, passive keyboard hook for the lifetime of the
// app and emits a Wails "ptt:down" event on each press (the rising edge) of the
// configured hotkey. Key releases are tracked only to re-arm for the next press.
//
// The OS hook (libuiohook, via gohook) is started AT MOST ONCE and torn down
// only at shutdown. Enabling/disabling and rebinding the hotkey just swap guarded
// fields that the hook goroutine reads — never a restart. This is deliberate:
// libuiohook keeps global state and its macOS event-tap teardown is asynchronous,
// so a Stop-then-Start cycle races the C layer and segfaults. All exported
// methods are safe for concurrent use.
type Listener struct {
	mu          sync.Mutex
	ctx         context.Context // app context for EventsEmit
	cancel      context.CancelFunc
	done        chan struct{}
	started     bool     // hook.Start() has been called (lifetime, not per-config)
	enabled     bool     // emit events for matched edges?
	spec        Spec     // the hotkey to match
	m           *matcher // matcher for the current spec; swapped on rebind
	hookEnabled bool     // the OS hook has delivered ≥1 event (proof it's live)
	lastErr     string
}

// New creates an idle Listener. Call Apply to configure and start it.
func New() *Listener { return &Listener{} }

// Apply makes the listener match the given configuration. It starts the OS hook
// the first time push-to-talk is enabled and never restarts it: a later rebind or
// enable/disable just swaps the guarded spec/enabled fields the running goroutine
// reads. ctx is the Wails app context used to emit events.
func (l *Listener) Apply(ctx context.Context, enabled bool, spec Spec) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.spec = spec
	l.m = newMatcher(spec)
	l.enabled = enabled
	if enabled && !l.started {
		l.ctx = ctx
		l.started = true
		l.done = make(chan struct{})
		hookCtx, cancel := context.WithCancel(ctx)
		l.cancel = cancel
		go l.loop(hookCtx)
	}
}

// Shutdown stops the OS hook and waits for the goroutine to exit. Called once at
// app shutdown — this is the only hook.End() in the app's lifetime.
func (l *Listener) Shutdown() {
	l.mu.Lock()
	if !l.started {
		l.mu.Unlock()
		return
	}
	cancel, done := l.cancel, l.done
	l.started = false
	l.cancel = nil
	l.mu.Unlock()

	cancel()
	<-done
}

// Status returns a snapshot for the frontend.
func (l *Listener) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Status{
		Running:     l.started && l.enabled,
		HookEnabled: l.hookEnabled,
		Spec:        l.spec.String(),
		Label:       l.spec.Label(runtime.GOOS),
		Goos:        runtime.GOOS,
		Error:       l.lastErr,
	}
}

// loop is the single hook goroutine: it reads the global event stream and emits a
// "ptt:down" Wails event on each press of the currently-configured hotkey.
// spec/enabled are read under the mutex per event so a live rebind takes effect
// without a restart. KeyHold (OS auto-repeat) is ignored and the matcher re-arms
// on release, so each physical press emits exactly one "ptt:down".
func (l *Listener) loop(ctx context.Context) {
	defer close(l.done)
	defer hook.End() // the one and only teardown, as the goroutine unwinds
	defer func() {
		// libuiohook can panic on some teardown/permission paths — never crash
		// the host app; record it for Status instead.
		if r := recover(); r != nil {
			l.mu.Lock()
			l.lastErr = fmt.Sprintf("hotkey hook stopped: %v", r)
			l.started = false
			l.mu.Unlock()
		}
	}()

	confirmed := false
	evChan := hook.Start() // begins the global hook (needs Input Monitoring on macOS)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-evChan:
			if !ok {
				return // channel closed by hook.End()
			}
			if !confirmed {
				// The first delivered event proves the hook is live — and on
				// macOS, that Input Monitoring was granted (denied ⇒ no events
				// at all). Drives the Settings permission hint.
				confirmed = true
				l.mu.Lock()
				l.hookEnabled = true
				l.mu.Unlock()
			}
			if ev.Kind != hook.KeyDown && ev.Kind != hook.KeyUp {
				continue // ignore HookEnabled, mouse, and KeyHold (auto-repeat)
			}
			// Match against the current spec under the lock so a concurrent
			// rebind (Apply) is observed atomically; release before emitting.
			l.mu.Lock()
			e := edgeNone
			if l.enabled && l.m != nil {
				if tok := tokenForKeycode(l.spec, ev.Keycode); tok != "" {
					e = l.m.feed(tok, ev.Kind == hook.KeyDown)
				}
			}
			l.mu.Unlock()
			// Toggle UX: emit only on the press (rising edge). The matcher still
			// tracks releases internally (edgeUp) to re-arm for the next press, but
			// the frontend acts on presses only, so no release event is sent.
			if e == edgeDown {
				wruntime.EventsEmit(ctx, "ptt:down")
			}
		}
	}
}
