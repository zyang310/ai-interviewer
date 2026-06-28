// cleanForDisplay strips markdown punctuation from an interviewer reply so the
// chat bubble renders clean prose instead of raw symbols. The interviewer is
// meant to speak in plain text (its replies are also read aloud by TTS), but the
// model can still emit backticks, asterisks, headings, or list markers — and the
// chat has no markdown renderer, so those would otherwise show literally.
//
// This is the display-side mirror of the Go SanitizeForSpeech
// (internal/ai/speech.go), with one difference: speech collapses every newline
// into a space, while here we keep line breaks (the bubble uses
// white-space: pre-wrap) so any stray list still reads as separate lines rather
// than a run-on sentence.
export function cleanForDisplay(text: string): string {
  let out = text;
  out = out.replace(/```[a-zA-Z0-9]*\n?/g, ""); // drop code-fence markers, keep inner text
  out = out.replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1"); // ![alt](url) -> alt
  out = out.replace(/\[([^\]]*)\]\([^)]*\)/g, "$1"); // [text](url) -> text
  out = out.replace(/^[ \t]*#{1,6}[ \t]*/gm, ""); // ## Heading -> Heading
  out = out.replace(/^[ \t]*>[ \t]?/gm, ""); // > quote -> quote
  out = out.replace(/^[ \t]*[-*+][ \t]+/gm, ""); // - item -> item
  out = out.replace(/^[ \t]*\d+\.[ \t]+/gm, ""); // 1. item -> item
  out = out.replace(/~~([^~\n]+)~~/g, "$1"); // ~~x~~ -> x
  // Bold/italic: markers must hug non-space text so "a * b" (multiplication) survives.
  out = out.replace(/\*{1,3}([^*\s][^*\n]*[^*\s]|[^*\s])\*{1,3}/g, "$1");
  out = out.replace(/`/g, ""); // remaining inline backticks
  out = out.replace(/^[ \t]+/gm, ""); // drop leading indentation per line
  out = out.replace(/[ \t]{2,}/g, " "); // collapse intra-line whitespace runs
  out = out.replace(/[ \t]+$/gm, ""); // trim trailing spaces per line
  out = out.replace(/\n{2,}/g, "\n"); // collapse blank-line runs
  return out.trim();
}
