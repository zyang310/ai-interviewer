// Browser-side mirror of the token vocabulary in internal/hotkey/keymap.go,
// used only by the Settings hotkey-capture control. It turns a KeyboardEvent
// into the same canonical hotkey string the Go backend parses (e.g. "Ctrl+Space",
// "RightAlt", "F8"). Keep this in sync with the Go keymap.

interface KeyLike {
  code: string;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  metaKey: boolean;
}

// mainKeyToken maps a non-modifier KeyboardEvent.code to its canonical token, or
// "" if the code is a modifier or unsupported. Uses e.code (physical key) so the
// result is keyboard-layout independent and matches the Go side.
function mainKeyToken(code: string): string {
  if (code === "Space") return "Space";
  if (code === "Backquote") return "`";
  if (code === "Tab") return "Tab";
  if (code === "Enter") return "Enter";
  if (/^Key[A-Z]$/.test(code)) return code.slice(3); // KeyA -> A
  if (/^Digit[0-9]$/.test(code)) return code.slice(5); // Digit1 -> 1
  if (/^F([1-9]|1[0-2])$/.test(code)) return code; // F1..F12
  return "";
}

// bareModifierFromCode maps a modifier KeyboardEvent.code to its canonical token,
// for binding a modifier on its own (e.g. RightAlt). Returns "" otherwise.
export function bareModifierFromCode(code: string): string {
  switch (code) {
    case "ControlLeft":
    case "ControlRight":
      return "Ctrl";
    case "ShiftLeft":
      return "Shift";
    case "ShiftRight":
      return "RightShift";
    case "AltLeft":
      return "Alt";
    case "AltRight":
      return "RightAlt";
    case "MetaLeft":
      return "Meta";
    case "MetaRight":
      return "RightMeta";
    default:
      return "";
  }
}

// comboFromKeyboardEvent builds a canonical hotkey string when a non-modifier key
// is pressed (with any held modifiers prepended). Returns "" while only modifiers
// are down, so the caller keeps waiting for the main key (or a bare-modifier
// key-up). Modifiers are emitted left/right-agnostic ("Ctrl", "Alt", …); the Go
// matcher accepts either physical key for those.
export function comboFromKeyboardEvent(e: KeyLike): string {
  const main = mainKeyToken(e.code);
  if (!main) return "";
  const mods: string[] = [];
  if (e.ctrlKey) mods.push("Ctrl");
  if (e.altKey) mods.push("Alt");
  if (e.shiftKey) mods.push("Shift");
  if (e.metaKey) mods.push("Meta");
  return [...mods, main].join("+");
}

// Keycap tokens for rendering a hotkey as individual keys. Right-hand modifiers
// split into a "Right" cap + the glyph so the physical key reads clearly.
// Mac-style glyphs — the global hotkey is used mainly on macOS (where it needs
// Accessibility), matching the Settings copy.
const KEYCAP_TOKENS: Record<string, string[]> = {
  Ctrl: ["⌃"],
  Alt: ["⌥"],
  Shift: ["⇧"],
  Meta: ["⌘"],
  RightAlt: ["Right", "⌥"],
  RightShift: ["Right", "⇧"],
  RightMeta: ["Right", "⌘"],
};

// hotkeyKeycaps splits a canonical hotkey string into keycap labels for display,
// e.g. "RightAlt" -> ["Right", "⌥"], "Ctrl+Space" -> ["⌃", "Space"].
export function hotkeyKeycaps(key: string): string[] {
  return key.split("+").flatMap((t) => KEYCAP_TOKENS[t] ?? [t]);
}
