package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/backup"
)

func uploadRestore(t *testing.T, handler http.Handler, field string, data []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile(field, "sicherung.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/restore", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func backupArchive(t *testing.T, a *app.App, settings map[string]string) []byte {
	t.Helper()
	ctx := context.Background()
	if err := a.Store.PutSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	result, err := backup.Create(ctx, a.Config, filepath.Join(t.TempDir(), "backup.tar.gz"), false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertNoStagedUploads(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".restore-upload-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Fatalf("staged upload left behind: %v", matches)
	}
}

func TestRestoreUploadRestoresStateAndRemovesStagedFile(t *testing.T) {
	a := newAuthTestApp(t)
	archive := backupArchive(t, a, map[string]string{"viewer.performance.mode": "low"})
	if err := a.Store.PutSettings(context.Background(), map[string]string{"viewer.performance.mode": "quality"}); err != nil {
		t.Fatal(err)
	}
	res := uploadRestore(t, New(a).Handler(), "backup", archive, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "sicherung.tar.gz") {
		t.Fatalf("result should name the uploaded file: %s", res.Body.String())
	}
	settings, err := a.Store.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings["viewer.performance.mode"] != "low" {
		t.Fatalf("state not restored: %v", settings["viewer.performance.mode"])
	}
	assertNoStagedUploads(t, a.Config.StateDir)
}

func TestRestoreUploadRejectsInvalidArchiveWithoutChanges(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.Store.PutSettings(context.Background(), map[string]string{"viewer.performance.mode": "quality"}); err != nil {
		t.Fatal(err)
	}
	res := uploadRestore(t, New(a).Handler(), "backup", []byte("not a backup"), nil)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "keine gültige Sicherung") {
		t.Fatalf("invalid archive: status %d: %s", res.Code, res.Body.String())
	}
	settings, _ := a.Store.Settings(context.Background())
	if settings["viewer.performance.mode"] != "quality" {
		t.Fatal("invalid archive changed state")
	}
	assertNoStagedUploads(t, a.Config.StateDir)
}

func TestRestoreUploadEnforcesSizeLimit(t *testing.T) {
	a := newAuthTestApp(t)
	archive := backupArchive(t, a, map[string]string{"viewer.performance.mode": "low"})
	previous := restoreUploadLimit
	restoreUploadLimit = int64(len(archive) - 1)
	t.Cleanup(func() { restoreUploadLimit = previous })
	res := uploadRestore(t, New(a).Handler(), "backup", archive, nil)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	assertNoStagedUploads(t, a.Config.StateDir)
}

func TestRestoreUploadRequiresBackupField(t *testing.T) {
	a := newAuthTestApp(t)
	res := uploadRestore(t, New(a).Handler(), "other", []byte("x"), nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
}

func TestRestoreUploadRequiresAdmin(t *testing.T) {
	a := newAuthTestApp(t)
	archive := backupArchive(t, a, map[string]string{"viewer.performance.mode": "low"})
	for _, role := range []string{"admin", "viewer"} {
		if err := a.SetAuthPassword(context.Background(), role, "fixture-pass"); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(a).Handler()
	viewer := loginCookie(t, handler, "viewer", "fixture-pass")
	for _, cookie := range []*http.Cookie{nil, viewer} {
		res := uploadRestore(t, handler, "backup", archive, cookie)
		if res.Code != http.StatusUnauthorized && res.Code != http.StatusForbidden {
			t.Fatalf("restore upload exposed with status %d", res.Code)
		}
	}
}
