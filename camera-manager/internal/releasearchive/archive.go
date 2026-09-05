package releasearchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Manifest struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

func (s Source) Validate() error {
	archivePath, rawURL, allowInsecure := s.Archive, s.URL, s.AllowInsecureURL
	if (archivePath == "") == (rawURL == "") {
		return errors.New("exactly one of --archive or --url is required")
	}
	if rawURL != "" {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid update URL %q", redactUpdateURL(rawURL))
		}
		if parsed.Scheme != "https" && !(allowInsecure && parsed.Scheme == "http") {
			return fmt.Errorf("update URL must use https, got %q; set CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE=1 only for local development", redactUpdateURL(rawURL))
		}
	}
	return nil
}

func redactUpdateURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User == nil {
		return rawURL
	}
	parsed.User = url.User(parsed.User.Username())
	return parsed.String()
}

// verifyDigest checks the downloaded or local archive against an expected
// checksum ("sha256:<hex>" or bare hex). Updates replace the running binary,
// so integrity must never be skipped silently.
func verifyDigest(archivePath, expected string) error {
	if strings.TrimSpace(expected) == "" {
		return nil
	}
	algorithm, digest := "sha256", strings.ToLower(strings.TrimSpace(expected))
	if algo, rest, found := strings.Cut(digest, ":"); found {
		algorithm, digest = strings.ToLower(strings.TrimSpace(algo)), strings.ToLower(strings.TrimSpace(rest))
	}
	if algorithm != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q, only sha256 is supported", algorithm)
	}
	if len(digest) != 64 {
		return fmt.Errorf("invalid sha256 digest %q: expected 64 hex characters", digest)
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(sum.Sum(nil))
	if actual != digest {
		return fmt.Errorf("update archive digest mismatch: expected sha256:%s, got sha256:%s", digest, actual)
	}
	return nil
}

func downloadArchive(ctx context.Context, rawURL string, client *http.Client) (string, func(), error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("update download failed: %s", resp.Status)
	}
	file, err := os.CreateTemp("", "camera-appliance-update-*.tar.gz")
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 512*1024*1024)); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func extractReleaseArchive(ctx context.Context, archivePath, dst string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := cleanArchiveName(header.Name)
		if err != nil {
			return err
		}
		if forbiddenArchivePath(name) {
			return fmt.Errorf("release archive contains forbidden path %q", name)
		}
		target := filepath.Join(dst, name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(filepath.Separator)) {
			return fmt.Errorf("release archive path escapes destination: %q", header.Name)
		}
		mode := header.FileInfo().Mode()
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode.Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func cleanArchiveName(name string) (string, error) {
	clean := filepath.Clean(strings.TrimLeft(name, string(filepath.Separator)))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid release archive path %q", name)
	}
	return clean, nil
}

func forbiddenArchivePath(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		lower := strings.ToLower(part)
		if lower == ".ds_store" || strings.HasPrefix(lower, "._") || strings.HasPrefix(lower, ".upload-password-") {
			return true
		}
		switch lower {
		case ".git", ".private", "data", "node_modules", "secrets.env", "local.env", ".env", "snapshot-upload-password.json":
			return true
		}
	}
	return false
}

func findReleaseRoot(stageDir string) (string, Manifest, error) {
	if validReleaseRoot(stageDir) {
		manifest, err := ReadManifest(stageDir)
		return stageDir, manifest, err
	}
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return "", Manifest{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(stageDir, entry.Name())
		if validReleaseRoot(candidate) {
			manifest, err := ReadManifest(candidate)
			return candidate, manifest, err
		}
	}
	return "", Manifest{}, errors.New("release archive must contain bin/camera-appliance")
}

func validReleaseRoot(root string) bool {
	info, err := os.Stat(filepath.Join(root, "bin", "camera-appliance"))
	return err == nil && !info.IsDir()
}

func ReadManifest(root string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Manifest{Version: "unknown", Commit: "unknown"}, nil
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Version == "" {
		manifest.Version = "unknown"
	}
	if manifest.Commit == "" {
		manifest.Commit = "unknown"
	}
	return manifest, nil
}

// Source identifies exactly one archive. Prepare owns download, verification,
// extraction and cleanup; callers never manage intermediate archive paths.
type Source struct {
	Archive          string
	URL              string
	Digest           string
	AllowInsecureURL bool
}
type Release struct {
	Root     string
	Manifest Manifest
	cleanup  func()
}

func (r *Release) Close() {
	if r.cleanup != nil {
		r.cleanup()
	}
}
func OpenDirectory(path string) (*Release, error) {
	root, manifest, err := findReleaseRoot(path)
	if err != nil {
		return nil, err
	}
	return &Release{Root: root, Manifest: manifest}, nil
}
func Prepare(ctx context.Context, source Source, workspace string, client *http.Client) (*Release, error) {
	if err := source.Validate(); err != nil {
		return nil, err
	}
	archive := source.Archive
	cleanup := func() {}
	var err error
	if source.URL != "" {
		archive, cleanup, err = downloadArchive(ctx, source.URL, client)
		if err != nil {
			return nil, err
		}
	}
	defer cleanup()
	if err := verifyDigest(archive, source.Digest); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(workspace, "camera-appliance-release-")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := extractReleaseArchive(ctx, archive, dir); err != nil {
		return nil, err
	}
	root, manifest, err := findReleaseRoot(dir)
	if err != nil {
		return nil, err
	}
	keep = true
	return &Release{Root: root, Manifest: manifest, cleanup: func() { _ = os.RemoveAll(dir) }}, nil
}
