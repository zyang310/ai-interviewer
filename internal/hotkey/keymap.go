// Package hotkey runs a global, passive keyboard hook (via robotn/gohook) and
// emits a Wails "ptt:down" event on each press of a configured voice hotkey; the
// frontend toggles recording on that event. "Passive" means it observes
// keystrokes but never consumes them, so the focused app (the IDE) still gets them.
package hotkey

import (
	"fmt"
	"strings"

	hook "github.com/robotn/gohook"
)

// DefaultSpec is the out-of-the-box push-to-talk hotkey. A bare right-hand
// modifier is chosen deliberately: held alone it types nothing, triggers no
// app shortcut, and avoids the macOS "unhandled key" alert beep that a combo
// like Ctrl+Space causes (the hook is passive — the key still reaches the IDE).
const DefaultSpec = "RightAlt"

// Token is a stable, human-readable key name persisted in Preferences and shared
// verbatim with the frontend (see frontend/src/lib/hotkey.ts). Examples: "Ctrl",
// "Space", "F8", "RightAlt". A hotkey is stored as tokens joined by "+", with
// modifiers first, e.g. "Ctrl+Space".
type Token string

// modifierTokens is the set of tokens allowed to act as the modifier half of a
// combo. (Bare modifiers may also stand alone as the main key, e.g. "RightAlt".)
var modifierTokens = map[Token]bool{
	"Ctrl": true, "Alt": true, "Shift": true, "Meta": true,
	"RightAlt": true, "RightShift": true, "RightMeta": true,
}

// codesByToken maps each supported token to the gohook keycodes that satisfy it.
// A token may map to several codes — "Ctrl"/"Alt"/"Shift"/"Meta" match both the
// left- and right-hand physical keys. Populated at init() from gohook's portable
// keycode map (which is what Event.Keycode is reported against) so we never
// hardcode raw integers and stay correct across macOS and Windows.
var codesByToken map[Token][]uint16

func init() {
	k := hook.Keycode // map[string]uint16 of portable codes matching Event.Keycode
	codesByToken = map[Token][]uint16{
		// Modifiers. gohook exposes no distinct right-Ctrl code, so "Ctrl" is the
		// single Ctrl key; the others match left+right.
		"Ctrl":  {k["ctrl"]},
		"Shift": {k["shift"], k["rshift"]},
		"Alt":   {k["alt"], k["ralt"]},
		"Meta":  {k["cmd"], k["rcmd"]},
		// Right-hand modifiers, usable as bare push-to-talk keys.
		"RightShift": {k["rshift"]},
		"RightAlt":   {k["ralt"]},
		"RightMeta":  {k["rcmd"]},
		// Common standalone keys.
		"Space": {k["space"]},
		"Tab":   {k["tab"]},
		"Esc":   {k["esc"]},
		"Enter": {k["enter"]},
		"`":     {k["`"]},
	}
	// Letters A–Z.
	for c := 'a'; c <= 'z'; c++ {
		codesByToken[Token(strings.ToUpper(string(c)))] = []uint16{k[string(c)]}
	}
	// Digits 0–9.
	for d := '0'; d <= '9'; d++ {
		codesByToken[Token(string(d))] = []uint16{k[string(d)]}
	}
	// Function keys F1–F12.
	for i := 1; i <= 12; i++ {
		codesByToken[Token(fmt.Sprintf("F%d", i))] = []uint16{k[fmt.Sprintf("f%d", i)]}
	}
}

// Spec is a parsed hotkey: zero or more modifier tokens plus a single main key.
// A bare modifier (e.g. "RightAlt") parses with that modifier as Key and no Mods.
type Spec struct {
	Mods []Token
	Key  Token
}

// ParseSpec parses a canonical hotkey string ("Ctrl+Space", "F8", "RightAlt").
// The last token is the main key; any preceding tokens must be modifiers.
func ParseSpec(s string) (Spec, error) {
	var toks []Token
	for _, p := range strings.Split(strings.TrimSpace(s), "+") {
		if p = strings.TrimSpace(p); p != "" {
			toks = append(toks, Token(p))
		}
	}
	if len(toks) == 0 {
		return Spec{}, fmt.Errorf("hotkey: empty spec %q", s)
	}
	for _, t := range toks {
		if _, ok := codesByToken[t]; !ok {
			return Spec{}, fmt.Errorf("hotkey: unknown key %q", t)
		}
	}
	mods := toks[:len(toks)-1]
	for _, m := range mods {
		if !modifierTokens[m] {
			return Spec{}, fmt.Errorf("hotkey: %q cannot be a modifier", m)
		}
	}
	return Spec{Mods: mods, Key: toks[len(toks)-1]}, nil
}

// String renders the canonical persisted form, e.g. "Ctrl+Space".
func (s Spec) String() string {
	parts := make([]string, 0, len(s.Mods)+1)
	for _, m := range s.Mods {
		parts = append(parts, string(m))
	}
	if s.Key != "" {
		parts = append(parts, string(s.Key))
	}
	return strings.Join(parts, "+")
}

// Label renders an OS-appropriate display string (⌃⌥⇧⌘ on macOS; Ctrl/Alt/Shift/
// Win on other platforms). goos is a runtime.GOOS value.
func (s Spec) Label(goos string) string {
	sym := func(t Token) string {
		if goos == "darwin" {
			switch t {
			case "Ctrl":
				return "⌃"
			case "Alt", "RightAlt":
				return "⌥"
			case "Shift", "RightShift":
				return "⇧"
			case "Meta", "RightMeta":
				return "⌘"
			}
		} else {
			switch t {
			case "Meta", "RightMeta":
				return "Win"
			case "RightAlt":
				return "Alt"
			case "RightShift":
				return "Shift"
			}
		}
		return string(t)
	}
	parts := make([]string, 0, len(s.Mods)+1)
	for _, m := range s.Mods {
		parts = append(parts, sym(m))
	}
	if s.Key != "" {
		parts = append(parts, sym(s.Key))
	}
	sep := " + "
	if goos == "darwin" {
		sep = " "
	}
	return strings.Join(parts, sep)
}

// requiredTokens is the set of tokens that must all be held for the hotkey to be
// active (modifiers plus the main key).
func (s Spec) requiredTokens() []Token {
	toks := make([]Token, 0, len(s.Mods)+1)
	toks = append(toks, s.Mods...)
	if s.Key != "" {
		toks = append(toks, s.Key)
	}
	return toks
}

// tokenForKeycode returns the spec's required token satisfied by the given
// gohook keycode, or "" if the keycode is not part of the spec.
func tokenForKeycode(spec Spec, code uint16) Token {
	for _, t := range spec.requiredTokens() {
		for _, c := range codesByToken[t] {
			if c == code {
				return t
			}
		}
	}
	return ""
}
