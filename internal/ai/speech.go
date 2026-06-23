package ai

import (
	"regexp"
	"strings"
)

// Precompiled markdown patterns stripped before speech synthesis.
var (
	reCodeFence  = regexp.MustCompile("(?s)```.*?```")           // fenced code blocks
	reImage      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)  // ![alt](url)
	reLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)   // [text](url)
	reHeading    = regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]*`) // ## Heading
	reBlockquote = regexp.MustCompile(`(?m)^[ \t]*>[ \t]?`)      // > quote
	reListBullet = regexp.MustCompile(`(?m)^[ \t]*[-*+][ \t]+`)  // - item
	reListNumber = regexp.MustCompile(`(?m)^[ \t]*\d+\.[ \t]+`)  // 1. item
	reStrike     = regexp.MustCompile(`~~([^~\n]+)~~`)           // ~~text~~
	// Bold/italic: markers must hug non-space text so "a * b" (multiplication)
	// isn't mistaken for emphasis.
	reEmphasis   = regexp.MustCompile(`\*{1,3}([^*\s][^*\n]*[^*\s]|[^*\s])\*{1,3}`)
	reWhitespace = regexp.MustCompile(`\s+`)
)

// SanitizeForSpeech converts the model's markdown-ish reply into plain text
// suitable for text-to-speech. Both ElevenLabs and Google read raw markdown
// punctuation (backticks, asterisks, list markers, …) aloud, so we strip the
// formatting and keep only the words. The chat UI still renders the original
// markdown; only the spoken copy is cleaned. If stripping leaves nothing (e.g. a
// reply that was only a code block), the original text is returned so synthesis
// still has something to say.
func SanitizeForSpeech(text string) string {
	out := text
	out = reCodeFence.ReplaceAllString(out, " ")  // drop code blocks — unreadable aloud
	out = reImage.ReplaceAllString(out, "$1")     // ![alt](url) -> alt
	out = reLink.ReplaceAllString(out, "$1")      // [text](url) -> text
	out = reHeading.ReplaceAllString(out, "")     // ## Heading -> Heading
	out = reBlockquote.ReplaceAllString(out, "")  // > quote -> quote
	out = reListBullet.ReplaceAllString(out, "")  // - item -> item
	out = reListNumber.ReplaceAllString(out, "")  // 1. item -> item
	out = reStrike.ReplaceAllString(out, "$1")    // ~~x~~ -> x
	out = reEmphasis.ReplaceAllString(out, "$1")  // **x** / *x* -> x
	out = strings.ReplaceAll(out, "`", "")        // remaining inline backticks
	out = reWhitespace.ReplaceAllString(out, " ") // collapse newlines/spaces
	out = strings.TrimSpace(out)
	if out == "" {
		return strings.TrimSpace(text)
	}
	return out
}
