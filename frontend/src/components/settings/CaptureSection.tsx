import { models } from "../../lib/wailsBridge";

interface Props {
  // Interval input state lives in the Settings shell so "Clear all data" can
  // reset it centrally and unsaved edits survive switching sections.
  intervalSec: string;
  setIntervalSec: (v: string) => void;
  prefs: models.Preferences | null;
  saving: boolean;
  savePrefs: (patch: Partial<models.Preferences>, msg: string) => Promise<void>;
}

// CaptureSection is the Settings → Capture Prefs pane: how often the app sends
// a fresh screenshot to the interviewer. Persistence goes through the shell's
// shared savePrefs.
export default function CaptureSection({
  intervalSec,
  setIntervalSec,
  prefs,
  saving,
  savePrefs,
}: Props) {
  function saveInterval() {
    const sec = Math.max(1, Math.round(Number(intervalSec) || 3));
    return savePrefs({ captureIntervalMs: sec * 1000 }, "Capture interval saved.");
  }

  return (
    <>
      <header className="settings-head">
        <h1>Capture Prefs</h1>
        <p>How often the interviewer sees a fresh view of your screen.</p>
      </header>
      <div className="settings-card">
        <h3 className="settings-card-title">Capture interval</h3>
        <p className="settings-hint">
          How often the app sends a fresh screenshot to the interviewer (seconds).
        </p>
        <div className="settings-field-row">
          <input
            type="number"
            min={1}
            className="settings-input settings-input-grow"
            value={intervalSec}
            onChange={(e) => setIntervalSec(e.target.value)}
            disabled={saving || !prefs}
            onKeyDown={(e) => e.key === "Enter" && saveInterval()}
          />
          <button
            className="btn btn-primary settings-field-save"
            onClick={saveInterval}
            disabled={saving || !prefs}
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
        <p className="settings-hint settings-hint-muted">
          Choose the display and crop region from the Hub before starting a session.
        </p>
      </div>
    </>
  );
}
