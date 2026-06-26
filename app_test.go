package main

import "testing"

// TestStripNonSpeech verifies that bracketed audio-event annotations are removed
// and tag-only transcripts (silence / noise) collapse to empty, while real
// speech is preserved.
func TestStripNonSpeech(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tag only", "[background noise]", ""},
		{"phone ringing", "[phone ringing]", ""},
		{"parens cough", "(coughs)", ""},
		{"tags around speech", "[noise] hello [music]", "hello"},
		{"trailing tag", "let's start (laughs)", "let's start"},
		{"plain speech unchanged", "merge two sorted linked lists", "merge two sorted linked lists"},
		{"empty", "", ""},
		{"collapses whitespace", "  use   two   pointers ", "use two pointers"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripNonSpeech(c.in); got != c.want {
				t.Errorf("stripNonSpeech(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
