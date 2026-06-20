import { useEffect, useState } from "react";
import {
  SetAPIKey,
  GetAuthStatus,
  GetPreferences,
  UpdatePreferences,
  models,
} from "../lib/wailsBridge";
import "./Settings.css";

interface Props {
  authStatus: models.AuthStatus;
  onUpdate: (status: models.AuthStatus) => void;
  onClose: () => void;
}

export default function Settings({ authStatus, onUpdate, onClose }: Props) {
  const [openRouterKey, setOpenRouterKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const [prefs, setPrefs] = useState<models.Preferences | null>(null);
  const [intervalSec, setIntervalSec] = useState("3");
  const [limitMinutes, setLimitMinutes] = useState("30");
  const [warningMinutes, setWarningMinutes] = useState("25");

  // Load preferences on mount.
  useEffect(() => {
    GetPreferences()
      .then((p) => {
        setPrefs(p);
        setIntervalSec(String(Math.max(1, Math.round(p.captureIntervalMs / 1000))));
        setLimitMinutes(String(p.sessionLimitMinutes ?? 30));
        setWarningMinutes(String(p.softWarningMinutes ?? 25));
      })
      .catch(() => {});
  }, []);

  async function saveInterval() {
    if (!prefs) return;
    const sec = Math.max(1, Math.round(Number(intervalSec) || 3));
    setSaving(true);
    setError("");
    setSuccess("");
    try {
      const updated = new models.Preferences({ ...prefs, captureIntervalMs: sec * 1000 });
      await UpdatePreferences(updated);
      setPrefs(updated);
      setSuccess("Capture interval saved.");
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setSaving(false);
    }
  }

  async function saveTimerSettings() {
    if (!prefs) return;
    const limit = Math.max(0, Math.round(Number(limitMinutes) || 0));
    const warning = Math.max(0, Math.round(Number(warningMinutes) || 0));
    if (limit > 0 && warning >= limit) {
      setError("Warning time must be less than the session limit.");
      return;
    }
    setSaving(true);
    setError("");
    setSuccess("");
    try {
      const updated = new models.Preferences({
        ...prefs,
        sessionLimitMinutes: limit,
        softWarningMinutes: warning,
      });
      await UpdatePreferences(updated);
      setPrefs(updated);
      setSuccess("Session timer saved.");
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setSaving(false);
    }
  }

  async function saveKey() {
    const key = openRouterKey.trim();
    if (!key) return;

    setSaving(true);
    setError("");
    setSuccess("");

    try {
      await SetAPIKey("openrouter", key);
      const status = await GetAuthStatus();
      onUpdate(status);
      setOpenRouterKey("");
      setSuccess("OpenRouter API key saved.");
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="settings-overlay" onClick={onClose}>
      <div className="settings-panel" onClick={(e) => e.stopPropagation()}>
        <div className="settings-header">
          <h2>Settings</h2>
          <button className="settings-close" onClick={onClose}>
            &times;
          </button>
        </div>

        <section className="settings-section">
          <h3>OpenRouter API Key</h3>
          <p className="settings-hint">
            Get a key at{" "}
            <a
              href="https://openrouter.ai/keys"
              target="_blank"
              rel="noopener noreferrer"
            >
              openrouter.ai/keys
            </a>
          </p>
          <div className="settings-status">
            Status:{" "}
            {authStatus.openRouterConfigured ? (
              <span className="status-ok">Configured</span>
            ) : (
              <span className="status-missing">Not configured</span>
            )}
          </div>
          <div className="settings-key-row">
            <input
              type="password"
              className="settings-input"
              value={openRouterKey}
              onChange={(e) => setOpenRouterKey(e.target.value)}
              placeholder="sk-or-..."
              disabled={saving}
              onKeyDown={(e) => e.key === "Enter" && saveKey()}
            />
            <button
              className="settings-save"
              onClick={saveKey}
              disabled={!openRouterKey.trim() || saving}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </section>

        <section className="settings-section">
          <h3>Capture interval</h3>
          <p className="settings-hint">
            How often the app sends a fresh screenshot to the interviewer
            (seconds).
          </p>
          <div className="settings-key-row">
            <input
              type="number"
              min={1}
              className="settings-input"
              value={intervalSec}
              onChange={(e) => setIntervalSec(e.target.value)}
              disabled={saving || !prefs}
              onKeyDown={(e) => e.key === "Enter" && saveInterval()}
            />
            <button
              className="settings-save"
              onClick={saveInterval}
              disabled={saving || !prefs}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </section>

        <section className="settings-section">
          <h3>Session time limit</h3>
          <p className="settings-hint">
            Set limit to 0 for untimed practice. Warning fires N minutes before
            the limit and 0 disables it.
          </p>
          <div className="settings-limit-row">
            <div className="settings-limit-field">
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
            <div className="settings-limit-field">
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
              className="settings-save settings-save-bottom"
              onClick={saveTimerSettings}
              disabled={saving || !prefs}
            >
              {saving ? "Saving..." : "Save"}
            </button>
          </div>
        </section>

        {error && <p className="settings-error">{error}</p>}
        {success && <p className="settings-success">{success}</p>}
      </div>
    </div>
  );
}
