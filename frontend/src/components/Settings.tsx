import { useState } from "react";
import { SetAPIKey, GetAuthStatus, models } from "../lib/wailsBridge";
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

        {error && <p className="settings-error">{error}</p>}
        {success && <p className="settings-success">{success}</p>}
      </div>
    </div>
  );
}
