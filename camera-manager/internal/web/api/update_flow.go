package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	updater "camera-appliance/camera-manager/internal/update"
	"camera-appliance/camera-manager/internal/version"
)

// Update flow phases, mirroring the t3-chat style UX:
// idle → (check) → up_to_date | available → (download) → ready → (install) → restarting.
const (
	updateIdle        = "idle"
	updateChecking    = "checking"
	updateUpToDate    = "up_to_date"
	updateAvailable   = "available"
	updateDownloading = "downloading"
	updateReady       = "ready"
	updateInstalling  = "installing"
	updateFailed      = "failed"
)

type updateReleaseInfo struct {
	Tag  string `json:"tag"`
	Name string `json:"name"`
	// Notes is raw markdown; the UI renders it as text, never as HTML.
	Notes string `json:"notes"`
	// URL is the release archive asset, HTMLURL the human release page.
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type updateFlowStatus struct {
	Phase          string              `json:"phase"`
	CurrentVersion string              `json:"current_version"`
	Latest         *updateReleaseInfo  `json:"latest,omitempty"`
	Changes        []updateReleaseInfo `json:"changes,omitempty"`
	Digest         string              `json:"digest,omitempty"`
	ArchiveName    string              `json:"archive_name,omitempty"`
	// Download progress in bytes while the phase is downloading. Total is 0
	// when the server sends no Content-Length, so the UI falls back to an
	// indeterminate display.
	Downloaded int64     `json:"downloaded,omitempty"`
	Total      int64     `json:"total,omitempty"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at,omitempty"`
}

type updateFlow struct {
	mu         sync.Mutex
	st         updateFlowStatus
	archiveDir string
	apiBase    string
	client     *http.Client
	// currentVersion is the installed version every release is compared
	// against. Injectable so tests can pin it.
	currentVersion string
}

func newUpdateFlow(archiveDir string) *updateFlow {
	current := version.Current().Version
	return &updateFlow{
		archiveDir:     archiveDir,
		apiBase:        updater.DefaultRepoAPI,
		client:         &http.Client{Timeout: 20 * time.Second},
		currentVersion: current,
		st:             updateFlowStatus{Phase: updateIdle, CurrentVersion: current},
	}
}

func (f *updateFlow) status() updateFlowStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.st
	st.Changes = append([]updateReleaseInfo(nil), f.st.Changes...)
	if st.Latest != nil {
		latest := *st.Latest
		st.Latest = &latest
	}
	return st
}

func (f *updateFlow) setPhase(phase, errText string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Phase = phase
	f.st.Error = errText
	if phase == updateIdle || phase == updateReady || phase == updateAvailable || phase == updateUpToDate {
		f.st.CheckedAt = time.Now().UTC()
	}
}

// check queries the GitHub releases API and collects notes of every release
// newer than the installed one.
func (f *updateFlow) check(ctx context.Context) error {
	current := f.currentVersion
	f.setPhase(updateChecking, "")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase+"/releases?per_page=15", nil)
	if err != nil {
		f.fail("Release-Prüfung fehlgeschlagen", err)
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := f.client.Do(req)
	if err != nil {
		f.fail("Release-Prüfung fehlgeschlagen", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("github api: %s", resp.Status)
		f.fail("Release-Prüfung fehlgeschlagen", err)
		return err
	}
	var releases []updater.Release
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err == nil {
		err = json.Unmarshal(body, &releases)
	}
	if err != nil {
		f.fail("Release-Prüfung fehlgeschlagen", err)
		return err
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.CurrentVersion = current
	f.st.Latest = latestInfo
	f.st.Changes = changes
	f.st.Digest = ""
	f.st.ArchiveName = ""
	f.st.Downloaded = 0
	f.st.Total = 0
	f.st.Error = ""
	f.st.CheckedAt = time.Now().UTC()
	if latestInfo == nil {
		f.st.Phase = updateUpToDate
	} else {
		f.st.Phase = updateAvailable
	}
	return nil
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
func (f *updateFlow) compareCommitNotes(ctx context.Context, current, latest string) (string, error) {
	base := strings.TrimSpace(current)
	if !isComparableVersion(base) {
		return "", errors.New("installed version cannot be compared")
	}
	if !strings.HasPrefix(strings.ToLower(base), "v") {
		base = "v" + base
	}
	endpoint := strings.TrimRight(f.apiBase, "/") + "/compare/" + url.PathEscape(base) + "..." + url.PathEscape(strings.TrimSpace(latest))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := f.client.Do(req)
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

func collectNewerReleases(releases []updater.Release, current string) (*updateReleaseInfo, []updateReleaseInfo) {
	var changes []updateReleaseInfo
	var latest *updateReleaseInfo
	for _, rel := range releases {
		if strings.TrimSpace(rel.Tag) == "" {
			continue
		}
		if updater.CompareVersions(rel.Tag, current) <= 0 {
			continue
		}
		info := updateReleaseInfo{
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

func (f *updateFlow) fail(message string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.st.Phase = updateFailed
	f.st.Error = message + ": " + err.Error()
}

var errNoDownload = errors.New("kein Update zum Herunterladen vorhanden")

// download fetches the newest release archive in the background. The UI polls
// status until the phase reaches ready or failed.
func (s *Server) startUpdateDownload(ctx context.Context) error {
	f := s.updates
	f.mu.Lock()
	switch f.st.Phase {
	case updateDownloading:
		f.mu.Unlock()
		return nil // already running
	case updateReady:
		f.mu.Unlock()
		return nil
	case updateAvailable:
	case updateFailed, updateIdle, updateUpToDate:
		latest := f.st.Latest
		if latest == nil {
			f.mu.Unlock()
			return errNoDownload
		}
	default:
		f.mu.Unlock()
		return fmt.Errorf("download im Zustand %q nicht möglich", f.st.Phase)
	}
	phase := updateDownloading
	f.st.Phase = phase
	f.st.Error = ""
	f.st.Downloaded = 0
	f.st.Total = 0
	url := ""
	if f.st.Latest != nil {
		url = f.st.Latest.URL
	}
	f.mu.Unlock()

	go func() {
		downloadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		path, digest, err := updater.FetchArchive(downloadCtx, url, s.updates.archiveDir, http.DefaultClient, func(done, total int64) {
			s.updates.mu.Lock()
			s.updates.st.Downloaded = done
			s.updates.st.Total = total
			s.updates.mu.Unlock()
		})
		s.updates.mu.Lock()
		defer s.updates.mu.Unlock()
		if err != nil {
			s.updates.st.Phase = updateFailed
			s.updates.st.Error = "Download fehlgeschlagen: " + err.Error()
			s.updates.st.Downloaded = 0
			s.updates.st.Total = 0
			return
		}
		s.updates.st.Phase = updateReady
		s.updates.st.Digest = digest
		s.updates.st.ArchiveName = filepath.Base(path)
		s.updates.st.Downloaded = s.updates.st.Total
		_ = s.app.Store.AddEvent(context.Background(), "info", "update.downloaded", "Update wurde heruntergeladen", map[string]string{"digest": digest})
	}()
	return nil
}

// installFromCache applies a previously downloaded archive. It reuses the same
// single-flight mutex as URL-based updates.
func (s *Server) startUpdateInstall(w http.ResponseWriter, r *http.Request) {
	f := s.updates
	f.mu.Lock()
	if f.st.Phase != updateReady || f.st.ArchiveName == "" {
		phase := f.st.Phase
		f.mu.Unlock()
		writeError(w, fmt.Errorf("kein heruntergeladenes Update bereit (Zustand: %s)", phase), http.StatusConflict)
		return
	}
	archivePath := filepath.Join(f.archiveDir, f.st.ArchiveName)
	digest := f.st.Digest
	f.st.Phase = updateInstalling
	f.st.Error = ""
	f.mu.Unlock()

	if !s.updateMu.TryLock() {
		f.setPhase(updateFailed, "es läuft bereits ein Update")
		writeError(w, errors.New("es läuft bereits ein Update"), http.StatusConflict)
		return
	}
	_ = s.app.Store.AddEvent(r.Context(), "info", "update.install_started", "Installieren des heruntergeladenen Updates gestartet", nil)
	go func() {
		defer s.updateMu.Unlock()
		s.runApply(func(ctx context.Context) (updater.Result, error) {
			return updater.Apply(ctx, updater.Options{
				Config:           s.app.Config,
				Archive:          archivePath,
				Digest:           digest,
				InstallDir:       s.app.Config.InstallDir,
				AutoRollback:     true,
				Restart:          updater.StackRestart(s.app.Config),
				Healthcheck:      updater.HTTPHealthcheck(s.app.Config),
				AllowInsecureURL: insecureUpdateAllowed(),
			})
		}, "archiv:"+filepath.Base(archivePath))
	}()
	writeJSON(w, map[string]string{"status": "installing"}, http.StatusAccepted)
}

// cleanupCachedArchive removes a stale downloaded archive after install/failure.
func (f *updateFlow) cleanupCachedArchive() {
	f.mu.Lock()
	name := f.st.ArchiveName
	f.st.ArchiveName = ""
	f.st.Digest = ""
	f.mu.Unlock()
	if name == "" {
		return
	}
	_ = removeArchiveFile(filepath.Join(f.archiveDir, name))
}

func removeArchiveFile(path string) error {
	if path == "" || !strings.HasPrefix(filepath.Base(path), "release-") {
		return errors.New("refusing to remove unexpected archive name")
	}
	return os.Remove(path)
}

func (s *Server) getUpdateStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.updates.status(), http.StatusOK)
}

func (s *Server) checkForUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.updates.check(ctx); err != nil {
		// The flow status carries the failure details; still answer with the
		// status so the UI can render the failed state.
		writeJSON(w, s.updates.status(), http.StatusBadGateway)
		return
	}
	writeJSON(w, s.updates.status(), http.StatusOK)
}

func (s *Server) downloadUpdate(w http.ResponseWriter, r *http.Request) {
	if err := s.startUpdateDownload(r.Context()); err != nil {
		writeError(w, err, http.StatusConflict)
		return
	}
	writeJSON(w, s.updates.status(), http.StatusAccepted)
}

func insecureUpdateAllowed() bool {
	return os.Getenv("CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE") == "1"
}
