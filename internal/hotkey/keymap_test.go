package hotkey

import (
	"testing"

	hook "github.com/robotn/gohook"
)

func TestParseSpecRoundTrip(t *testing.T) {
	cases := map[string]Spec{
		"Ctrl+Space":   {Mods: []Token{"Ctrl"}, Key: "Space"},
		"F8":           {Key: "F8"},
		"RightAlt":     {Key: "RightAlt"},
		"Ctrl+Shift+Z": {Mods: []Token{"Ctrl", "Shift"}, Key: "Z"},
	}
	for in, want := range cases {
		got, err := ParseSpec(in)
		if err != nil {
			t.Fatalf("ParseSpec(%q) error: %v", in, err)
		}
		if got.Key != want.Key || len(got.Mods) != len(want.Mods) {
			t.Errorf("ParseSpec(%q) = %+v, want %+v", in, got, want)
		}
		if got.String() != in {
			t.Errorf("String() = %q, want %q", got.String(), in)
		}
	}
}

func TestParseSpecErrors(t *testing.T) {
	for _, in := range []string{"", "Bogus", "Space+Ctrl"} {
		if _, err := ParseSpec(in); err == nil {
			t.Errorf("ParseSpec(%q) expected error, got nil", in)
		}
	}
}

func TestLabel(t *testing.T) {
	s, _ := ParseSpec("Ctrl+Space")
	if got := s.Label("darwin"); got != "⌃ Space" {
		t.Errorf("Label(darwin) = %q, want %q", got, "⌃ Space")
	}
	if got := s.Label("windows"); got != "Ctrl + Space" {
		t.Errorf("Label(windows) = %q, want %q", got, "Ctrl + Space")
	}
}

func TestTokenForKeycode(t *testing.T) {
	s, _ := ParseSpec("Ctrl+Space")
	if got := tokenForKeycode(s, hook.Keycode["ctrl"]); got != "Ctrl" {
		t.Errorf("ctrl keycode -> %q, want Ctrl", got)
	}
	if got := tokenForKeycode(s, hook.Keycode["space"]); got != "Space" {
		t.Errorf("space keycode -> %q, want Space", got)
	}
	if got := tokenForKeycode(s, hook.Keycode["a"]); got != "" {
		t.Errorf("unrelated keycode -> %q, want empty", got)
	}
}

func TestMatcherCombo(t *testing.T) {
	s, _ := ParseSpec("Ctrl+Space")
	m := newMatcher(s)

	if e := m.feed("Ctrl", true); e != edgeNone {
		t.Errorf("Ctrl down: got %v, want edgeNone", e)
	}
	if e := m.feed("Space", true); e != edgeDown {
		t.Errorf("Space down: got %v, want edgeDown", e)
	}
	// Auto-repeat: duplicate downs while held must not re-fire.
	if e := m.feed("Ctrl", true); e != edgeNone {
		t.Errorf("Ctrl repeat: got %v, want edgeNone", e)
	}
	if e := m.feed("Space", true); e != edgeNone {
		t.Errorf("Space repeat: got %v, want edgeNone", e)
	}
	// Releasing the modifier first still ends the combo.
	if e := m.feed("Ctrl", false); e != edgeUp {
		t.Errorf("Ctrl up: got %v, want edgeUp", e)
	}
	if e := m.feed("Space", false); e != edgeNone {
		t.Errorf("Space up: got %v, want edgeNone", e)
	}
}

func TestMatcherSingleKeyReusable(t *testing.T) {
	s, _ := ParseSpec("F8")
	m := newMatcher(s)
	for i := 0; i < 2; i++ {
		if e := m.feed("F8", true); e != edgeDown {
			t.Errorf("cycle %d F8 down: got %v, want edgeDown", i, e)
		}
		if e := m.feed("F8", true); e != edgeNone {
			t.Errorf("cycle %d F8 repeat: got %v, want edgeNone", i, e)
		}
		if e := m.feed("F8", false); e != edgeUp {
			t.Errorf("cycle %d F8 up: got %v, want edgeUp", i, e)
		}
	}
	if e := m.feed("", true); e != edgeNone {
		t.Errorf("empty token: got %v, want edgeNone", e)
	}
}
