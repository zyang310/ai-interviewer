// Single import point for all Wails-bound Go methods and model types.
// Components should import from here, never from wailsjs/ directly.

export {
  DeleteAPIKey,
  EndSession,
  EnterOverlayMode,
  ExitOverlayMode,
  GetAuthStatus,
  GetHotkeyStatus,
  GetLatestScreenshot,
  GetPreferences,
  GetSessionTranscript,
  ListAvailableModels,
  ListDisplays,
  ListSessions,
  ListVoices,
  MinimiseWindow,
  OpenInputMonitoringSettings,
  PreviewVoice,
  QuitApp,
  SendMessage,
  SetAPIKey,
  SetCaptureRegion,
  SetOverlayExpanded,
  SnapshotDisplay,
  StartCapture,
  StartSession,
  StopCapture,
  SynthesizeSpeech,
  ToggleMaximiseWindow,
  TranscribeAudio,
  UpdatePreferences,
} from "../../wailsjs/go/main/App";

export { models, capture, hotkey } from "../../wailsjs/go/models";

// Wails runtime event bus — used for backend-pushed events (e.g. the global
// voice-hotkey "ptt:down"). Re-exported here so components keep a single import
// point and never reach into wailsjs/ directly.
export { EventsOn } from "../../wailsjs/runtime";
