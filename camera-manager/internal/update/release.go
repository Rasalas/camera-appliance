package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultRepoAPI  = "https://api.github.com/repos/Rasalas/camera-appliance"
	MaxArchiveBytes = 512 * 1024 * 1024
)

// Release describes one GitHub release with its archive asset.
type Release struct {
	Tag         string `json:"tag_name"`
	Name        string `json:"name"`
	Notes       string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// ArchiveURL picks the versioned archive asset of the release, falling back
// to the rolling latest asset.
func (r Release) ArchiveURL() string {
	for _, asset := range r.Assets {
		if strings.HasSuffix(asset.Name, ".tar.gz") && strings.Contains(asset.Name, "camera-appliance") &&
			!strings.EqualFold(asset.Name, "camera-appliance-latest.tar.gz") {
			return asset.BrowserDownloadURL
		}
	}
	for _, asset := range r.Assets {
		if strings.EqualFold(asset.Name, "camera-appliance-latest.tar.gz") {
			return asset.BrowserDownloadURL
		}
	}
	return DefaultReleaseURL
}

// CompareVersions orders dotted release versions. It returns -1 when a < b,
// 0 when equal and 1 when a > b. Non-numeric parts are compared lexically;
// a missing part counts as lower. "dev" always compares as older.
func CompareVersions(a, b string) int {
	a, b = strings.TrimPrefix(strings.TrimSpace(a), "v"), strings.TrimPrefix(strings.TrimSpace(b), "v")
	if a == b {
		return 0
	}
	if strings.EqualFold(a, "dev") {
		return -1
	}
	if strings.EqualFold(b, "dev") {
		return 1
	}
	aParts, bParts := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aParts) || i < len(bParts); i++ {
		// Missing parts count as 0 so "1.0" equals "1.0.0".
		var as, bs string
		if i < len(aParts) {
			as = aParts[i]
		} else if i < len(bParts) {
			as = "0"
		}
		if i < len(bParts) {
			bs = bParts[i]
		} else if i < len(aParts) {
			bs = "0"
		}
		if as == bs {
			continue
		}
		an, aerr := strconv.Atoi(as)
		bn, berr := strconv.Atoi(bs)
		switch {
		case aerr == nil && berr == nil:
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		default:
			// Mixed shapes (e.g. "1" vs "1-beta"): lexical tiebreak.
			if as < bs {
				return -1
			}
			return 1
		}
	}
	return 0
}

// FetchArchive downloads the release archive to dir and returns the file path
// plus its sha256 hex digest. The caller owns the returned file.
func FetchArchive(ctx context.Context, url, dir string, httpClient *http.Client) (path, digest string, err error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("release download failed: %s", resp.Status)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", err
	}
	file, err := os.CreateTemp(dir, "release-*.tar.gz")
	if err != nil {
		return "", "", err
	}
	path = file.Name()
	sum := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, sum), io.LimitReader(resp.Body, MaxArchiveBytes))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || written >= MaxArchiveBytes {
		_ = os.Remove(path)
		if written >= MaxArchiveBytes {
			return "", "", fmt.Errorf("release archive exceeds %d bytes limit", MaxArchiveBytes)
		}
		return "", "", err
	}
	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chmod(path, info.Mode().Perm()&0o600)
	}
	return path, hex.EncodeToString(sum.Sum(nil)), nil
}

// ArchiveBaseName returns the file name portion of an archive path.
func ArchiveBaseName(path string) string { return filepath.Base(path) }
