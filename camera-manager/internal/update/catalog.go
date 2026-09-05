package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ReleaseInfo struct {
	Tag  string `json:"tag"`
	Name string `json:"name"`
	// Notes is raw markdown; the UI renders it as text, never as HTML.
	Notes string `json:"notes"`
	// URL is the release archive asset, HTMLURL the human release page.
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

// releaseNotesHaveDetails filters the body GitHub generates when a release
// has no PRs: it contains only a "Full Changelog" compare link. That link is
// useful in the browser, but not enough for the local update sheet.
func releaseNotesHaveDetails(notes string) bool {
	for _, rawLine := range strings.Split(notes, "\n") {
		line := strings.TrimSpace(rawLine)
		line = strings.TrimLeft(line, "#*-+ \t")
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if line == "" || lower == "what's changed" || strings.HasPrefix(lower, "full changelog") {
			continue
		}
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			continue
		}
		return true
	}
	return false
}

type compareCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

type compareResponse struct {
	Commits []compareCommit `json:"commits"`
}

// compareCommitNotes turns the commits between the installed and newest
// release into the same plain-text markdown format as release notes.
func (f Catalog) compareCommitNotes(ctx context.Context, current, latest string) (string, error) {
	base := strings.TrimSpace(current)
	if !isComparableVersion(base) {
		return "", errors.New("installed version cannot be compared")
	}
	if !strings.HasPrefix(strings.ToLower(base), "v") {
		base = "v" + base
	}
	endpoint := strings.TrimRight(f.BaseURL, "/") + "/compare/" + url.PathEscape(base) + "..." + url.PathEscape(strings.TrimSpace(latest))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := f.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github compare api: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", err
	}
	var comparison compareResponse
	if err := json.Unmarshal(body, &comparison); err != nil {
		return "", err
	}

	lines := make([]string, 0, len(comparison.Commits))
	for _, commit := range comparison.Commits {
		message := strings.TrimSpace(strings.SplitN(commit.Commit.Message, "\n", 2)[0])
		if message == "" {
			continue
		}
		sha := strings.TrimSpace(commit.SHA)
		if len(sha) > 7 {
			sha = sha[:7]
		}
		if sha != "" {
			message += " (" + sha + ")"
		}
		lines = append(lines, "* "+message)
	}
	return strings.Join(lines, "\n"), nil
}

func isComparableVersion(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func collectNewerReleases(releases []Release, current string) (*ReleaseInfo, []ReleaseInfo) {
	var changes []ReleaseInfo
	var latest *ReleaseInfo
	for _, rel := range releases {
		if strings.TrimSpace(rel.Tag) == "" {
			continue
		}
		if CompareVersions(rel.Tag, current) <= 0 {
			continue
		}
		info := ReleaseInfo{
			Tag:         rel.Tag,
			Name:        rel.Name,
			Notes:       rel.Notes,
			URL:         rel.ArchiveURL(),
			HTMLURL:     rel.HTMLURL,
			PublishedAt: rel.PublishedAt,
		}
		if latest == nil {
			cp := info
			latest = &cp
		}
		changes = append(changes, info)
	}
	return latest, changes
}

type Catalog struct {
	BaseURL string
	Client  *http.Client
}
type CatalogResult struct {
	Latest  *ReleaseInfo
	Changes []ReleaseInfo
}

// Check loads releases newer than the installed version and expands empty notes.
func (f Catalog) Check(ctx context.Context, current string) (CatalogResult, error) {
	if f.BaseURL == "" {
		f.BaseURL = DefaultRepoAPI
	}
	if f.Client == nil {
		f.Client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.BaseURL+"/releases?per_page=15", nil)
	if err != nil {
		return CatalogResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := f.Client.Do(req)
	if err != nil {
		return CatalogResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("github api: %s", resp.Status)
		return CatalogResult{}, err
	}
	var releases []Release
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err == nil {
		err = json.Unmarshal(body, &releases)
	}
	if err != nil {
		return CatalogResult{}, err
	}

	latestInfo, changes := collectNewerReleases(releases, current)
	if latestInfo != nil && !releaseNotesHaveDetails(latestInfo.Notes) {
		if notes, err := f.compareCommitNotes(ctx, current, latestInfo.Tag); err == nil && notes != "" {
			latestCopy := *latestInfo
			latestCopy.Notes = notes
			latestInfo = &latestCopy
			if len(changes) > 0 && changes[0].Tag == latestCopy.Tag {
				changes[0].Notes = notes
			}
		}
	}
	return CatalogResult{Latest: latestInfo, Changes: changes}, nil
}
