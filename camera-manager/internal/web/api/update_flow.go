package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	Downloaded int64        `json:"downloaded,omitempty"`
	Total      int64        `json:"total,omitempty"`
	Error      string       `json:"error,omitempty"`
	CheckedAt  time.Time    `json:"checked_at,omitempty"`
	Job        *updater.Job `json:"job,omitempty"`
}

type updateFlow struct {
	mu         sync.Mutex
	st         updateFlowStatus
	archiveDir string
	catalog    updater.Catalog
	// currentVersion is the installed version every release is compared
	// against. Injectable so tests can pin it.
	currentVersion string
}

func newUpdateFlow(archiveDir string) *updateFlow {
	current := version.Current().Version
	return &updateFlow{
		archiveDir:     archiveDir,
		catalog:        updater.Catalog{BaseURL: updater.DefaultRepoAPI, Client: &http.Client{Timeout: 20 * time.Second}},
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
	result, err := f.catalog.Check(ctx, current)
	if err != nil {
		f.fail("Release-Prüfung fehlgeschlagen", err)
		return err
	}
	latestInfo, changes := result.Latest, result.Changes
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

// startUpdateInstall hands the archive to an independent, durable update job.
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
	s.submitUpdate(w, r, updater.Request{Archive: archivePath, Digest: digest, AutoRollback: true})
}

func (s *Server) submitUpdate(w http.ResponseWriter, r *http.Request, req updater.Request) {
	job, err := s.startUpdateJob(r.Context(), s.app.Config, req)
	if err != nil {
		s.updates.setPhase(updateFailed, err.Error())
		status := http.StatusInternalServerError
		if errors.Is(err, updater.ErrUpdateBusy) {
			status = http.StatusConflict
		}
		writeError(w, err, status)
		return
	}
	s.updates.setPhase(updateInstalling, "")
	// StartJob has its own copy; the downloaded cache is no longer needed.
	s.updates.cleanupCachedArchive()
	writeJSON(w, map[string]string{"status": "installing", "job_id": job.ID}, http.StatusAccepted)
}

func (s *Server) updateStatus() updateFlowStatus {
	st := s.updates.status()
	job, err := updater.ReadJob(s.app.Config)
	if errors.Is(err, os.ErrNotExist) {
		return st
	}
	if err != nil {
		st.Phase, st.Error = updateFailed, err.Error()
		return st
	}
	st.Job = &job
	if job.Phase == "queued" || job.Phase == "installing" {
		st.Phase, st.Error = updateInstalling, ""
	} else if st.CheckedAt.IsZero() || !job.UpdatedAt.Before(st.CheckedAt) || st.Phase == updateInstalling {
		if job.Phase == "failed" {
			st.Phase, st.Error = updateFailed, job.Error
		} else {
			st.Phase, st.Error = updateIdle, ""
		}
	}
	return st
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
	writeJSON(w, s.updateStatus(), http.StatusOK)
}

func (s *Server) checkForUpdate(w http.ResponseWriter, r *http.Request) {
	if st := s.updateStatus(); st.Phase == updateInstalling {
		writeJSON(w, st, http.StatusConflict)
		return
	}
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
	if st := s.updateStatus(); st.Phase == updateInstalling {
		writeJSON(w, st, http.StatusConflict)
		return
	}
	if err := s.startUpdateDownload(r.Context()); err != nil {
		writeError(w, err, http.StatusConflict)
		return
	}
	writeJSON(w, s.updates.status(), http.StatusAccepted)
}

func insecureUpdateAllowed() bool {
	return os.Getenv("CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE") == "1"
}

type updateReleaseInfo = updater.ReleaseInfo
