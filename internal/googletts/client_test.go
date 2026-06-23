package googletts

import "testing"

func TestLangCodeFromVoice(t *testing.T) {
	cases := map[string]string{
		"en-US-Neural2-F":          "en-US",
		"en-GB-Wavenet-A":          "en-GB",
		"en-US-Chirp3-HD-Achernar": "en-US",
		"en-US-Studio-O":           "en-US",
		"weird":                    "en-US", // no hyphens → fallback
		"":                         "en-US",
	}
	for input, want := range cases {
		if got := langCodeFromVoice(input); got != want {
			t.Errorf("langCodeFromVoice(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestVoiceFamily(t *testing.T) {
	cases := map[string]string{
		"en-US-Neural2-F":          "Neural2",
		"en-US-Wavenet-A":          "WaveNet",
		"en-US-Studio-O":           "Studio",
		"en-US-Chirp3-HD-Achernar": "Chirp3 HD",
		"en-US-Chirp-HD-F":         "Chirp HD",
		"en-US-Standard-A":         "", // lower tier, filtered out
		"en-US-Polyglot-1":         "", // filtered out
	}
	for input, want := range cases {
		if got := voiceFamily(input); got != want {
			t.Errorf("voiceFamily(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseSTTResponse(t *testing.T) {
	cases := map[string]string{
		// Single result.
		`{"results":[{"alternatives":[{"transcript":"hello world"}]}]}`: "hello world",
		// Multiple results are joined with a space; only the top alternative is used.
		`{"results":[{"alternatives":[{"transcript":"hello "}]},{"alternatives":[{"transcript":" again"},{"transcript":"ignored"}]}]}`: "hello again",
		// Empty / no results.
		`{}`:             "",
		`{"results":[]}`: "",
	}
	for input, want := range cases {
		got, err := parseSTTResponse([]byte(input))
		if err != nil {
			t.Errorf("parseSTTResponse(%q) returned error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("parseSTTResponse(%q) = %q, want %q", input, got, want)
		}
	}

	if _, err := parseSTTResponse([]byte("not json")); err == nil {
		t.Error("parseSTTResponse(invalid json) = nil error, want error")
	}
}

func TestIsEnglish(t *testing.T) {
	if !isEnglish([]string{"en-US"}) {
		t.Error("isEnglish([en-US]) = false, want true")
	}
	if !isEnglish([]string{"fr-FR", "en-GB"}) {
		t.Error("isEnglish([fr-FR, en-GB]) = false, want true")
	}
	if isEnglish([]string{"fr-FR", "de-DE"}) {
		t.Error("isEnglish([fr-FR, de-DE]) = true, want false")
	}
	if isEnglish(nil) {
		t.Error("isEnglish(nil) = true, want false")
	}
}
