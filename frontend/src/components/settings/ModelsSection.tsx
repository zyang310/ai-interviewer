import { models } from "../../lib/wailsBridge";
import ModelPicker from "./ModelPicker";

interface Props {
  prefs: models.Preferences | null;
  savePrefs: (patch: Partial<models.Preferences>, msg: string) => Promise<void>;
}

// ModelsSection is the Settings → Models pane: a thin wrapper that hosts the
// ModelPicker in a card and persists the selection through the shell's
// shared savePrefs.
export default function ModelsSection({ prefs, savePrefs }: Props) {
  function saveModel(modelId: string) {
    return savePrefs({ model: modelId }, "Model saved.");
  }

  return (
    <>
      <header className="settings-head">
        <h1>Model Architecture</h1>
      </header>
      {/* Flush layout: the picker floats in its own card like every
          other section's content. */}
      <div className="settings-card">
        <ModelPicker currentModelId={prefs?.model ?? ""} onSelect={saveModel} />
      </div>
    </>
  );
}
