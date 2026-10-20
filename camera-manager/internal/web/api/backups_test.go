package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/backup"
)

func seedBackupFile(t *testing.T, a *app.App, name, content string) {
	t.Helper()
	if err := os.MkdirAll(a.Config.BackupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Config.BackupDir(), name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBackupListShowsKindsSizeAndWarning(t *testing.T) {
	a := newAuthTestApp(t)
	seedBackupFile(t, a, "camera-appliance-20260101-120000.tar.gz", "manual")
	seedBackupFile(t, a, "camera-appliance-auto-20260102-120000.tar.gz", "auto")
	seedBackupFile(t, a, "camera-appliance-update-20260103-120000.tar.gz", "update")
	seedBackupFile(t, a, "support-bundle-20260104-120000.tar.gz", "bundle")
	res := performJSON(New(a).Handler(), http.MethodGet, "/api/backups", nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	var body struct {
		Backups []backup.Entry       `json:"backups"`
		Auto    app.AutoBackupStatus `json:"auto"`
		Warning string               `json:"warning"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Backups) != 3 || body.Backups[0].Kind != backup.KindUpdate || body.Backups[1].Kind != backup.KindAutomatic || body.Backups[2].Kind != backup.KindManual {
		t.Fatalf("backups = %+v", body.Backups)
	}
	if body.Backups[2].Size != 6 || !body.Auto.Enabled || body.Auto.Keep != 7 || body.Auto.LastAt == nil {
		t.Fatalf("body = %+v", body)
	}
	if !strings.Contains(body.Warning, "Kamera-Passwörter") {
		t.Fatalf("warning = %q", body.Warning)
	}
	if strings.Contains(res.Body.String(), a.Config.StateDir) {
		t.Fatal("list leaks server paths")
	}
}

func TestBackupDownloadReturnsAttachment(t *testing.T) {
	a := newAuthTestApp(t)
	name := "camera-appliance-auto-20260102-120000.000000001.tar.gz"
	seedBackupFile(t, a, name, "archive-bytes")
	res := performJSON(New(a).Handler(), http.MethodGet, "/api/backups/"+name, nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if res.Body.String() != "archive-bytes" || res.Header().Get("Content-Type") != "application/gzip" {
		t.Fatalf("body %q type %q", res.Body.String(), res.Header().Get("Content-Type"))
	}
	if got := res.Header().Get("Content-Disposition"); got != `attachment; filename=`+name {
		t.Fatalf("disposition = %q", got)
	}
	if res.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("download may be cached")
	}
}

func TestBackupDownloadAndDeleteRejectTraversal(t *testing.T) {
	a := newAuthTestApp(t)
	seedBackupFile(t, a, "camera-appliance-20260101-120000.tar.gz", "ok")
	handler := New(a).Handler()
	for _, target := range []string{
		"/api/backups/..%2Fstate.db",
		"/api/backups/..%2F..%2Fetc%2Fpasswd",
		"/api/backups/%2Fetc%2Fpasswd",
		"/api/backups/last-update.json",
		"/api/backups/state.db",
		"/api/backups/camera-appliance-20260101-120000.tar.gz%2F..",
		"/api/backups/..",
		"/api/backups/../state.db",
		"/api/backups/../../../etc/passwd",
	} {
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			res := performJSON(handler, method, target, nil, nil)
			if res.Code == http.StatusOK {
				t.Fatalf("%s %s served: %s", method, target, res.Body.String())
			}
			if strings.Contains(res.Body.String(), "SQLite") || strings.Contains(res.Body.String(), "root:") {
				t.Fatalf("%s %s leaked file content", method, target)
			}
		}
	}
	res := performJSON(handler, http.MethodGet, "/api/backups/..%2Fstate.db", nil, nil)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("encoded traversal status %d", res.Code)
	}
	if _, err := os.Stat(a.Config.DBPath()); err != nil {
		t.Fatal("state.db was removed")
	}
	if res := performJSON(handler, http.MethodGet, "/api/backups/camera-appliance-20260109-120000.tar.gz", nil, nil); res.Code != http.StatusNotFound {
		t.Fatalf("missing backup status %d", res.Code)
	}
}

func TestBackupDeleteRemovesOnlyTheNamedBackup(t *testing.T) {
	a := newAuthTestApp(t)
	seedBackupFile(t, a, "camera-appliance-20260101-120000.tar.gz", "a")
	seedBackupFile(t, a, "camera-appliance-update-20260102-120000.tar.gz", "b")
	res := performJSON(New(a).Handler(), http.MethodDelete, "/api/backups/camera-appliance-20260101-120000.tar.gz", nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	entries, err := backup.List(a.Config)
	if err != nil || len(entries) != 1 || entries[0].Kind != backup.KindUpdate {
		t.Fatalf("entries = %+v err = %v", entries, err)
	}
}

func TestCreateBackupIgnoresClientPathAndSecrets(t *testing.T) {
	a := newAuthTestApp(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.tar.gz")
	res := performJSON(New(a).Handler(), http.MethodPost, "/api/backup", map[string]any{"out": outside, "include_secrets": true}, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("backup written to client path")
	}
	if strings.Contains(res.Body.String(), "secrets.env") {
		t.Fatal("secrets.env included")
	}
	entries, err := backup.List(a.Config)
	if err != nil || len(entries) != 1 || entries[0].Kind != backup.KindManual {
		t.Fatalf("entries = %+v err = %v", entries, err)
	}
}

func TestBackupEndpointsRequireAdmin(t *testing.T) {
	a := newAuthTestApp(t)
	name := "camera-appliance-20260101-120000.tar.gz"
	seedBackupFile(t, a, name, "archive")
	for _, role := range []string{"admin", "viewer"} {
		if err := a.SetAuthPassword(context.Background(), role, "fixture-pass"); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(a).Handler()
	viewer := loginCookie(t, handler, "viewer", "fixture-pass")
	endpoints := []struct{ method, path string }{
		{http.MethodGet, "/api/backups"},
		{http.MethodGet, "/api/backups/" + name},
		{http.MethodDelete, "/api/backups/" + name},
		{http.MethodPost, "/api/backup"},
	}
	for _, endpoint := range endpoints {
		for _, cookie := range []*http.Cookie{nil, viewer} {
			res := performJSON(handler, endpoint.method, endpoint.path, nil, cookie)
			if res.Code != http.StatusUnauthorized && res.Code != http.StatusForbidden {
				t.Fatalf("%s %s exposed with status %d", endpoint.method, endpoint.path, res.Code)
			}
		}
	}
	admin := loginCookie(t, handler, "admin", "fixture-pass")
	if res := performJSON(handler, http.MethodGet, "/api/backups/"+name, nil, admin); res.Code != http.StatusOK || res.Body.String() != "archive" {
		t.Fatalf("admin download status %d", res.Code)
	}
}
