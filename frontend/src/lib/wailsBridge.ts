// Single import point for all Wails-bound Go methods and model types.
// Components should import from here, never from wailsjs/ directly.

export {
  EndSession,
  GetAuthStatus,
  GetLatestScreenshot,
  GetPreferences,
  GetSessionTranscript,
  ListDisplays,
  ListSessions,
  SendMessage,
  SetAPIKey,
  SetCaptureRegion,
  SnapshotDisplay,
  StartCapture,
  StartSession,
  StopCapture,
  UpdatePreferences,
} from "../../wailsjs/go/main/App";

export { models, capture } from "../../wailsjs/go/models";
