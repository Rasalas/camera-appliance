package releasearchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSourceRequiresHTTPS(t *testing.T) {
	if err := (Source{Archive: "", URL: "https://example.com/release.tar.gz", AllowInsecureURL: false}).Validate(); err != nil {
		t.Fatalf("https URL should be accepted: %v", err)
	}
	if err := (Source{Archive: "", URL: "http://example.com/release.tar.gz", AllowInsecureURL: false}).Validate(); err == nil {
		t.Fatal("http URL should be rejected without explicit opt-in")
	}
	if err := (Source{Archive: "", URL: "http://127.0.0.1:8080/release.tar.gz", AllowInsecureURL: true}).Validate(); err != nil {
		t.Fatalf("http URL should pass with opt-in: %v", err)
	}
	if err := (Source{Archive: "archive.tar.gz", URL: "", AllowInsecureURL: false}).Validate(); err != nil {
		t.Fatalf("local archive should be accepted: %v", err)
	}
}

func TestPrepareVerifiesDigest(t *testing.T) {
	path := testArchive(t, map[string]string{"release/bin/camera-appliance": "binary", "release/manifest.json": `{"version":"1.2.3","commit":"abc"}`})
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	prepare := func(digest string) error {
		release, err := Prepare(context.Background(), Source{Archive: path, Digest: digest}, workspace, nil)
		if release != nil {
			release.Close()
		}
		return err
	}
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	if err := prepare("sha256:" + hexSum); err != nil {
		t.Fatalf("prefixed digest should match: %v", err)
	}
	if err := prepare(hexSum); err != nil {
		t.Fatalf("bare digest should match: %v", err)
	}
	if err := prepare(strings.ToUpper(hexSum)); err != nil {
		t.Fatalf("digest comparison should be case-insensitive: %v", err)
	}
	if err := prepare(strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong digest should fail")
	}
	if err := prepare("abc"); err == nil {
		t.Fatal("malformed digest should fail")
	}
	if err := prepare(""); err != nil {
		t.Fatalf("empty digest must not check anything: %v", err)
	}
	if err := prepare("md5:" + hexSum); err == nil {
		t.Fatal("unsupported algorithm should fail")
	}
}

func testArchive(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareOwnsDownloadAndStagingLifetime(t *testing.T) {
	archive := testArchive(t, map[string]string{"release/bin/camera-appliance": "binary", "release/manifest.json": `{"version":"1.2.3","commit":"abc"}`})
	payload, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	workspace := t.TempDir()
	release, err := Prepare(context.Background(), Source{URL: server.URL, AllowInsecureURL: true}, workspace, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if release.Manifest.Version != "1.2.3" {
		t.Fatalf("manifest %+v", release.Manifest)
	}
	binary, err := os.ReadFile(filepath.Join(release.Root, "bin", "camera-appliance"))
	if err != nil || string(binary) != "binary" {
		t.Fatalf("prepared binary %q %v", binary, err)
	}
	release.Close()
	release.Close()
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging leaked: %v %v", entries, err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal("source archive removed")
	}
}

func TestPrepareRejectsUnsafeInputWithoutLeavingPartialRelease(t *testing.T) {
	for _, name := range []string{"release/secrets.env", "release/.private/password", "release/snapshot-upload-password.json", "release/.upload-password-temp", "../escape"} {
		t.Run(name, func(t *testing.T) {
			archive := testArchive(t, map[string]string{"release/bin/camera-appliance": "binary", name: "sensitive"})
			workspace := t.TempDir()
			release, err := Prepare(context.Background(), Source{Archive: archive}, workspace, nil)
			if err == nil || release != nil {
				t.Fatalf("unsafe release accepted: %v", err)
			}
			entries, err := os.ReadDir(workspace)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed staging leaked: %v %v", entries, err)
			}
		})
	}
}

func TestOpenDirectoryDoesNotOwnCallerFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "bin", "camera-appliance")
	if err := os.WriteFile(binary, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	release, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	release.Close()
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("directory source was removed: %v", err)
	}
}
