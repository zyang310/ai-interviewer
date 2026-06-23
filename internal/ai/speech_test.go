package ai

import "testing"

func TestSanitizeForSpeech(t *testing.T) {
	cases := map[string]string{
		// The reported bug: backticks read aloud.
		"Use a `hashmap` here":            "Use a hashmap here",
		"Can you get `O(1)` lookup?":      "Can you get O(1) lookup?",
		"What's the **time complexity**?": "What's the time complexity?",
		"That's *interesting* — why?":     "That's interesting — why?",
		"~~wrong~~ try again":             "wrong try again",
		// Links and images keep their visible text.
		"See [the docs](http://x) for more": "See the docs for more",
		// Headings, quotes, and list markers are removed.
		"## Hint\nthink about it":   "Hint think about it",
		"> consider the empty case": "consider the empty case",
		"- first\n- second":         "first second",
		"1. start\n2. then":         "start then",
		// Multiplication (spaces around *) is left intact, not treated as emphasis.
		"is it a * b or a + b?": "is it a * b or a + b?",
		// Plain text is unchanged.
		"What is the time complexity?": "What is the time complexity?",
	}
	for input, want := range cases {
		if got := SanitizeForSpeech(input); got != want {
			t.Errorf("SanitizeForSpeech(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSanitizeForSpeechDropsCodeBlocks(t *testing.T) {
	in := "Consider this:\n```py\nx = 1\n```\nWhat does it cost?"
	want := "Consider this: What does it cost?"
	if got := SanitizeForSpeech(in); got != want {
		t.Errorf("SanitizeForSpeech(code block) = %q, want %q", got, want)
	}
}
