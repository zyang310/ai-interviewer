// Package updater checks GitHub Releases for a newer version of the app,
// reports whether one is available (Check), and can install it in place
// (Install, macOS-only — see install.go and install_other.go). All network
// access is here, mirroring the "external calls live in the Go backend"
// pattern in internal/ai/client.go. See docs/ci-cd-and-auto-update.md.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mogi/internal/models"

	"golang.org/x/mod/semver"
)

const (
	// latestReleaseURL is GitHub's "latest published, non-prerelease release"
	// endpoint for this repo. It returns 404 until the first release is cut.
	latestReleaseURL = "https://api.github.com/repos/zyang310/mogi/releases/latest"
	// httpTimeout is short: the check runs on launch and must never block the UI.
	httpTimeout = 15 * time.Second
)

// ghAsset is a single downloadable file attached to a GitHub release.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ghRelease is the subset of GitHub's release JSON we read.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	HTMLURL string    `json:"html_url"`
	Body    string    `json:"body"`
	Assets  []ghAsset `json:"assets"`
}

// Check asks GitHub for the latest release and compares it to currentVersion.
// It returns whether a newer version is available plus the URLs/notes needed to
// download it. Dev/local builds (a non-semver version like "dev") short-circuit
// to "no update" without a network call, so they never nag and never spend the
// unauthenticated GitHub rate limit. A repo with no releases yet (404) is also
// "no update", not an error.
func Check(ctx context.Context, currentVersion string) (models.UpdateInfo, error) {
	info := models.UpdateInfo{CurrentVersion: currentVersion}

	// Dev builds carry a non-semver version; nothing to compare against.
	if !semver.IsValid(normalize(currentVersion)) {
		return info, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return info, fmt.Errorf("updater: build request: %w", err)
	}
	// GitHub requires a User-Agent; the other headers pin the API version.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mogi")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return info, fmt.Errorf("updater: http request: %w", err)
	}
	defer resp.Body.Close()

	// No releases published yet — there is simply nothing to update to.
	if resp.StatusCode == http.StatusNotFound {
		return info, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return info, fmt.Errorf("updater: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("updater: GitHub returned %d: %s", resp.StatusCode, string(body))
	}

	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return info, fmt.Errorf("updater: decode response: %w", err)
	}

	return infoFromRelease(currentVersion, rel), nil
}

// infoFromRelease maps a GitHub release onto the UpdateInfo the frontend acts
// on. Available requires BOTH a newer version and a packaged .zip to install:
// release-please publishes the GitHub Release (tag + changelog) the moment the
// Release PR merges, but the signed build is attached by release.yml only after
// notarization — up to a couple of hours later. In that window (or if a build
// fails and never attaches), prompting would offer an "update" that can only
// open the release page, where there is nothing to download either. LatestVersion
// is still reported, so the About pane can say the build is on its way.
func infoFromRelease(currentVersion string, rel ghRelease) models.UpdateInfo {
	info := models.UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  rel.TagName,
		ReleaseURL:     rel.HTMLURL,
		Notes:          rel.Body,
		DownloadURL:    pickZipAsset(rel),
	}
	info.Available = isNewer(currentVersion, rel.TagName) && info.DownloadURL != ""
	return info
}

// isNewer reports whether latest is a valid semantic version strictly greater
// than current. Invalid or empty versions (a "dev" build, or no release yet)
// yield false, so dev builds and unreleased repos never prompt to update.
func isNewer(current, latest string) bool {
	cv, lv := normalize(current), normalize(latest)
	if !semver.IsValid(cv) || !semver.IsValid(lv) {
		return false
	}
	return semver.Compare(lv, cv) > 0
}

// normalize ensures a leading "v" so values work with golang.org/x/mod/semver,
// which requires the canonical "vMAJOR.MINOR.PATCH" form. An empty string is
// left as-is (and is reported invalid by semver.IsValid).
func normalize(v string) string {
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// pickZipAsset returns the download URL of the release's .zip asset (the
// packaged macOS .app), or "" if the release has none. The frontend falls back
// to the release page when this is empty, so the user can always reach the file.
func pickZipAsset(rel ghRelease) string {
	for _, a := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
			return a.BrowserDownloadURL
		}
	}
	return ""
}
