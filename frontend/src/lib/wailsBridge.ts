// Single import point for all Wails-bound Go methods and model types.
// Components should import from here, never from wailsjs/ directly.

export {
  EndSession,
  GetAuthStatus,
  GetLatestScreenshot,
  GetPreferences,
  GetProblem,
  GetSessionTranscript,
  ListProblems,
  ListSessions,
  SendMessage,
  SetAPIKey,
  StartCapture,
  StartSession,
  StopCapture,
  UpdatePreferences,
} from "../../wailsjs/go/main/App";

export { models } from "../../wailsjs/go/models";
