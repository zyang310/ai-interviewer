import { useEffect, useState } from "react";
import { OpenAccessibilitySettings, models, hotkey } from "../../lib/wailsBridge";
import { comboFromKeyboardEvent, bareModifierFromCode, hotkeyKeycaps } from "../../lib/hotkey";
import "./PushToTalkSection.css";

interface Props {
  prefs: models.Preferences | null;
  saving: boolean;
  savePrefs: (patch: Partial<models.Preferences>, msg: string) => Promise<void>;
  // Global-hook status lives in the Settings shell (About reads it too); the
  // section re-reads it after enable/bind changes via this callback.
  hkStatus: hotkey.Status | null;
  onRefreshHotkeyStatus: () => void;
}

// PushToTalkSection is the Settings → Voice Hotkey pane: the enable toggle, the
// key-capture chip that binds a new global hotkey, and a footer strip showing
// whether the OS-level hook is live (with the macOS Accessibility shortcut).
export default function PushToTalkSection({
  prefs,
  saving,
  savePrefs,
  hkStatus,
  onRefreshHotkeyStatus,
}: Props) {
  // capturing = listening for the next keypress to bind.
  const [capturing, setCapturing] = useState(false);

  // While capturing, listen window-wide for the next key (or bare modifier) and
  // store it as the hotkey. Esc cancels. Capture-phase so it pre-empts inputs.
  useEffect(() => {
    if (!capturing) return;
    let sawMainKey = false;
    function onKeyDown(e: KeyboardEvent) {
      e.preventDefault();
      e.stopPropagation();
      if (e.key === "Escape") {
        setCapturing(false);
        return;
      }
      const combo = comboFromKeyboardEvent(e);
      if (combo) {
        sawMainKey = true;
        void commitHotkey(combo);
      }
    }
    function onKeyUp(e: KeyboardEvent) {
      e.preventDefault();
      e.stopPropagation();
      if (sawMainKey) return; // a combo already committed on key-down
      const bare = bareModifierFromCode(e.code);
      if (bare) void commitHotkey(bare);
    }
    window.addEventListener("keydown", onKeyDown, true);
    window.addEventListener("keyup", onKeyUp, true);
    return () => {
      window.removeEventListener("keydown", onKeyDown, true);
      window.removeEventListener("keyup", onKeyUp, true);
    };
  }, [capturing]);

  // Enable/disable push-to-talk, then re-read hook status (the backend starts or
  // stops the global hook in UpdatePreferences). The delayed re-read catches the
  // async hookEnabled confirmation (and, on macOS, a denied-permission outcome).
  async function savePTTEnabled(on: boolean) {
    await savePrefs(
      { pushToTalkEnabled: on },
      on ? "Push-to-talk enabled." : "Push-to-talk disabled."
    );
    onRefreshHotkeyStatus();
    setTimeout(onRefreshHotkeyStatus, 600);
  }

  // Persist a captured hotkey and stop listening.
  async function commitHotkey(spec: string) {
    setCapturing(false);
    await savePrefs({ pushToTalkKey: spec }, "Hotkey saved.");
    onRefreshHotkeyStatus();
    setTimeout(onRefreshHotkeyStatus, 600);
  }

  // Voice-hotkey footer state. Off → muted; on but (macOS) still awaiting
  // Accessibility → warning with a shortcut to grant it; otherwise the hook is live.
  const pttEnabled = !!prefs?.pushToTalkEnabled;
  const needsAccessibility =
    hkStatus?.goos === "darwin" && pttEnabled && !hkStatus.hookEnabled;
  const hotkeyStatus: { tone: "off" | "active" | "warning"; text: string } = !pttEnabled
    ? { tone: "off", text: "Voice hotkey is off" }
    : needsAccessibility
      ? { tone: "warning", text: "Needs Accessibility — enable Mogi, then relaunch" }
      : { tone: "active", text: "Global hotkey is active" };

  return (
    <>
      <header className="settings-head">
        <h1>Voice Hotkey</h1>
        <p>Press a hotkey to talk to the interviewer — even while your IDE is focused.</p>
      </header>

      {/* One consolidated card: enable toggle, key binding, footer status. */}
      <div className="settings-card hotkey-card">
        <div className="hotkey-head">
          <div className="hotkey-head-left">
            <span className="hotkey-icon">
              <span className="material-symbols-outlined">keyboard</span>
            </span>
            <div>
              <div className="hotkey-title">Enable voice hotkey</div>
              <div className="hotkey-subtitle">Toggle push-to-talk on or off</div>
            </div>
          </div>
          <div className="settings-segmented hotkey-toggle">
            <button
              type="button"
              className={`settings-segment${pttEnabled ? " active" : ""}`}
              onClick={() => savePTTEnabled(true)}
              disabled={saving || !prefs}
            >
              On
            </button>
            <button
              type="button"
              className={`settings-segment${prefs && !pttEnabled ? " active" : ""}`}
              onClick={() => savePTTEnabled(false)}
              disabled={saving || !prefs}
            >
              Off
            </button>
          </div>
        </div>

        <p className="settings-hint hotkey-desc">
          When on, press your hotkey to start recording and press it again to stop and
          send — same as the mic button. The key isn't captured exclusively, so it also
          reaches your editor; pick one your IDE ignores (a right-hand modifier or
          function key) if that's a problem.
        </p>

        <div className="hotkey-divider" />

        <div className={`hotkey-bind${pttEnabled ? "" : " is-disabled"}`}>
          <div className="hotkey-bind-head">
            <span className="material-symbols-outlined">keyboard_command_key</span>
            <span className="hotkey-bind-label">Assigned key</span>
          </div>
          <p className="settings-hint">
            Click the key field, then press the key you want — tap once to start, again
            to stop. A single right-hand modifier like <strong>Right ⌥ Option</strong>{" "}
            works best: tapping it types nothing and avoids the macOS beep a combo like
            Ctrl+Space causes. Press Esc to cancel.
          </p>
          <div className="hotkey-bind-row">
            <button
              type="button"
              className={`hotkey-chip${capturing ? " is-capturing" : ""}`}
              onClick={() => setCapturing((c) => !c)}
              disabled={saving || !prefs}
            >
              {capturing ? (
                <span className="hotkey-chip-prompt">Press a key…</span>
              ) : (
                <span className="hotkey-keycaps">
                  {hotkeyKeycaps(prefs?.pushToTalkKey || "RightAlt").map((cap, i) => (
                    <span className="hotkey-keycap" key={i}>
                      {cap}
                    </span>
                  ))}
                </span>
              )}
            </button>
            <button
              type="button"
              className="settings-link-btn"
              onClick={() => commitHotkey("RightAlt")}
              disabled={saving || !prefs}
            >
              Reset to default
            </button>
          </div>
        </div>

        <div className={`hotkey-status hotkey-status--${hotkeyStatus.tone}`}>
          <span className="hotkey-status-dot" />
          <span className="hotkey-status-text">{hotkeyStatus.text}</span>
          {hotkeyStatus.tone === "warning" && (
            <button
              type="button"
              className="hotkey-status-action"
              onClick={() => OpenAccessibilitySettings()}
              disabled={saving}
            >
              Open settings
            </button>
          )}
        </div>
      </div>
    </>
  );
}
