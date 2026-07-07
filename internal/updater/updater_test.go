package updater

import "testing"

// TestIsNewer covers the version-comparison logic that decides whether to
// prompt for an update: only a valid semver strictly greater than the running
// build counts, and dev/malformed versions must never trigger a prompt.
func TestIsNewer(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		want            bool
	}{
		{"newer patch", "v1.0.0", "v1.0.1", true},
		{"newer minor", "v1.0.0", "v1.1.0", true},
		{"newer major", "v1.9.9", "v2.0.0", true},
		{"equal", "v1.2.3", "v1.2.3", false},
		{"older", "v2.0.0", "v1.0.0", false},
		{"missing v prefix still compares", "1.0.0", "1.0.1", true},
		{"dev current never updates", "dev", "v1.0.0", false},
		{"ci dev current never updates", "dev-abc1234", "v1.0.0", false},
		{"malformed latest ignored", "v1.0.0", "not-a-version", false},
		{"empty latest ignored", "v1.0.0", "", false},
		{"empty current ignored", "", "v1.0.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNewer(tc.current, tc.latest); got != tc.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

// TestPickZipAsset confirms we select the packaged .app .zip (not other release
// files) and return empty when no zip is attached.
func TestPickZipAsset(t *testing.T) {
	rel := ghRelease{Assets: []ghAsset{
		{Name: "checksums.txt", BrowserDownloadURL: "https://example/checksums.txt"},
		{Name: "Mogi-v1.0.0-macos-universal.zip", BrowserDownloadURL: "https://example/app.zip"},
	}}
	if got := pickZipAsset(rel); got != "https://example/app.zip" {
		t.Errorf("pickZipAsset = %q, want the .zip url", got)
	}
	if got := pickZipAsset(ghRelease{}); got != "" {
		t.Errorf("pickZipAsset(no assets) = %q, want empty", got)
	}
}
