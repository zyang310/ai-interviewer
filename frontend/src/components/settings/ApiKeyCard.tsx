import type { ReactNode } from "react";
import "./ApiKeyCard.css";

export type KeyProvider = "openrouter" | "elevenlabs" | "google";

// Per-provider metadata for an API-key card (label, icon tile, input hint,
// placeholder). Defined here with the card; ApiKeysSection owns the list.
export interface KeyCard {
  id: KeyProvider;
  icon: string; // Material Symbols name for the icon tile
  placeholder: string;
  hint: ReactNode;
}

interface Props {
  card: KeyCard;
  label: string; // provider display name, also used in the status pill row
  isSet: boolean; // key configured on the backend
  // Resting mode follows configured status ("view"/"bare"); ApiKeysSection
  // overrides it while the user is replacing or confirming a remove.
  mode: "view" | "edit" | "bare" | "confirmRemove";
  draft: string; // key being typed (edit/bare only — stored keys never reach the frontend)
  onDraftChange: (v: string) => void;
  revealed: boolean;
  onToggleReveal: () => void;
  menuOpen: boolean;
  onToggleMenu: () => void;
  onCloseMenu: () => void;
  onSetMode: (mode: "edit" | "confirmRemove" | null) => void;
  onSave: () => void;
  onRemove: () => void;
  saving: boolean;
}

// ApiKeyCard is one provider row in Settings → API Keys: an icon tile +
// name/hint head with a status pill, then a body that switches between view
// (masked key + overflow menu), edit/bare (input + Save), and confirm-remove.
// Purely presentational — state and persistence live in ApiKeysSection.
export default function ApiKeyCard({
  card,
  label,
  isSet,
  mode,
  draft,
  onDraftChange,
  revealed,
  onToggleReveal,
  menuOpen,
  onToggleMenu,
  onCloseMenu,
  onSetMode,
  onSave,
  onRemove,
  saving,
}: Props) {
  return (
    <div className="settings-card apikey-card">
      <div className="apikey-head">
        <div className={`apikey-icon${isSet ? "" : " muted"}`}>
          <span className="material-symbols-outlined">{card.icon}</span>
        </div>
        <div className="apikey-meta">
          <div className="apikey-name">{label}</div>
          <div className="apikey-sub">{card.hint}</div>
        </div>
        <div className={`apikey-status${isSet ? " is-configured" : ""}`}>
          <span className="apikey-status-dot" />
          {isSet ? "Configured" : "Not set"}
        </div>
      </div>

      {/* VIEW — a stored key, shown as a masked (non-revealable, the
          frontend never holds it) field with an overflow menu. */}
      {mode === "view" && (
        <div className="apikey-row">
          <div className="apikey-input-wrap apikey-input-wrap-static">
            <span className="material-symbols-outlined apikey-input-icon">key</span>
            <span className="apikey-masked">••••••••••••••••</span>
          </div>
          <div className="apikey-menu-wrap">
            <button
              type="button"
              className="apikey-menu-btn"
              title="More actions"
              aria-label="More actions"
              onClick={onToggleMenu}
              disabled={saving}
            >
              <span className="material-symbols-outlined">more_vert</span>
            </button>
            {menuOpen && (
              <>
                <div className="apikey-menu-overlay" onClick={onCloseMenu} />
                <div className="apikey-menu" role="menu">
                  <button
                    type="button"
                    className="apikey-menu-item"
                    onClick={() => onSetMode("edit")}
                  >
                    <span className="material-symbols-outlined">sync</span>
                    Replace key
                  </button>
                  <button
                    type="button"
                    className="apikey-menu-item apikey-menu-item-danger"
                    onClick={() => onSetMode("confirmRemove")}
                  >
                    <span className="material-symbols-outlined">delete</span>
                    Remove key
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      {/* EDIT (replacing) / BARE (first time) — editable input + Save. */}
      {(mode === "edit" || mode === "bare") && (
        <div className="apikey-row">
          <div
            className={`apikey-input-wrap${mode === "edit" ? " is-editing" : ""}`}
          >
            <span className="material-symbols-outlined apikey-input-icon">key</span>
            <input
              type={revealed ? "text" : "password"}
              className="apikey-input"
              value={draft}
              onChange={(e) => onDraftChange(e.target.value)}
              placeholder={card.placeholder}
              disabled={saving}
              autoFocus={mode === "edit"}
              onKeyDown={(e) => e.key === "Enter" && onSave()}
            />
            <button
              type="button"
              className="apikey-input-eye"
              onClick={onToggleReveal}
              title={revealed ? "Hide key" : "Show key"}
              aria-label={revealed ? "Hide key" : "Show key"}
              tabIndex={-1}
            >
              <span className="material-symbols-outlined">
                {revealed ? "visibility_off" : "visibility"}
              </span>
            </button>
          </div>
          <button
            className="btn btn-primary settings-field-save"
            onClick={onSave}
            disabled={!draft.trim() || saving}
          >
            {saving ? "Saving…" : "Save"}
          </button>
          {mode === "edit" && (
            <button
              type="button"
              className="apikey-icon-btn"
              title="Cancel"
              aria-label="Cancel"
              onClick={() => onSetMode(null)}
              disabled={saving}
            >
              <span className="material-symbols-outlined">close</span>
            </button>
          )}
        </div>
      )}

      {/* CONFIRM REMOVE — a beat before the destructive action. */}
      {mode === "confirmRemove" && (
        <div className="apikey-confirm">
          <span className="apikey-confirm-text">
            Remove this key? You'll need to paste it again later.
          </span>
          <div className="apikey-confirm-actions">
            <button
              type="button"
              className="apikey-confirm-keep"
              onClick={() => onSetMode(null)}
              disabled={saving}
            >
              Keep it
            </button>
            <button
              type="button"
              className="apikey-confirm-remove"
              onClick={onRemove}
              disabled={saving}
            >
              {saving ? "Removing…" : "Remove"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
