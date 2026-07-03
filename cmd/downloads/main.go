// Command downloads reports how many times this app's release assets have been
// downloaded from GitHub. GitHub tracks a download_count per release asset for
// free, so adoption ("how many people downloaded the app") needs no backend and
// no in-app telemetry — this tool just reads the public Releases API and adds
// up the counts.
//
// It is a manual, repo-owner utility (run `go run ./cmd/downloads`); the app
// binary never invokes it. Network access lives here, mirroring the
// "external calls in Go" pattern in internal/updater.
//
// GitHub rate-limits unauthenticated requests to 60/hour. Set GITHUB_TOKEN to
// raise that if you hit it (the token needs no scopes for a public repo).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	// releasesURL lists this repo's releases, newest first, 100 per page.
	releasesURL = "https://api.github.com/repos/zyang310/ai-interviewer/releases?per_page=100"
	httpTimeout = 30 * time.Second
)

// ghAsset is one downloadable file attached to a release, plus its running
// download total as tracked by GitHub.
type ghAsset struct {
	Name          string `json:"name"`
	DownloadCount int    `json:"download_count"`
}

// ghRelease is the subset of GitHub's release JSON we read.
type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []ghAsset `json:"assets"`
}

func main() {
	jsonOut := flag.Bool("json", false, "emit raw per-asset counts as JSON instead of a table")
	flag.Parse()

	releases, err := fetchReleases()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if *jsonOut {
		if err := json.NewEncoder(os.Stdout).Encode(releases); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	printReport(releases)
}

// fetchReleases pages through the Releases API and returns every release,
// newest first. It sends the same headers internal/updater uses (GitHub
// requires a User-Agent) and forwards GITHUB_TOKEN when present to raise the
// rate limit. A repo with no releases yet returns an empty slice, not an error.
func fetchReleases() ([]ghRelease, error) {
	client := &http.Client{Timeout: httpTimeout}
	var all []ghRelease
	url := releasesURL

	for url != "" {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "ai-interviewer")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("http request: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil // no releases published yet
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var page []ghRelease
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		all = append(all, page...)

		// Follow the Link header's rel="next" for pagination, if present.
		url = nextPageURL(resp.Header.Get("Link"))
	}

	return all, nil
}

// nextPageURL extracts the rel="next" URL from a GitHub Link header, or "" when
// there are no more pages.
func nextPageURL(link string) string {
	for _, part := range strings.Split(link, ",") {
		segs := strings.Split(strings.TrimSpace(part), ";")
		if len(segs) < 2 {
			continue
		}
		if strings.Contains(segs[1], `rel="next"`) {
			return strings.Trim(strings.TrimSpace(segs[0]), "<>")
		}
	}
	return ""
}

// printReport writes a human-readable per-release breakdown and a grand total.
// Releases are shown newest first; each asset's individual count is listed so a
// ".zip" (the packaged app) can be told apart from other attachments.
func printReport(releases []ghRelease) {
	if len(releases) == 0 {
		fmt.Println("No releases published yet — nothing to count.")
		return
	}

	// Sort newest first regardless of API order.
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].PublishedAt.After(releases[j].PublishedAt)
	})

	grand := 0
	for _, rel := range releases {
		label := rel.TagName
		if label == "" {
			label = rel.Name
		}
		tags := []string{}
		if rel.Draft {
			tags = append(tags, "draft")
		}
		if rel.Prerelease {
			tags = append(tags, "prerelease")
		}
		suffix := ""
		if len(tags) > 0 {
			suffix = " (" + strings.Join(tags, ", ") + ")"
		}

		relTotal := 0
		for _, a := range rel.Assets {
			relTotal += a.DownloadCount
		}
		grand += relTotal

		fmt.Printf("%-16s %6d downloads%s\n", label, relTotal, suffix)
		for _, a := range rel.Assets {
			fmt.Printf("    %-32s %6d\n", a.Name, a.DownloadCount)
		}
		if len(rel.Assets) == 0 {
			fmt.Println("    (no assets)")
		}
	}

	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("%-16s %6d downloads (all releases)\n", "TOTAL", grand)
}
