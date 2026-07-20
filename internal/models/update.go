package models

// UpdateInfo is the result of an app-update check against GitHub Releases,
// surfaced to the frontend so it can prompt the user to download a newer
// version. The app does not self-install — DownloadURL points at the release's
// packaged .zip for the user to install manually.
type UpdateInfo struct {
	Available      bool   `json:"available"`      // a newer release than the running build exists
	CurrentVersion string `json:"currentVersion"` // the running app's version (e.g. "v0.1.0" or "dev")
	LatestVersion  string `json:"latestVersion"`  // the latest release tag on GitHub (empty if none)
	ReleaseURL     string `json:"releaseUrl"`     // GitHub release page (human-readable)
	DownloadURL    string `json:"downloadUrl"`    // direct link to the .zip asset (empty if none)
	Notes          string `json:"notes"`          // release notes / changelog body
}
