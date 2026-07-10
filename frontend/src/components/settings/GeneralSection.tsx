import { models } from "../../lib/wailsBridge";
import type { ThemePref } from "../../lib/theme";
import "./GeneralSection.css";

// Per-theme palettes for the Appearance preview tiles. Hardcoded on purpose
// (not MD3 tokens): each tile must render its own theme's colors regardless of
// the app's current theme, so the light preview stays light even in dark mode.
// Values mirror the light/dark token blocks in style.css.
interface PreviewPalette {
  bg: string;
  surface: string;
  border: string;
  ink: string;
  muted: string;
  accent: string;
  onAccent: string;
  secondary: string;
  secondarySoft: string;
  accentSoft: string;
}

const LIGHT_PREVIEW: PreviewPalette = {
  bg: "#F1E7D7",
  surface: "#FCF9F2",
  border: "#E4D7C4",
  ink: "#23264A",
  muted: "#8a8172",
  accent: "#2C2F5A",
  onAccent: "#EDE3D6",
  secondary: "#5F7639",
  secondarySoft: "rgba(95,118,57,.14)",
  accentSoft: "rgba(44,47,90,.12)",
};

const DARK_PREVIEW: PreviewPalette = {
  bg: "#141631",
  surface: "#1E2142",
  border: "#333764",
  ink: "#EDE3D6",
  muted: "#8f8aa0",
  accent: "#8E93E0",
  onAccent: "#14162A",
  secondary: "#A0BA76",
  secondarySoft: "rgba(160,186,118,.16)",
  accentSoft: "rgba(142,147,224,.18)",
};

// Appearance theme choices, each with the preview palette(s) it renders. "System"
// shows both halves (light | dark); the concrete themes show one.
const THEME_TILES: {
  id: ThemePref;
  name: string;
  sub: string;
  halves: PreviewPalette[];
}[] = [
  { id: "system", name: "System", sub: "Matches your OS", halves: [LIGHT_PREVIEW, DARK_PREVIEW] },
  { id: "light", name: "Light", sub: "Warm washi", halves: [LIGHT_PREVIEW] },
  { id: "dark", name: "Dark", sub: "Indigo night", halves: [DARK_PREVIEW] },
];

interface Props {
  // Theme lives in App (mirrored to <html> + localStorage), so the tiles
  // read/write through props to stay in sync with the pill-nav quick toggle.
  themePref: ThemePref;
  onThemeChange: (pref: ThemePref) => void;
  // Timer inputs live in the Settings shell so "Clear all data" can reset them
  // centrally and unsaved edits survive switching sections.
  limitMinutes: string;
  setLimitMinutes: (v: string) => void;
  warningMinutes: string;
  setWarningMinutes: (v: string) => void;
  prefs: models.Preferences | null;
  saving: boolean;
  savePrefs: (patch: Partial<models.Preferences>, msg: string) => Promise<void>;
  // Timer validation errors surface through the shell's shared error line.
  setError: (msg: string) => void;
}

// GeneralSection is the Settings → General pane: the Appearance theme tiles and
// the session time limit card with its live timeline preview.
export default function GeneralSection({
  themePref,
  onThemeChange,
  limitMinutes,
  setLimitMinutes,
  warningMinutes,
  setWarningMinutes,
  prefs,
  saving,
  savePrefs,
  setError,
}: Props) {
  function saveTimerSettings() {
    const limit = Math.max(0, Math.round(Number(limitMinutes) || 0));
    const warning = Math.max(0, Math.round(Number(warningMinutes) || 0));
    if (limit > 0 && warning >= limit) {
      setError("Warning time must be less than the session limit.");
      return;
    }
    return savePrefs(
      { sessionLimitMinutes: limit, softWarningMinutes: warning },
      "Session timer saved."
    );
  }

  // Session-timeline geometry: the accent fill runs 0 → warning with a gold
  // marker at the warning point; the whole bar spans the limit. A limit of 0 is
  // untimed practice — no fill, no markers.
  const limitNum = Math.max(0, Number(limitMinutes) || 0);
  const warnNum = Math.max(0, Number(warningMinutes) || 0);
  const hasWarn = limitNum > 0 && warnNum > 0 && warnNum < limitNum;
  const warnPct = hasWarn ? (warnNum / limitNum) * 100 : 0;

  return (
    <>
      <header className="settings-head">
        <h1>General</h1>
        <p>Session behavior and appearance for your mock interviews.</p>
      </header>
      <div className="settings-card">
        <div className="settings-card-head">
          <span className="material-symbols-outlined">contrast</span>
          <h3 className="settings-card-title">Appearance</h3>
        </div>
        <p className="settings-hint">
          Color theme for the app. “System” follows your OS light/dark setting.
        </p>
        <div className="appearance-grid">
          {THEME_TILES.map((t) => {
            const on = themePref === t.id;
            return (
              <button
                key={t.id}
                type="button"
                className={`theme-tile${on ? " is-selected" : ""}`}
                onClick={() => onThemeChange(t.id)}
              >
                <div className={`theme-prev${t.halves.length > 1 ? " is-split" : ""}`}>
                  {t.halves.map((h, i) => (
                    <div
                      key={i}
                      className="theme-prev-half"
                      style={{ background: h.bg }}
                    >
                      <div
                        className="theme-prev-card"
                        style={{ background: h.surface, borderColor: h.border }}
                      >
                        <div className="theme-prev-status">
                          <span
                            className="theme-prev-dot"
                            style={{ background: h.secondary }}
                          />
                          <span style={{ color: h.muted }}>Ready</span>
                        </div>
                        <div className="theme-prev-title" style={{ color: h.ink }}>
                          Ready to begin?
                        </div>
                        <div
                          className="theme-prev-btn"
                          style={{ background: h.accent, color: h.onAccent }}
                        >
                          Start session
                        </div>
                        <div className="theme-prev-pills">
                          <span
                            style={{ background: h.secondarySoft, color: h.secondary }}
                          >
                            Easy
                          </span>
                          <span style={{ background: h.accentSoft, color: h.accent }}>
                            Focus
                          </span>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                <div className="theme-tile-foot">
                  <div>
                    <div className="theme-tile-name">{t.name}</div>
                    <div className="theme-tile-sub">{t.sub}</div>
                  </div>
                  {on && (
                    <span className="material-symbols-outlined theme-tile-check">
                      check_circle
                    </span>
                  )}
                </div>
              </button>
            );
          })}
        </div>
      </div>
      <div className="settings-card">
        <div className="settings-card-head">
          <span className="material-symbols-outlined">schedule</span>
          <h3 className="settings-card-title">Session time limit</h3>
        </div>
        <p className="settings-hint">
          Set the limit to 0 for untimed practice. The warning fires N minutes
          before the limit; 0 disables it.
        </p>

        {/* Live timeline: reflects the current Limit/Warning inputs. */}
        <div className="timeline">
          <div className="timeline-track">
            {limitNum > 0 ? (
              <>
                <div
                  className="timeline-fill"
                  style={{ width: `${hasWarn ? warnPct : 100}%` }}
                />
                {hasWarn && (
                  <div className="timeline-marker" style={{ left: `${warnPct}%` }} />
                )}
              </>
            ) : (
              <div className="timeline-fill timeline-fill--untimed" />
            )}
          </div>
          <div className="timeline-labels">
            {limitNum > 0 ? (
              <>
                <span className="timeline-label">
                  <strong>0m</strong> start
                </span>
                {hasWarn && (
                  <span className="timeline-label timeline-label--warn">
                    <span className="timeline-warn-dot" />
                    Warning at <strong>{warnNum}m</strong>
                  </span>
                )}
                <span className="timeline-label">
                  <strong className="timeline-limit">{limitNum}m</strong> limit
                </span>
              </>
            ) : (
              <span className="timeline-label timeline-label--muted">
                Untimed practice — no limit or warning
              </span>
            )}
          </div>
        </div>

        <div className="settings-field-row">
          <div className="settings-field">
            <label className="settings-label">Limit (min)</label>
            <input
              type="number"
              min={0}
              className="settings-input"
              value={limitMinutes}
              onChange={(e) => setLimitMinutes(e.target.value)}
              disabled={saving || !prefs}
            />
          </div>
          <div className="settings-field">
            <label className="settings-label">Warning (min)</label>
            <input
              type="number"
              min={0}
              className="settings-input"
              value={warningMinutes}
              onChange={(e) => setWarningMinutes(e.target.value)}
              disabled={saving || !prefs}
            />
          </div>
          <button
            className="btn btn-primary settings-field-save"
            onClick={saveTimerSettings}
            disabled={saving || !prefs}
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </>
  );
}
